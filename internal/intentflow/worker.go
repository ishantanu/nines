package intentflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

//go:embed skill.yaml
var skillYAML []byte

type intentToolDefinition struct {
	Description string         `yaml:"description"`
	InputSchema map[string]any `yaml:"inputSchema"`
}
type skillContract struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec struct {
		Definitions   map[string]intentToolDefinition `yaml:"definitions"`
		Instructions  string                          `yaml:"instructions"`
		Tools         []string                        `yaml:"tools"`
		MaxModelCalls int                             `yaml:"maxModelCalls"`
		MaxToolCalls  int                             `yaml:"maxToolCalls"`
	} `yaml:"spec"`
}

var contract = func() skillContract {
	var c skillContract
	if e := yaml.Unmarshal(skillYAML, &c); e != nil {
		panic(e)
	}
	if c.APIVersion != v1.APIVersion || c.Kind != "IntentAuthoringSkill" || c.Metadata.Name != "intent-authoring" || c.Metadata.Version != "0.1.0" || c.Spec.MaxModelCalls != 6 || c.Spec.MaxToolCalls != 6 || len(c.Spec.Tools) != 6 || len(c.Spec.Definitions) != 6 {
		panic("invalid intent skill contract")
	}
	seen := map[string]bool{}
	for _, name := range c.Spec.Tools {
		d, ok := c.Spec.Definitions[name]
		if !ok || seen[name] || d.Description == "" || d.InputSchema["type"] != "object" || d.InputSchema["additionalProperties"] != false {
			panic("invalid intent skill tool schema")
		}
		seen[name] = true
	}
	return c
}()

