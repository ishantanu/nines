package model

import (
	"strings"
	"time"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

const JourneyAPIVersion = APIVersion

type Journey v1alpha1.CriticalUserJourney
type JourneyMetadata = ResourceMetadata
type JourneySpec = v1alpha1.JourneySpec
type JourneySLO = v1alpha1.JourneySLO
type JourneyStep = v1alpha1.JourneyStep
type AgenticContract = v1alpha1.AgenticContract

type JourneyRevision struct {
	Manifest  Journey   `json:"manifest"`
	Revision  int64     `json:"revision"`
	Digest    string    `json:"digest"`
	AppliedAt time.Time `json:"appliedAt"`
}

// ParseJourney accepts one strict YAML or JSON document. KnownFields prevents
// silently accepting misspelled contract fields. yaml.v3 rejects duplicate keys.
func ParseJourney(body []byte) (Journey, error) {
	var j Journey
	if err := decodeDefinition(body, &j); err != nil {
		return j, err
	}
	return j, j.Validate()
}
func JourneyDigest(j Journey) string   { return DefinitionDigest(j) }
func validText(s string, max int) bool { return strings.TrimSpace(s) != "" && len(s) <= max }
func oneOf(s string, allowed ...string) bool {
	for _, v := range allowed {
		if s == v {
			return true
		}
	}
	return false
}
func (j Journey) Validate() error {
	if j.APIVersion != JourneyAPIVersion || j.Kind != "CriticalUserJourney" {
		return Invalid("expected apiVersion %s and kind CriticalUserJourney", JourneyAPIVersion)
	}
	if !identifier.MatchString(j.Metadata.Name) || !validText(j.Metadata.Project, 200) {
		return Invalid("journey name must be a valid identifier and project is required")
	}
	if !validText(j.Spec.DisplayName, 200) || !validText(j.Spec.Owner, 200) || len(j.Spec.Description) > 4000 {
		return Invalid("displayName and owner are required; description is limited to 4000 bytes")
	}
	if !oneOf(j.Spec.Criticality, "critical", "high", "medium") {
		return Invalid("criticality must be critical, high or medium")
	}
	if len(j.Spec.SLOs) < 1 || len(j.Spec.SLOs) > 100 || len(j.Spec.Steps) < 1 || len(j.Spec.Steps) > 100 {
		return Invalid("provide 1–100 SLO links and 1–100 steps")
	}
	links := map[string]bool{}
	required := false
	for _, l := range j.Spec.SLOs {
		if !identifier.MatchString(l.ID) || links[l.ID] || !oneOf(l.Dimension, "availability", "latency", "correctness", "safety", "cost") {
			return Invalid("SLO links need unique valid IDs and a supported dimension")
		}
		links[l.ID] = true
		required = required || l.Required
	}
	if !required {
		return Invalid("at least one SLO link must be required")
	}
	steps := map[string]JourneyStep{}
	used := map[string]bool{}
	for _, step := range j.Spec.Steps {
		if _, exists := steps[step.ID]; exists || !identifier.MatchString(step.ID) || !validText(step.Name, 200) || !oneOf(step.Actor, "user", "service", "agent", "human") {
			return Invalid("steps need unique valid IDs, names and actors (user, service, agent, human)")
		}
		if len(step.SLORefs) < 1 || len(step.SLORefs) > 100 || len(step.DependsOn) > 100 {
			return Invalid("each step needs 1–100 SLO references and at most 100 dependencies")
		}
		seen := map[string]bool{}
		for _, ref := range step.SLORefs {
			if !links[ref] || seen[ref] {
				return Invalid("step %s has an undeclared or duplicate SLO reference %s", step.ID, ref)
			}
			seen[ref] = true
			used[ref] = true
		}
		steps[step.ID] = step
	}
	for ref := range links {
		if !used[ref] {
			return Invalid("SLO %s must be linked to at least one step", ref)
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return Invalid("step dependency cycle at %s", id)
		}
		if state[id] == 2 {
			return nil
		}
		step, ok := steps[id]
		if !ok {
			return Invalid("unknown step dependency %s", id)
		}
		state[id] = 1
		seen := map[string]bool{}
		for _, dep := range step.DependsOn {
			if seen[dep] {
				return Invalid("duplicate dependency %s", dep)
			}
			seen[dep] = true
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range steps {
		if err := visit(id); err != nil {
			return err
		}
	}
	if a := j.Spec.Agentic; a != nil {
		if !validText(a.SuccessDefinition, 2000) || a.MaxDurationSeconds < 0 || a.MaxDurationSeconds > 7*86400 || a.MaxSteps < 0 || a.MaxSteps > 100000 {
			return Invalid("agentic successDefinition is required; duration must be 0–7 days and steps 0–100000")
		}
		if a.MaxCostUSD != nil && (!Finite(*a.MaxCostUSD) || *a.MaxCostUSD < 0 || *a.MaxCostUSD > 1e6) {
			return Invalid("maxCostUSD must be finite and between 0 and 1000000")
		}
	}
	return nil
}

// RunEvidence is reported by the caller. These fields record observations;
// approvalRequired is an evaluated contract, not an execution authorization.
type RunEvidence struct {
	ID               string    `json:"id"`
	JourneyRevision  int64     `json:"journeyRevision"`
	StartedAt        time.Time `json:"startedAt"`
	FinishedAt       time.Time `json:"finishedAt"`
	Outcome          string    `json:"outcome"`
	CostUSD          *float64  `json:"costUSD,omitempty"`
	Steps            *int      `json:"steps,omitempty"`
	ApprovalObserved *bool     `json:"approvalObserved,omitempty"`
	PolicyViolations []string  `json:"policyViolations,omitempty"`
}
type JourneyRun struct {
	Evidence       RunEvidence `json:"evidence"`
	JourneyID      string      `json:"journeyId"`
	ContractDigest string      `json:"contractDigest"`
	Verdict        string      `json:"verdict"`
	Findings       []string    `json:"findings"`
	ReceivedAt     time.Time   `json:"receivedAt"`
}

func EvaluateRun(id string, rev JourneyRevision, e RunEvidence, now time.Time) (JourneyRun, error) {
	r := JourneyRun{Evidence: e, JourneyID: id, ContractDigest: rev.Digest, ReceivedAt: now.UTC(), Findings: []string{}}
	if !identifier.MatchString(e.ID) || e.JourneyRevision != rev.Revision || e.StartedAt.IsZero() || e.FinishedAt.Before(e.StartedAt) || e.FinishedAt.After(now) || now.Sub(e.FinishedAt) > 31*24*time.Hour || e.FinishedAt.Sub(e.StartedAt) > 7*24*time.Hour {
		return r, Invalid("run requires a valid ID, exact revision, ordered timestamps ending within the last 31 days, and duration at most 7 days")
	}
	if !oneOf(e.Outcome, "success", "failure", "handoff", "unknown") {
		return r, Invalid("outcome must be success, failure, handoff or unknown")
	}
	if e.CostUSD != nil && (!Finite(*e.CostUSD) || *e.CostUSD < 0 || *e.CostUSD > 1e6) {
		return r, Invalid("costUSD must be finite, 0–1000000")
	}
	if e.Steps != nil && (*e.Steps < 0 || *e.Steps > 100000) {
		return r, Invalid("steps must be 0–100000")
	}
	if len(e.PolicyViolations) > 50 {
		return r, Invalid("at most 50 policy violations")
	}
	for _, v := range e.PolicyViolations {
		if !validText(v, 500) {
			return r, Invalid("policy violation must contain 1–500 bytes")
		}
	}
	a := rev.Manifest.Spec.Agentic
	if a == nil {
		return r, Invalid("journey revision has no agentic contract")
	}
	violated, incomplete := false, false
	if len(e.PolicyViolations) > 0 {
		violated = true
		r.Findings = append(r.Findings, e.PolicyViolations...)
	}
	if a.MaxDurationSeconds > 0 && e.FinishedAt.Sub(e.StartedAt) > time.Duration(a.MaxDurationSeconds)*time.Second {
		violated = true
		r.Findings = append(r.Findings, "duration limit exceeded")
	}
	if a.MaxCostUSD != nil {
		if e.CostUSD == nil {
			incomplete = true
			r.Findings = append(r.Findings, "cost evidence missing")
		} else if *e.CostUSD > *a.MaxCostUSD {
			violated = true
			r.Findings = append(r.Findings, "cost limit exceeded")
		}
	}
	if a.MaxSteps > 0 {
		if e.Steps == nil {
			incomplete = true
			r.Findings = append(r.Findings, "step count missing")
		} else if *e.Steps > a.MaxSteps {
			violated = true
			r.Findings = append(r.Findings, "step limit exceeded")
		}
	}
	if a.ApprovalRequired {
		if e.ApprovalObserved == nil {
			incomplete = true
			r.Findings = append(r.Findings, "approval evidence missing")
		} else if !*e.ApprovalObserved {
			violated = true
			r.Findings = append(r.Findings, "required approval not observed")
		}
	}
	switch {
	case violated:
		r.Verdict = "violated"
	case e.Outcome == "failure":
		r.Verdict = "failed"
	case incomplete:
		r.Verdict = "incomplete"
	case e.Outcome != "success":
		r.Verdict = "inconclusive"
	default:
		r.Verdict = "passed"
	}
	return r, nil
}
