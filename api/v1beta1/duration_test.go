package v1beta1

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"28d":     28 * 24 * time.Hour,
		"1h30m":   90 * time.Minute,
		"5m":      5 * time.Minute,
		"15s":     15 * time.Second,
		"300ms":   300 * time.Millisecond,
		"1d12h":   36 * time.Hour,
		"2m500ms": 2*time.Minute + 500*time.Millisecond,
	}
	for text, want := range cases {
		got, err := ParseDuration(text)
		if err != nil || got.Duration != want {
			t.Fatalf("%s = %v, %v; want %v", text, got.Duration, err, want)
		}
		if again, _ := ParseDuration(got.String()); again != got {
			t.Fatalf("%s does not round-trip through %s", text, got)
		}
	}
	for _, bad := range []string{"", "0s", "5", "5x", "-5m", "1.5h", "5 m", "401d", "1w"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestDurationString(t *testing.T) {
	for d, want := range map[time.Duration]string{
		28 * 24 * time.Hour:        "28d",
		90 * time.Minute:           "1h30m",
		300 * time.Millisecond:     "300ms",
		25*time.Hour + time.Second: "1d1h1s",
	} {
		if got := D(d).String(); got != want {
			t.Fatalf("%v = %s, want %s", d, got, want)
		}
	}
}

func TestDurationEncodings(t *testing.T) {
	var v struct {
		Window Duration `json:"window" yaml:"window"`
	}
	if err := yaml.Unmarshal([]byte("window: 28d\n"), &v); err != nil || v.Window.Duration != 28*24*time.Hour {
		t.Fatal(v, err)
	}
	if err := yaml.Unmarshal([]byte("window: 2419200\n"), &v); err == nil {
		t.Fatal("a bare number was accepted as a duration")
	}
	out, _ := json.Marshal(v)
	if string(out) != `{"window":"28d"}` {
		t.Fatal(string(out))
	}
	if err := json.Unmarshal([]byte(`{"window":"6h"}`), &v); err != nil || v.Window.Duration != 6*time.Hour {
		t.Fatal(v, err)
	}
	if _, err := D(300 * time.Millisecond).WholeSeconds("window"); err == nil {
		t.Fatal("a fractional window was accepted")
	}
}
