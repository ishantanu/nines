package v1alpha1

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBurnWindowsDurationAndLegacyRoundTrip(t *testing.T) {
	want := BurnRatePolicy{ShortWindowSeconds: 300, LongWindowSeconds: 5400, BurnRate: 6, ForSeconds: 120, MinCoverage: 1}
	for _, input := range []string{
		"shortWindow: 300s\nlongWindow: 1h30m\nburnRate: 6\nforSeconds: 120\nminCoverage: 1\n",
		"shortWindow: 5m\nlongWindow: 90m\nburnRate: 6\nforSeconds: 120\nminCoverage: 1\n",
		"shortWindowSeconds: 300\nlongWindowSeconds: 5400\nburnRate: 6\nforSeconds: 120\nminCoverage: 1\n",
	} {
		var got BurnRatePolicy
		if err := yaml.Unmarshal([]byte(input), &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("decode: %v %+v", err, got)
		}
		canonical, err := yaml.Marshal(got)
		if err != nil || !strings.Contains(string(canonical), "shortWindow: 5m") || strings.Contains(string(canonical), "WindowSeconds:") {
			t.Fatalf("export: %s %v", canonical, err)
		}
		var back BurnRatePolicy
		if err = yaml.Unmarshal(canonical, &back); err != nil || !reflect.DeepEqual(back, want) {
			t.Fatalf("round trip: %v %+v", err, back)
		}
		runtime, err := json.Marshal(got)
		if err != nil || !strings.Contains(string(runtime), `"shortWindowSeconds":300`) || strings.Contains(string(runtime), `"shortWindow":`) {
			t.Fatalf("runtime JSON changed: %s %v", runtime, err)
		}
		if err = json.Unmarshal(runtime, &back); err != nil || !reflect.DeepEqual(back, want) {
			t.Fatalf("stored JSON: %v %+v", err, back)
		}
	}
	var got BurnRatePolicy
	if err := json.Unmarshal([]byte(`{"shortWindow":"5m","longWindow":"1h30m","burnRate":6,"forSeconds":120,"minCoverage":1}`), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("API duration input: %v %+v", err, got)
	}
}
func TestBurnWindowStrictInput(t *testing.T) {
	for _, fields := range []string{
		"shortWindow: 5m\nshortWindowSeconds: 300", "longWindow: 1h\nlongWindowSeconds: 3600",
		"shortWindow: 5m\nshortWindow: 10m", "shortWindows: 5m", "shortWindow: null", "shortWindowSeconds: null",
		"shortWindow: 300", "shortWindowSeconds: 300.5", "forSeconds: 1.5", "shortWindow: 0s", "shortWindow: -1m", "shortWindow: 1ms", "shortWindow: 0.5m", "shortWindow: 1d", "shortWindow: 99999999999999999h", "shortWindow: 1m trailing",
	} {
		t.Run(fields, func(t *testing.T) {
			var got BurnRatePolicy
			if err := yaml.Unmarshal([]byte(fields+"\nburnRate: 6\n"), &got); err == nil {
				t.Fatalf("accepted unsafe policy: %s", fields)
			}
		})
	}
	for _, input := range []string{`{"shortWindow":"5m","shortWindowSeconds":300}`, `{"shortWindow":300}`, `{"shortWindowSeconds":300.0}`, `{"shortWindowSeconds":"300"}`, `{"shortWindow":null}`, `{"typo":1}`, `{"shortWindow":"1m","shortWindow":"2m"}`} {
		var got BurnRatePolicy
		if err := json.Unmarshal([]byte(input), &got); err == nil {
			t.Fatalf("accepted unsafe JSON: %s", input)
		}
	}
}
