package service

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

var ErrIntentModel = errors.New("intent model is unavailable, refused or returned an invalid draft; review configuration or clarify the goal")

type IntentDrafter interface {
	Draft(context.Context, string) (v1.ModelGoal, error)
	ModelID() string
}

var goalPercent = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*(?:%|percent)\s*$`)
var goalDuration = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?|one|two|three|four|five|six|seven|eight|nine|ten)\s*(ms|milliseconds?|s|seconds?|m|minutes?|h|hours?|d|days?|w|weeks?)\s*$`)

func phrase(text string, p *string) (string, bool) {
	if p == nil || strings.TrimSpace(*p) == "" || !strings.Contains(text, *p) {
		return "", false
	}
	return *p, true
}
func durationPhrase(text string) (float64, bool) {
	m := goalDuration.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	number, e := strconv.ParseFloat(m[1], 64)
	if e != nil {
		number = map[string]float64{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10}[strings.ToLower(m[1])]
	}
	unit := strings.ToLower(m[2])
	factor := float64(1)
	switch {
	case unit == "ms" || strings.HasPrefix(unit, "millisecond"):
		factor = .001
	case unit == "m" || strings.HasPrefix(unit, "minute"):
		factor = 60
	case unit == "h" || strings.HasPrefix(unit, "hour"):
		factor = 3600
	case unit == "d" || strings.HasPrefix(unit, "day"):
		factor = 86400
	case unit == "w" || strings.HasPrefix(unit, "week"):
		factor = 604800
	}
	return number * factor, number > 0 && model.Finite(number*factor)
}
func (s *Service) DraftIntent(ctx context.Context, in v1.DraftIntentInput) (out v1.IntentDraft, err error) {
	ctx, finish := observability.Operation(ctx, "intent.draft", s.TenantID)
	defer func() { finish(err) }()
	out = v1.IntentDraft{APIVersion: v1.APIVersion, Tenant: s.TenantID, Status: "needs_clarification", Clarifications: []string{}, Warnings: []string{"This is a model-assisted draft, not an approved measurement. Review all typed fields and confirm the SLO measurement before planning.", "Only the entered description is sent to the configured model. Numeric fields are parsed from exact phrases; no catalog, provider credentials or telemetry are sent."}}
	if len(in.Text) > 2000 || strings.TrimSpace(in.Text) == "" {
		return out, model.Invalid("provide a goal description of 1–2000 bytes")
	}
	skeleton := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: in.Metadata, Spec: v1.IntentSpec{Description: in.Text, Service: in.Service, Source: in.Source, Population: in.Population, NoDataPolicy: in.NoDataPolicy, Outcome: "availability", Target: .5, WindowSeconds: 60}}
	if err := model.ValidateIntent(skeleton); err != nil {
		return out, err
	}
	if _, err := s.telemetrySource(in.Source); err != nil {
		return out, err
	}
	if s.IntentDrafter == nil {
		return out, model.ErrUnavailable
	}
	out.Model = s.IntentDrafter.ModelID()
	goal, err := s.IntentDrafter.Draft(ctx, in.Text)
	if err != nil {
		return out, ErrIntentModel
	}
	out.Clarifications = append(out.Clarifications, goal.Clarifications...)
	if goal.Outcome != "availability" && goal.Outcome != "latency" {
		out.Clarifications = append(out.Clarifications, "This goal requires a separate instrumentation contract. Choose availability or latency; a joint condition cannot be inferred from two ratios.")
		return out, nil
	}
	skeleton.Spec.Outcome = goal.Outcome
	target, ok := phrase(in.Text, goal.TargetText)
	m := goalPercent.FindStringSubmatch(target)
	if !ok || m == nil {
		out.Clarifications = append(out.Clarifications, "State the target as an explicit percentage, for example 99.9%.")
	} else {
		n, _ := strconv.ParseFloat(m[1], 64)
		normalized := strconv.FormatFloat(n/100, 'g', 12, 64)
		skeleton.Spec.Target, _ = strconv.ParseFloat(normalized, 64)
	}
	window, ok := phrase(in.Text, goal.WindowText)
	seconds, valid := durationPhrase(window)
	if !ok || !valid || seconds != float64(int64(seconds)) {
		out.Clarifications = append(out.Clarifications, "State an explicit rolling duration, for example 28 days, rather than a calendar month.")
	} else {
		skeleton.Spec.WindowSeconds = int64(seconds)
	}
	if goal.Outcome == "latency" {
		threshold, ok := phrase(in.Text, goal.ThresholdText)
		n, valid := durationPhrase(threshold)
		if !ok || !valid {
			out.Clarifications = append(out.Clarifications, "State the maximum attempt duration, for example 300 milliseconds.")
		} else {
			skeleton.Spec.ThresholdSeconds = &n
		}
	} else if goal.ThresholdText != nil {
		out.Clarifications = append(out.Clarifications, "Availability and a duration condition need a joint measurement contract; choose one outcome.")
	}
	if len(out.Clarifications) > 0 {
		return out, nil
	}
	if err := model.ValidateIntent(skeleton); err != nil {
		out.Clarifications = append(out.Clarifications, err.Error())
		return out, nil
	}
	out.Intent = &skeleton
	out.Status = "draft"
	return out, nil
}
