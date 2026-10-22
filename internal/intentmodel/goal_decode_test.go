package intentmodel

import (
	"errors"
	"testing"
)

func TestGoalRejectsDuplicateAndEscapedDuplicateFields(t *testing.T) {
	for _, content := range []string{
		`{"outcome":"availability","targetText":"99.5%","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`,
		`{"outcome":"availability","targetText":"99.5%","\u0074argetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`,
		`{"outcome":"latency","outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`,
		`{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":["Clarify the measurement"],"clarifications":[]}`,
	} {
		if _, err := decodeGoal(content); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("accepted ambiguous structured goal: %v", err)
		}
	}
}

func TestGoalRejectsNullOrBlankClarificationItems(t *testing.T) {
	for _, items := range []string{`[null]`, `["Which target?",null]`, `[""]`, `["   "]`} {
		content := `{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":` + items + `}`
		if _, err := decodeGoal(content); !errors.Is(err, ErrUnavailable) {
			t.Errorf("accepted invalid clarification array %s: %v", items, err)
		}
	}
}
