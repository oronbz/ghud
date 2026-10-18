package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type settings struct {
	units, city, lat, lon string
}

func settingsOf(m map[string]any) settings {
	str := func(k string) string {
		v, _ := m[k].(string)
		return strings.TrimSpace(v)
	}
	return settings{units: str("units"), city: str("city"), lat: str("lat"), lon: str("lon")}
}

// Where a place came from, which the header names when it's a guess.
const (
	fromSettings = "settings"
	fromIP       = "ip"
	fromZone     = "timezone"
)

type place struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Name   string  `json:"place"`
	Source string  `json:"source"`
}

func (p place) header() string {
	switch p.Source {
	case fromIP:
		return p.Name + " · from IP"
	case fromZone:
		return "~" + p.Name + " · from timezone"
	}
	return p.Name
}

const (
	// fresh is how long an IP location is trusted before asking again.
	fresh = 3 * time.Hour
	// locationFile, under ghud's cache dir, is shared by every instance.
	locationFile = "weather/location.json"
)

// cached is location.json. IP is the last good IP location with the time
// zone the machine was in then; IPRetry is when ipapi.is may be asked again
// after refusing (it blocks an IP for a day if asked on).
type cached struct {
	IP      *ipPlace         `json:"ip,omitempty"`
	IPRetry time.Time        `json:"ip_retry,omitzero"`
	Cities  map[string]place `json:"cities,omitempty"`
}

type ipPlace struct {
	place
	Zone string    `json:"zone"`
	At   time.Time `json:"at"`
}

// locations resolves places through the location chain, keeping
// location.json up to date.
type locations struct {
	cfg config
	mu  sync.Mutex
}

// locate finds where a Widget shows the weather for: lat/lon, else city,
// else the IP chain. force skips the cache.
func (l *locations) locate(ctx context.Context, s settings, force bool) (place, error) {
	if s.lat != "" || s.lon != "" {
		return coordinates(s)
	}
	if s.city != "" {
		return l.city(ctx, s.city, force)
	}
	return l.byIP(ctx, force)
}

func coordinates(s settings) (place, error) {
	lat, err1 := strconv.ParseFloat(s.lat, 64)
	lon, err2 := strconv.ParseFloat(s.lon, 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return place{}, &setupError{hint: "Set lat and lon to numbers, such as lat = \"32.08\" and lon = \"34.88\""}
	}
	name := s.city
	if name == "" {
		name = fmt.Sprintf("%.2f, %.2f", lat, lon)
	}
	return place{Lat: lat, Lon: lon, Name: name, Source: fromSettings}, nil
}

// city geocodes name once; later lookups read location.json.
func (l *locations) city(ctx context.Context, name string, force bool) (place, error) {
	key := strings.Join(strings.Fields(strings.ToLower(name)), " ")
	if key == "" {
		return place{}, &setupError{hint: "Set city"}
	}
	if !force {
		if p, ok := l.read().Cities[key]; ok {
			return p, nil
		}
	}
	p, err := geocode(ctx, l.cfg, name)
	if err != nil {
		return place{}, err
	}
	l.update(func(c *cached) {
		if c.Cities == nil {
			c.Cities = map[string]place{}
		}
		c.Cities[key] = p
	})
	return p, nil
}

// byIP is the IP chain: a fresh cached location, ipapi.is, the last good
// location, then the time zone's principal city.
func (l *locations) byIP(ctx context.Context, force bool) (place, error) {
	zone, now := l.cfg.zone(), l.cfg.now()
	c := l.read()
	last := c.IP
	if last != nil && last.Zone != zone {
		// Moved to another time zone: the cached place is likely somewhere else.
		last = nil
	}
	if !force && last != nil && now.Sub(last.At) < fresh && !last.At.After(now) {
		return last.place, nil
	}
	if now.Before(c.IPRetry) {
		return fallback(last, zone)
	}
	p, err := lookupIP(ctx, l.cfg)
	var refused *refusedError
	switch {
	case errors.As(err, &refused):
		retry := nextUTCMidnight(now)
		l.update(func(c *cached) { c.IPRetry = retry })
		return fallback(last, zone)
	case err != nil:
		return fallback(last, zone)
	}
	l.update(func(c *cached) { c.IP = &ipPlace{place: p, Zone: zone, At: now} })
	return p, nil
}