func bytesDigest(b []byte) string { d := sha256.Sum256(b); return hex.EncodeToString(d[:]) }
func skillRef() v1.ReviewSkillRef {
	return v1.ReviewSkillRef{ID: "intent-authoring", Version: "0.1.0", Digest: bytesDigest(skillYAML)}
}
func (s *Service) Capabilities() any {
	return map[string]any{"apiVersion": v1.APIVersion, "tenant": s.Platform.TenantID, "modelEnabled": s.Model != nil, "draftEnabled": s.Platform.IntentDrafter != nil, "skill": skillRef(), "maxRuns": 8, "maxModelCalls": 6, "maxToolCalls": 6, "tools": contract.Spec.Tools, "appliesResources": false, "reviewedChangeSets": true, "humanApprovalRequired": true, "skillYAML": string(skillYAML)}
}
func decodeArgs(raw json.RawMessage, out any) error {
	if len(raw) > 4096 {
		return rm.Failure("unsafe_tool_call")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return rm.Failure("unsafe_tool_call")
	}
	return nil
}
func toolset(r record) []rm.Tool {
	names := []string{"ask_user"}
	if r.Session.Goal == nil {
		names = append(names, "draft_goal")
	} else if r.Session.Selection == nil {
		names = append(names, "find_measurements")
	} else {
		names = append(names, "compile_intent", "backtest_intent", "finish_proposal")
	}
	tools := []rm.Tool{}
	for _, name := range names {
		d := contract.Spec.Definitions[name]
		schema, _ := json.Marshal(d.InputSchema)
		tools = append(tools, rm.Tool{Name: name, Description: d.Description, Schema: schema})
	}

	return tools
}
func guidedTool(r record) string {
	if r.Session.Goal == nil {
		return "draft_goal"
	}
	if r.Session.Selection == nil {
		return "find_measurements"
	}
	if r.Session.Plan == nil || r.Session.Plan.Compiled == nil {
		return "compile_intent"
	}
	if r.RunRequest.Backtest != nil && r.Session.Backtest == nil {
		return "backtest_intent"
	}
	return "finish_proposal"
}
func fenced(r record, token string, now time.Time) error {
	if !active(r) || r.Token != token || !r.LeaseUntil.After(now) {
		return errLease
	}
	return nil
}
func (s *Service) RunWorker(ctx context.Context, log *slog.Logger) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for ctx.Err() == nil {
		found, e := s.WorkOne(ctx)
		if e != nil && ctx.Err() == nil {
			log.ErrorContext(ctx, "intent preparation step failed", "tenant", s.Platform.TenantID)
		}
		if found && e == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
func (s *Service) WorkOne(ctx context.Context) (bool, error) {
	token := model.NewID()
	r, e := s.Repository.Claim(ctx, token)
	if e == ErrNotFound {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if !active(r) {
		return true, nil
	}
	id := r.Session.ID
	run := current(&r)
	deadline := time.Now().Add(60 * time.Second)
	if run.DeadlineAt.Before(deadline) {
		deadline = run.DeadlineAt
	}
	job, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	job, end := observability.Operation(job, "intent.prepare_step", s.Platform.TenantID)
	var operationErr error
	defer func() { end(operationErr) }()
	// Cancellation fences result publication and interrupts an in-flight provider call.
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-job.Done():
				return
			case <-t.C:
				latest, err := s.Repository.Read(job, id)
				if err != nil || !active(latest) || latest.Token != token {
					cancel()
					return
				}
			}
		}
	}()
	fail := func(code string) (bool, error) {
		_, err := s.Repository.Update(ctx, id, func(next *record, now time.Time) error {
			if err := fenced(*next, token, now); err != nil {
				return err
			}
			finish(next, "failed", code, "Preparation stopped ("+code+"). Definitions and recording rules were not changed.", now)
			return nil
		})
		if errors.Is(err, errLease) {
			return true, nil
		}
		return true, err
	}
	if s.Authorize == nil || r.Session.Tenant != s.Platform.TenantID || s.Authorize(job, run.Actor) != nil {
		return fail("access_denied")
	}
	if run.Skill != skillRef() {
		return fail("skill_changed")
	}
	if run.Mode == "model" && (s.Model == nil || run.ModelRoute == nil || *run.ModelRoute != s.Model.Descriptor()) {
		return fail("model_configuration_changed")
	}
	if run.Mode == "model" && r.Pending == nil {
		req := rm.Request{System: contract.Spec.Instructions, Messages: r.Native, Tools: toolset(r), MaxOutputTokens: 1024}
		body, _ := json.Marshal(req)
		if run.ModelCalls >= contract.Spec.MaxModelCalls || len(body) > 32<<10 || run.ModelInputBytes+len(body) > 128<<10 {
			return fail("model_budget_exceeded")
		}
		r, e = s.Repository.Update(job, id, func(n *record, now time.Time) error {
			if e := fenced(*n, token, now); e != nil {
				return e
			}
			current(n).ModelCalls++
			current(n).ModelInputBytes += len(body)
			touch(n, now)
			return nil
		})
		if e != nil {
			return true, e
		}
		response, err := s.Model.Turn(job, req, func(rm.Progress) error { return job.Err() })
		operationErr = err
		if ctx.Err() != nil {
			return true, ctx.Err()
		}
		if err != nil {
			return fail("model_unavailable")
		}
		if len(response.Calls) != 1 || response.InputTokens < 0 || response.OutputTokens < 0 || response.OutputTokens > 1024 || len(response.Native) > 64<<10 || !json.Valid(response.Native) {
			return fail("unsafe_model_response")
		}
		call := response.Calls[0]
		if call.ID == "" || len(call.ID) > 128 || len(call.Arguments) > 4096 || !slices.ContainsFunc(req.Tools, func(t rm.Tool) bool { return t.Name == call.Name }) {
			return fail("unsafe_tool_call")
		}
		_, e = s.Repository.Update(ctx, id, func(n *record, now time.Time) error {
			if e := fenced(*n, token, now); e != nil {
				return e
			}
			n.Native = append(n.Native, rm.Message{Role: "assistant", Native: response.Native})
			n.Pending = &call
			n.Token = ""
			n.LeaseUntil = time.Time{}
			touch(n, now)
			return nil
		})
		if errors.Is(e, errLease) {
			return true, nil
		}
		return true, e
	}
	if len(run.Steps) >= contract.Spec.MaxToolCalls {
		return fail("tool_budget_exceeded")
	}
	call := rm.Call{Name: guidedTool(r), ID: "guided", Arguments: json.RawMessage(`{}`)}
	if run.Mode == "model" {
		call = *r.Pending
	}
	if !slices.Contains(contract.Spec.Tools, call.Name) {
		return fail("unsafe_tool_call")
	}
	// Reserve a drafting model call before the external operation, including crash recovery.
	if call.Name == "draft_goal" {
		if s.Platform.IntentDrafter == nil {
			if current(&r).DraftModelID != "" {
				return fail("draft_model_configuration_changed")
			}
		} else {
			if current(&r).DraftModelID != s.Platform.IntentDrafter.ModelID() {
				return fail("draft_model_configuration_changed")
			}
			if expected := current(&r).DraftRoute; expected != nil {
				descriptor, ok := s.Platform.IntentDrafter.(interface{ Descriptor() rm.Descriptor })
				if !ok || descriptor.Descriptor() != *expected {
					return fail("draft_model_configuration_changed")
				}
			}
		}
	}

	if call.Name == "draft_goal" && s.Platform.IntentDrafter != nil {
		if r.RunRequest == nil || !r.RunRequest.ModelConsent || run.ModelCalls >= contract.Spec.MaxModelCalls {
			return fail("model_budget_exceeded")
		}
		r, e = s.Repository.Update(job, id, func(n *record, now time.Time) error {
			if e := fenced(*n, token, now); e != nil {
				return e
			}
			current(n).ModelCalls++
			touch(n, now)
			return nil
		})
		if e != nil {
			return true, e
		}
	}
	next := clone(r)
	result, err := s.execute(job, &next, call)
	operationErr = err
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	if err != nil {
		code := "measurement_unavailable"
		var invalid *model.ValidationError
		if errors.As(err, &invalid) {
			code = "invalid_measurement"
		}
		if errors.Is(err, ErrConflict) || errors.Is(err, model.ErrRevisionConflict) {
			code = "revision_conflict"
		}
		var provider *rm.Error
		if errors.As(err, &provider) {
			code = provider.Code
		}
		return fail(code)
	}
	data, _ := json.Marshal(result)
	if len(data) > 64<<10 {
		return fail("evidence_budget_exceeded")
	}
	_, e = s.Repository.Update(ctx, id, func(n *record, now time.Time) error {
		if e := fenced(*n, token, now); e != nil {
			return e
		}
		// The session is immutable while queued/running. Every completed tool and its
		// exact evidence bytes commit atomically with goal/selection/proposal state.
		n.Session.Goal = next.Session.Goal
		n.Session.Plan = next.Session.Plan
		n.Session.Backtest = next.Session.Backtest
		n.Session.Selection = next.Session.Selection
		current(n).Steps = append(current(n).Steps, v1.IntentFlowStep{Tool: call.Name, At: now, Digest: bytesDigest(data), Result: json.RawMessage(data)})
		current(n).UpdatedAt = now
		if !active(next) {
			status := current(&next).Status
			code := current(&next).ErrorCode
			message := ""
			if len(next.Session.Messages) > len(r.Session.Messages) {
				message = next.Session.Messages[len(next.Session.Messages)-1].Text
			}
			finish(n, status, code, message, now)
			return nil
		}
		if current(n).Mode == "model" {
			// The model receives bounded goal/candidate metadata and evidence receipts,
			// never credentials, provider URLs, raw samples or executable YAML.
			compact := modelResult(next, call.Name, result)
			b, _ := json.Marshal(compact)
			if len(b) > 12<<10 {
				b = []byte(`{"status":"bounded_evidence","message":"See the saved evidence and ask the user to narrow the goal."}`)
			}
			n.Native = append(n.Native, rm.Message{Role: "tool", Name: call.Name, CallID: call.ID, ProviderID: call.ProviderID, Result: b})
		}
		n.Pending = nil
		n.Token = ""
		n.LeaseUntil = time.Time{}
		touch(n, now)
		return nil
	})
	if errors.Is(e, errLease) {
		return true, nil
	}
	return true, e
}
func modelResult(r record, name string, result any) any {
	if name == "compile_intent" && r.Session.Plan != nil {
		p := r.Session.Plan
		return map[string]any{"status": p.Status, "intentDigest": p.IntentDigest, "manifestDigest": p.Compiled.Digest, "warnings": p.Warnings, "journeyChange": p.Journey != nil}
	}
	if name == "backtest_intent" && r.Session.Backtest != nil {
		b := r.Session.Backtest
		return map[string]any{"manifestDigest": b.ManifestDigest, "completeCutoffs": b.CompleteCutoffs, "unknownCutoffs": b.UnknownCutoffs, "noTrafficCutoffs": b.NoTrafficCutoffs, "targetViolationCutoffs": b.TargetViolationCutoffs, "burnThresholdCutoffs": b.BurnThresholdCutoffs, "warnings": b.Warnings}
	}
	return result
}
func (s *Service) execute(ctx context.Context, r *record, call rm.Call) (any, error) {
	now := time.Now().UTC()
	switch call.Name {
	case "ask_user":
		var in struct {
			Question string `json:"question"`
		}
		if e := decodeArgs(call.Arguments, &in); e != nil || !text(in.Question, 2000) {
			return nil, rm.Failure("unsafe_tool_call")
		}
		status := "needs_input"
		if r.Session.Plan != nil && r.Session.Selection == nil && len(r.Session.Plan.Candidates) > 0 {
			status = "needs_selection"
		}
		finish(r, status, "", in.Question, now)
		return map[string]any{"status": status, "question": in.Question}, nil
	case "finish_proposal":
		summary := "The SLO and critical user journey proposal is ready for review. No definitions or rules have been applied."
		if current(r).Mode == "model" {
			var in struct {
				Summary string `json:"summary"`
			}
			if e := decodeArgs(call.Arguments, &in); e != nil || !text(in.Summary, 2000) {
				return nil, rm.Failure("unsafe_tool_call")
			}
			summary = in.Summary
		}
		if r.Session.Plan == nil || r.Session.Plan.Status != "ready" || r.Session.Plan.Compiled == nil || r.Session.Selection == nil || (r.RunRequest.Backtest != nil && r.Session.Backtest == nil) {
			return nil, model.Invalid("compile the confirmed measurement and complete requested historical validation first")
		}
		finish(r, "ready", "", summary, now)
		return map[string]any{"status": "ready", "intentDigest": r.Session.Plan.IntentDigest, "manifestDigest": r.Session.Plan.Compiled.Digest, "applied": false}, nil
	default:
		if e := decodeArgs(call.Arguments, &struct{}{}); e != nil {
			return nil, e
		}
	}
	switch call.Name {
	case "draft_goal":
		if s.Platform.IntentDrafter == nil {
			finish(r, "needs_input", "", "No drafting model is configured. Use the definition form to supply a typed intent, or configure a model provider.", now)
			return map[string]string{"status": "needs_input"}, nil
		}
		texts := []string{}
		for _, m := range r.Session.Messages {
			if m.Role == "user" {
				texts = append(texts, m.Text)
			}
		}
		description := strings.Join(texts, "\n")
		if len(description) > 2000 {
			finish(r, "needs_input", "", "Summarize the full goal and answers in a new conversation of at most 2,000 bytes, or use a typed intent.", now)
			return map[string]string{"status": "needs_input"}, nil
		}
		scope := r.Session.Scope
		draft, e := s.Platform.DraftIntent(ctx, v1.DraftIntentInput{Text: description, Metadata: scope.Metadata, Service: scope.Service, Source: scope.Source, Population: scope.Population, NoDataPolicy: scope.NoDataPolicy})
		if e != nil {
			return nil, e
		}
		if draft.Intent != nil {
			draft.Intent.Spec.Journey = scope.Journey
			if !sameScope(scope, *draft.Intent) {
				return nil, model.Invalid("draft changed pinned scope")
			}
			r.Session.Goal = draft.Intent
			r.Session.Selection = nil
			r.Session.Plan = nil
		} else {
			finish(r, "needs_input", "", strings.Join(draft.Clarifications, "\n"), now)
		}
		return draft, nil
	case "find_measurements":
		if r.Session.Goal == nil {
			return nil, model.Invalid("draft a typed goal first")
		}
		plan, e := s.Platform.PlanIntent(ctx, v1.PlanIntentInput{Intent: *r.Session.Goal})
		if e != nil {
			return nil, e
		}
		r.Session.Plan = &plan
		if current(r).Mode == "guided" {
			state := "needs_input"
			message := strings.Join(plan.Clarifications, "\n")
			if len(plan.Candidates) > 0 {
				state = "needs_selection"
				message = "Review the candidate observation point, success labels and retry semantics, then explicitly confirm one binding revision."
			}
			finish(r, state, "", message, now)
		}
		return plan, nil
	case "compile_intent":
		if r.Session.Goal == nil || r.Session.Selection == nil || hash(*r.Session.Goal) != r.Session.Selection.IntentDigest {
			return nil, model.Invalid("human binding confirmation is required")
		}
		plan, e := s.Platform.PlanIntent(ctx, v1.PlanIntentInput{Intent: *r.Session.Goal, Binding: &r.Session.Selection.Binding})
		if e != nil {
			return nil, e
		}
		r.Session.Plan = &plan
		return plan, nil
	case "backtest_intent":
		b := r.RunRequest.Backtest
		if b == nil || r.Session.Plan == nil || r.Session.Plan.Compiled == nil || r.Session.Selection == nil {
			return nil, model.Invalid("provide a historical window and compile the confirmed measurement first")
		}
		backtest, e := s.Platform.BacktestIntent(ctx, v1.IntentBacktestInput{Plan: v1.PlanIntentInput{Intent: *r.Session.Goal, Binding: &r.Session.Selection.Binding}, From: b.From, To: b.To, ResolutionSeconds: b.ResolutionSeconds})
		if e != nil {
			return nil, e
		}
		r.Session.Backtest = &backtest
		return backtest, nil
	}
	return nil, rm.Failure("unsafe_tool_call")
}
