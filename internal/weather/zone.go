package weather

import (
	_ "embed"
	"os"
	"strconv"
	"strings"
)

// zone1970 is IANA tzdb's zone1970.tab (2026e, public domain): each time
// zone's principal location. macOS doesn't ship it, so it's built in.
//
//go:embed zone1970.tab
var zone1970 string

// zoneCity is the principal city of IANA time zone name, as an approximate place.
func zoneCity(name string) (place, bool) {
	if name == "" {
		return place{}, false
	}
	for line := range strings.SplitSeq(zone1970, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 || cols[2] != name {
			continue
		}
		lat, lon, ok := iso6709(cols[1])
		if !ok {
			return place{}, false
		}
		city := name[strings.LastIndex(name, "/")+1:]
		return place{Lat: lat, Lon: lon, Name: strings.ReplaceAll(city, "_", " "), Source: fromZone}, true
	}
	return place{}, false
}

// iso6709 reads zone1970.tab's ±DDMM[SS]±DDDMM[SS] coordinates.
func iso6709(s string) (lat, lon float64, ok bool) {
	if len(s) < 2 {
		return 0, 0, false
	}
	i := strings.IndexAny(s[1:], "+-") + 1
	if i <= 0 {
		return 0, 0, false
	}
	lat, ok1 := dms(s[:i], 2)
	lon, ok2 := dms(s[i:], 3)
	return lat, lon, ok1 && ok2
}

func dms(s string, degDigits int) (float64, bool) {
	sign := 1.0
	if s[0] == '-' {
		sign = -1
	}
	d := s[1:]
	if len(d) != degDigits+2 && len(d) != degDigits+4 {
		return 0, false
	}
	deg, err1 := strconv.Atoi(d[:degDigits])
	minutes, err2 := strconv.Atoi(d[degDigits : degDigits+2])
	sec, err3 := 0, error(nil)
	if len(d) == degDigits+4 {
		sec, err3 = strconv.Atoi(d[degDigits+2:])
	}
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return sign * (float64(deg) + float64(minutes)/60 + float64(sec)/3600), true
}

// localZone is the machine's IANA time zone name: $TZ, else where
// /etc/localtime points, else /etc/timezone; "" when none says (Windows).
// time.Local can't tell: its name is "Local".
func localZone() string {
	if tz, ok := os.LookupEnv("TZ"); ok {
		return zoneName(strings.TrimPrefix(tz, ":"))
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.LastIndex(link, "zoneinfo/"); i >= 0 {
			return link[i+len("zoneinfo/"):]
		}
	}
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// zoneName is a TZ value's zone name, which may be a path into zoneinfo.
func zoneName(tz string) string {
	if i := strings.LastIndex(tz, "zoneinfo/"); i >= 0 {
		return tz[i+len("zoneinfo/"):]
	}
	return tz
}