func fallback(last *ipPlace, zone string) (place, error) {
	if last != nil {
		return last.place, nil
	}
	if p, ok := zoneCity(zone); ok {
		return p, nil
	}
	return place{}, &setupError{hint: "Set city"}
}

func nextUTCMidnight(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC)
}

func (l *locations) path() string { return filepath.Join(l.cfg.cacheDir, locationFile) }

func (l *locations) read() cached {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.load()
}

func (l *locations) load() cached {
	var c cached
	if b, err := os.ReadFile(l.path()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

// update rewrites location.json with change applied to what's on disk now,
// so another instance's write in between isn't lost. Failing to write only
// costs a lookup next time.
func (l *locations) update(change func(*cached)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.load()
	change(&c)
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	path := l.path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".location-*")
	if err != nil {
		return
	}
	_, werr := f.Write(b)
	if err := errors.Join(werr, f.Close()); err != nil {
		os.Remove(f.Name())
		return
	}
	if err := os.Rename(f.Name(), path); err != nil {
		os.Remove(f.Name())
	}
}

// refusedError is ipapi.is saying no more lookups today.
type refusedError struct{}

func (refusedError) Error() string { return "ipapi.is refused the lookup" }

// lookupIP asks ipapi.is where this machine's IP address is.
func lookupIP(ctx context.Context, cfg config) (place, error) {
	var res struct {
		City    *string  `json:"city"`
		Region  *string  `json:"region"`
		Country *string  `json:"country"`
		Lat     *float64 `json:"lat"`
		Lon     *float64 `json:"lon"`
	}
	status, err := getJSON(ctx, cfg, "ipapi.is", strings.TrimRight(cfg.ipapi, "/")+"/", &res)
	switch {
	case status == http.StatusTooManyRequests:
		return place{}, &refusedError{}
	case err != nil:
		return place{}, err
	case res.Lat == nil || res.Lon == nil:
		return place{}, errors.New("ipapi.is doesn't know where this IP is")
	}
	name := ""
	for _, n := range []*string{res.City, res.Region, res.Country} {
		if n != nil && *n != "" {
			name = *n
			break
		}
	}
	if name == "" {
		name = fmt.Sprintf("%.2f, %.2f", *res.Lat, *res.Lon)
	}
	return place{Lat: *res.Lat, Lon: *res.Lon, Name: name, Source: fromIP}, nil
}

// geocode finds a city with Open-Meteo's geocoding API.
func geocode(ctx context.Context, cfg config, name string) (place, error) {
	q := url.Values{"name": {name}, "count": {"1"}, "language": {"en"}, "format": {"json"}}
	var res struct {
		Results []struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if _, err := getJSON(ctx, cfg, "Open-Meteo", strings.TrimRight(cfg.geocoding, "/")+"/v1/search?"+q.Encode(), &res); err != nil {
		return place{}, err
	}
	if len(res.Results) == 0 {
		return place{}, &setupError{hint: fmt.Sprintf("Open-Meteo knows no place called %q; set city", name)}
	}
	r := res.Results[0]
	return place{Lat: r.Latitude, Lon: r.Longitude, Name: r.Name, Source: fromSettings}, nil
}

// getJSON GETs u from service into v. A failed request is a *failure
// naming the service; its status code comes back too.
func getJSON(ctx context.Context, cfg config, service, u string, v any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	res, err := cfg.http.Do(req)
	if err != nil {
		return 0, &failure{msg: fmt.Sprintf("%s: %v", service, err), reason: "offline", hint: "Check the network connection"}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var e struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		why := e.Reason
		if why == "" {
			why = res.Status
		}
		reason := service + " error"
		if res.StatusCode == http.StatusTooManyRequests {
			reason = "rate limited"
		}
		return res.StatusCode, &failure{msg: service + ": " + why, reason: reason}
	}
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return res.StatusCode, &failure{msg: fmt.Sprintf("%s sent an answer ghud-weather can't read: %v", service, err), reason: service + " error"}
	}
	return res.StatusCode, nil
}
