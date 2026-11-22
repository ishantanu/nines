package v1beta1

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time span written as a string with explicit units, for
// example 28d, 1h30m, 5m, 15s or 300ms. Days are exactly 24 hours.
type Duration struct{ time.Duration }

// D builds a Duration.
func D(d time.Duration) Duration { return Duration{d} }

var durationPattern = regexp.MustCompile(`^([0-9]+(d|h|m|s|ms))+$`)

// ParseDuration reads a positive duration such as 28d or 300ms.
func ParseDuration(text string) (Duration, error) {
	if len(text) > 32 || !durationPattern.MatchString(text) {
		return Duration{}, fmt.Errorf("%q is not a duration; use units d, h, m, s or ms, for example 28d, 5m or 300ms", text)
	}
	var total time.Duration
	rest := text
	for rest != "" {
		i := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
		n, err := strconv.ParseInt(rest[:i], 10, 64)
		if err != nil {
			return Duration{}, fmt.Errorf("%q is out of range", text)
		}
		unit := rest[i:]
		var step time.Duration
		switch {
		case strings.HasPrefix(unit, "ms"):
			step, unit = time.Millisecond, "ms"
		case strings.HasPrefix(unit, "d"):
			step, unit = 24*time.Hour, "d"
		case strings.HasPrefix(unit, "h"):
			step, unit = time.Hour, "h"
		case strings.HasPrefix(unit, "m"):
			step, unit = time.Minute, "m"
		default:
			step, unit = time.Second, "s"
		}
		if n > int64(400*24*time.Hour/step) {
			return Duration{}, fmt.Errorf("%q is longer than 400 days", text)
		}
		total += time.Duration(n) * step
		rest = rest[i+len(unit):]
	}
	if total <= 0 {
		return Duration{}, fmt.Errorf("%q must be longer than zero", text)
	}
	return Duration{total}, nil
}

// String writes the shortest exact form: 28d, 1h30m, 5m, 300ms.
func (d Duration) String() string {
	v := d.Duration
	if v <= 0 {
		return "0s"
	}
	var b strings.Builder
	for _, u := range []struct {
		name string
		size time.Duration
	}{{"d", 24 * time.Hour}, {"h", time.Hour}, {"m", time.Minute}, {"s", time.Second}, {"ms", time.Millisecond}} {
		if n := v / u.size; n > 0 {
			_, _ = fmt.Fprintf(&b, "%d%s", n, u.name)
			v -= n * u.size
		}
	}
	return b.String()
}

// WholeSeconds returns whole seconds, refusing a fractional value where a window
// needs whole seconds.
func (d Duration) WholeSeconds(field string) (int64, error) {
	if d.Duration%time.Second != 0 {
		return 0, fmt.Errorf("%s must be whole seconds, not %s", field, d)
	}
	return int64(d.Duration / time.Second), nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("a duration must be a string such as 5m")
	}
	v, err := ParseDuration(s)
	*d = v
	return err
}
func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
		return fmt.Errorf("line %d: a duration must be a string such as 5m", n.Line)
	}
	v, err := ParseDuration(n.Value)
	*d = v
	return err
}
