package widget

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Kind is the TOML type a Setting takes.
type Kind int

const (
	String Kind = iota
	Bool
	Int
	// Duration is a string time.ParseDuration accepts, such as "5m".
	Duration
	// Tables is an array of tables, each checked against Fields.
	Tables
)

// Setting is one key a Widget type accepts in its [[widget]] block.
type Setting struct {
	Name string
	Kind Kind
	// Default is a string, bool, int64, time.Duration or []Settings matching Kind.
	Default     any
	Description string
	// Why says when you'd change it; the config reference prints it.
	Why string
	// Choices, when set, are the only values a String accepts.
	Choices []string
	// Fields are the keys each table of a Tables setting takes.
	Fields []Setting
	// Required settings belong in every block: Add Widget writes them, at
	// their defaults, into the block it appends, and a table missing a
	// required field is rejected.
	Required bool
}

// Check converts a decoded TOML value to the setting's Go type, or says why it doesn't fit.
func (s Setting) Check(v any) (any, error) {
	switch s.Kind {
	case String:
		str, ok := v.(string)
		if !ok {
			return nil, errors.New("must be a string")
		}
		if len(s.Choices) > 0 && !slices.Contains(s.Choices, str) {
			return nil, fmt.Errorf("must be one of %s", quoteAll(s.Choices))
		}
		return str, nil
	case Bool:
		b, ok := v.(bool)
		if !ok {
			return nil, errors.New("must be true or false")
		}
		return b, nil
	case Int:
		n, ok := v.(int64)
		if !ok {
			return nil, errors.New("must be a whole number")
		}
		return n, nil
	case Duration:
		str, _ := v.(string)
		d, err := time.ParseDuration(str)
		if err != nil || d <= 0 {
			return nil, errors.New(`must be a duration such as "5m"`)
		}
		return d, nil
	case Tables:
		return s.checkTables(v)
	}
	return nil, fmt.Errorf("has unknown kind %d", s.Kind)
}

func (s Setting) checkTables(v any) (any, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, errors.New("must be an array of tables, such as [{ ... }]")
	}
	out := make([]Settings, 0, len(list))
	for i, t := range list {
		table, ok := t.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("entry %d must be a table", i+1)
		}
		got := Defaults(s.Fields)
		keys := make([]string, 0, len(table))
		for k := range table {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			fi := slices.IndexFunc(s.Fields, func(f Setting) bool { return f.Name == k })
			if fi < 0 {
				return nil, fmt.Errorf("entry %d has unknown key `%s`", i+1, k)
			}
			val, err := s.Fields[fi].Check(table[k])
			if err != nil {
				return nil, fmt.Errorf("entry %d: `%s` %v", i+1, k, err)
			}
			got[k] = val
		}
		for _, f := range s.Fields {
			if _, set := table[f.Name]; f.Required && !set {
				return nil, fmt.Errorf("entry %d needs `%s`", i+1, f.Name)
			}
		}
		out = append(out, got)
	}
	return out, nil
}

// TOMLValue writes a string, bool, int64 or time.Duration as a TOML value.
func TOMLValue(v any) string {
	switch v := v.(type) {
	case string:
		return strconv.Quote(v)
	case time.Duration:
		return strconv.Quote(ShortDuration(v))
	case []Settings:
		if len(v) == 0 {
			return "[]"
		}
		tables := make([]string, len(v))
		for i, t := range v {
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			pairs := make([]string, len(keys))
			for j, k := range keys {
				pairs[j] = k + " = " + TOMLValue(t[k])
			}
			tables[i] = "{ " + strings.Join(pairs, ", ") + " }"
		}
		return "[" + strings.Join(tables, ", ") + "]"
	default:
		return fmt.Sprint(v)
	}
}

// ShortDuration writes d the way people write it in config.toml, such as
// "30s", "5m" or "1h30m" rather than "5m0s".
func ShortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = strconv.Quote(s)
	}
	return strings.Join(q, ", ")
}

// Settings are a Widget's validated settings with defaults filled in.
type Settings map[string]any

func (s Settings) String(name string) string          { v, _ := s[name].(string); return v }
func (s Settings) Bool(name string) bool              { v, _ := s[name].(bool); return v }
func (s Settings) Int(name string) int64              { v, _ := s[name].(int64); return v }
func (s Settings) Duration(name string) time.Duration { v, _ := s[name].(time.Duration); return v }
func (s Settings) Tables(name string) []Settings      { v, _ := s[name].([]Settings); return v }

// Defaults are the settings of a Widget whose block sets none.
func Defaults(schema []Setting) Settings {
	s := make(Settings, len(schema))
	for _, st := range schema {
		s[st.Name] = st.Default
	}
	return s
}
