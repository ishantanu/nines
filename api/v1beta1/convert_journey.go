package v1beta1

import (
	"fmt"
	"time"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

// SLOLookup returns a linked SLO's stored spec, or false when it is unknown.
type SLOLookup func(id string) (v1alpha1.SLOSpec, bool)

// Dimension is the journey dimension an SLO measures. typed is false when the
// SLO has no measurement type (custom queries or a binding), so its dimension
// is only a default and any explicit dimension is accepted.
func Dimension(s v1alpha1.SLOSpec) (dimension string, typed bool) {
	if s.Measurement == nil {
		return "availability", false
	}
	switch s.Measurement.Type {
	case "latency", "freshness":
		return "latency", true
	case "correctness", "durability", "coverage":
		return "correctness", true
	default:
		// Availability and sessions count good events.
		return "availability", true
	}
}

// explicitOnly are dimensions no SLO type expresses.
var explicitOnly = map[string]bool{"cost": true, "safety": true}

// ToHub converts a journey to the stored v1alpha1 form. A link without a
// dimension takes its SLO's; an explicit dimension other than cost or safety
// must match its SLO's type. Without a lookup (offline validation), omitted
// dimensions are assumed to be availability and reported in the notes.
func (j CriticalUserJourney) ToHub(lookup SLOLookup) (v1alpha1.CriticalUserJourney, []string, error) {
	var notes []string
	if j.APIVersion != APIVersion || j.Kind != JourneyKind {
		return v1alpha1.CriticalUserJourney{}, nil, fmt.Errorf("expected apiVersion %s and kind %s", APIVersion, JourneyKind)
	}
	in := j.Spec
	out := v1alpha1.JourneySpec{DisplayName: in.DisplayName, Description: in.Description, Owner: in.Owner, Criticality: in.Criticality, Steps: in.Steps}
	for _, l := range in.SLOs {
		link := v1alpha1.JourneySLO{ID: l.ID, Dimension: l.Dimension, Required: l.Required}
		var spec v1alpha1.SLOSpec
		known := false
		if lookup != nil {
			spec, known = lookup(l.ID)
		}
		derived, typed := Dimension(spec)
		switch {
		case explicitOnly[l.Dimension]:
		case known && l.Dimension == "":
			link.Dimension = derived
		case known && typed && l.Dimension != derived:
			return v1alpha1.CriticalUserJourney{}, nil, fmt.Errorf("slos %s: dimension %s contradicts the SLO, which measures %s; omit dimension to use the SLO's", l.ID, l.Dimension, derived)
		case l.Dimension == "":
			link.Dimension = "availability"
			notes = append(notes, fmt.Sprintf("slos %s: dimension assumed to be availability; the server sets it from the SLO's type", l.ID))
		}
		out.SLOs = append(out.SLOs, link)
	}
	linked := map[string]bool{}
	for _, l := range in.SLOs {
		linked[l.ID] = true
	}
	for _, step := range in.Steps {
		for _, ref := range step.SLORefs {
			if !linked[ref] {
				return v1alpha1.CriticalUserJourney{}, nil, fmt.Errorf("step %s refers to SLO %s, which is not in slos; link it there first", step.ID, ref)
			}
		}
	}
	if a := in.Agentic; a != nil {
		out.Agentic = &v1alpha1.AgenticContract{SuccessDefinition: a.SuccessDefinition, MaxCostUSD: a.MaxCostUSD, MaxSteps: a.MaxSteps, ApprovalRequired: a.ApprovalRequired}
		if a.MaxDuration != nil {
			s, err := a.MaxDuration.WholeSeconds("agentic.maxDuration")
			if err != nil {
				return v1alpha1.CriticalUserJourney{}, nil, err
			}
			out.Agentic.MaxDurationSeconds = s
		}
	}
	if r := in.RunObjective; r != nil {
		s, err := r.Window.WholeSeconds("runObjective.window")
		if err != nil {
			return v1alpha1.CriticalUserJourney{}, nil, err
		}
		out.RunObjective = &v1alpha1.RunObjective{Target: r.Target, WindowSeconds: s, MinRuns: r.MinRuns}
	}
	return v1alpha1.NewJourney(j.Metadata.Name, j.Metadata.Project, out), notes, nil
}

// JourneyFromHub converts a stored journey for export. A dimension equal to
// the one its SLO implies is left out; any other is kept, so nothing changes
// meaning on the way through.
func JourneyFromHub(h v1alpha1.CriticalUserJourney, lookup SLOLookup) CriticalUserJourney {
	in := h.Spec
	out := JourneySpec{DisplayName: in.DisplayName, Description: in.Description, Owner: in.Owner, Criticality: in.Criticality, Steps: in.Steps}
	for _, l := range in.SLOs {
		link := JourneySLO{ID: l.ID, Dimension: l.Dimension, Required: l.Required}
		if lookup != nil {
			if spec, ok := lookup(l.ID); ok {
				if derived, _ := Dimension(spec); derived == l.Dimension {
					link.Dimension = ""
				}
			}
		}
		out.SLOs = append(out.SLOs, link)
	}
	if a := in.Agentic; a != nil {
		out.Agentic = &AgenticContract{SuccessDefinition: a.SuccessDefinition, MaxCostUSD: a.MaxCostUSD, MaxSteps: a.MaxSteps, ApprovalRequired: a.ApprovalRequired}
		if a.MaxDurationSeconds > 0 {
			out.Agentic.MaxDuration = dur(a.MaxDurationSeconds)
		}
	}
	if r := in.RunObjective; r != nil {
		out.RunObjective = &RunObjective{Target: r.Target, Window: D(time.Duration(r.WindowSeconds) * time.Second), MinRuns: r.MinRuns}
	}
	return CriticalUserJourney{APIVersion: APIVersion, Kind: JourneyKind, Metadata: h.Metadata, Spec: out}
}
