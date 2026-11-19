package model

import (
	"strings"
	"testing"
)

func TestValidateSLOLabelsBoundsShapeAndCount(t *testing.T) {
	for name, labels := range map[string]map[string]string{
		"none":     nil,
		"typical":  {"team": "payments", "feature_area": "Cloud Storage", "product.area": "core/infra"},
		"boundary": {strings.Repeat("a", 32): strings.Repeat("v", 63)},
	} {
		if err := ValidateSLOLabels(labels); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}
	tooMany := map[string]string{}
	for _, k := range "abcdefghi" {
		tooMany["k"+string(k)] = "v"
	}
	for name, labels := range map[string]map[string]string{
		"too many":        tooMany,
		"uppercase key":   {"Team": "x"},
		"digit key start": {"1team": "x"},
		"long key":        {strings.Repeat("a", 33): "x"},
		"empty value":     {"team": ""},
		"long value":      {"team": strings.Repeat("v", 64)},
		"punctuation":     {"team": "a;b"},
		"leading space":   {"team": " a"},
		"newline":         {"team": "a\nb"},
	} {
		if err := ValidateSLOLabels(labels); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestLabelsRoundTripThroughTheDefinitionWithoutChangingMeasurement(t *testing.T) {
	s := SLO{ID: "api", Project: "p", Service: "s", Name: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60, Labels: map[string]string{"team": "payments"}}
	back := DefinitionFromSLO(s).SLO()
	if back.Labels["team"] != "payments" {
		t.Fatalf("labels lost: %+v", back.Labels)
	}
}
