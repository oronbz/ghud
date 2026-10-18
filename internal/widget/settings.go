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
)

// Setting is one key a Widget type accepts in its [[widget]] block.
type Setting struct {
	Name string
	Kind Kind
	// Default is a string, bool, int64 or time.Duration matching Kind.
	Default     any
	Description string
	// Why says when you'd change it; the config reference prints it.
	Why string
	// Choices, when set, are the only values a String accepts.
	Choices []string
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
	}
	return nil, fmt.Errorf("has unknown kind %d", s.Kind)
}

// TOMLValue writes a string, bool, int64 or time.Duration as a TOML value.
func TOMLValue(v any) string {
	switch v := v.(type) {
	case string:
		return strconv.Quote(v)
	case time.Duration:
		return strconv.Quote(v.String())
	default:
		return fmt.Sprint(v)
	}
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

// Defaults are the settings of a Widget whose block sets none.
func Defaults(schema []Setting) Settings {
	s := make(Settings, len(schema))
	for _, st := range schema {
		s[st.Name] = st.Default
	}
	return s
}
