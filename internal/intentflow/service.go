package intentflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/tenancy"
)

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type Service struct {
	Platform   *service.Service
	Repository Repository
	Model      rm.Model
	Authorize  func(context.Context, v1.InvestigationActor) error
}

func New(p *service.Service, r Repository) *Service { return &Service{Platform: p, Repository: r} }
func text(s string, max int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && !strings.ContainsRune(s, 0) && len(s) <= max
}
func hash(v any) string { return model.DefinitionDigest(v) }
func actor(ctx context.Context, tenant string) (v1.InvestigationActor, error) {
	p, ok := tenancy.FromContext(ctx)
	if !ok || p.TenantID != tenant || p.Subject == "" || (p.Role != "owner" && p.Role != "editor" && p.Role != "agent") {
		return v1.InvestigationActor{}, ErrForbidden
	}
	return v1.InvestigationActor{Subject: p.Subject, Name: p.Name, Kind: p.Kind}, nil
}
func scopeGoal(scope v1.IntentAuthoringScope, description string) v1.SLOIntent {
	return v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: scope.Metadata, Spec: v1.IntentSpec{Description: description, Service: scope.Service, Source: scope.Source, Population: scope.Population, NoDataPolicy: scope.NoDataPolicy, Journey: scope.Journey, Outcome: "availability", Target: .5, WindowSeconds: 60}}
}
func sameScope(scope v1.IntentAuthoringScope, g v1.SLOIntent) bool {
	return hash(struct {
		Metadata                v1.Metadata
		Service, Source, NoData string
		Population              map[string]string
		Journey                 *v1.IntentJourney
	}{g.Metadata, g.Spec.Service, g.Spec.Source, g.Spec.NoDataPolicy, g.Spec.Population, g.Spec.Journey}) == hash(struct {
		Metadata                v1.Metadata
		Service, Source, NoData string
		Population              map[string]string
		Journey                 *v1.IntentJourney
	}{scope.Metadata, scope.Service, scope.Source, scope.NoDataPolicy, scope.Population, scope.Journey})
}
func (s *Service) Create(ctx context.Context, in v1.CreateIntentConversation) (v1.IntentConversation, error) {
	a, e := actor(ctx, s.Platform.TenantID)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	if (!identifier.MatchString(in.ID) || in.ID == "capabilities") || !text(in.Title, 160) || !text(in.Text, 2000) {
		return v1.IntentConversation{}, model.Invalid("provide a conversation ID, title and goal description")
	}
	if e = model.ValidateIntent(scopeGoal(in.Scope, in.Text)); e != nil {
		return v1.IntentConversation{}, e
	}
	if e = s.Platform.ValidateSource(model.SLO{Source: in.Scope.Source}); e != nil {
		return v1.IntentConversation{}, e
	}
	if in.Goal != nil {
		if e = model.ValidateIntent(*in.Goal); e != nil {
			return v1.IntentConversation{}, e
		}
		if !sameScope(in.Scope, *in.Goal) {
			return v1.IntentConversation{}, model.Invalid("typed goal must match the immutable conversation scope")
		}
	}
	now := time.Now().UTC()
	r := record{Session: v1.IntentConversation{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, IntentConversationSummary: v1.IntentConversationSummary{ID: in.ID, Title: in.Title, Version: 1, State: "draft", CreatedAt: now, UpdatedAt: now, Scope: in.Scope}, CreatedBy: a, Goal: clone(in.Goal), Runs: []v1.IntentFlowRun{}, Messages: []v1.IntentConversationMessage{{ID: "initial", Role: "user", Text: in.Text, At: now, Actor: a}}}, RequestDigest: hash(struct {
		Input v1.CreateIntentConversation
		Actor v1.InvestigationActor
	}{in, a})}
	saved, e := s.Repository.Create(ctx, r)
	return saved.Session, e
}
func (s *Service) Read(ctx context.Context, id string) (v1.IntentConversation, error) {
	if !identifier.MatchString(id) {
		return v1.IntentConversation{}, model.Invalid("invalid conversation ID")
	}
	r, e := s.Repository.Read(ctx, id)
	if e == nil && r.Session.Tenant != s.Platform.TenantID {
		return v1.IntentConversation{}, ErrNotFound
	}
	return clone(r.Session), e
}
func (s *Service) List(ctx context.Context, cursor string) (v1.IntentConversationList, error) {
	out := v1.IntentConversationList{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Items: []v1.IntentConversationSummary{}}
	after := ""
	if cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.Split(string(b), "\x00")
		if e != nil || len(cursor) > 300 || len(parts) != 2 || parts[0] != s.Platform.TenantID || !identifier.MatchString(parts[1]) {
			return out, model.Invalid("invalid scoped cursor")
		}
		after = parts[1]
	}
	rows, e := s.Repository.List(ctx, after, 51)
	if e != nil {
		return out, e
	}
	if len(rows) > 50 {
		rows = rows[:50]
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(s.Platform.TenantID + "\x00" + rows[49].ID))
	}
	out.Items = rows
	return out, nil
}
func (s *Service) Append(ctx context.Context, id string, in v1.AppendIntentConversation) (v1.IntentConversation, error) {
	a, e := actor(ctx, s.Platform.TenantID)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	if !identifier.MatchString(id) || !identifier.MatchString(in.ID) || !text(in.Text, 2000) || in.ExpectedVersion < 1 {
		return v1.IntentConversation{}, model.Invalid("message needs a valid ID, expectedVersion and text")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		for _, m := range r.Session.Messages {
			if m.ID == in.ID {
				if m.Role != "user" || m.Text != in.Text || m.Actor.Subject != a.Subject || m.Actor.Kind != a.Kind || r.MessageDigests[in.ID] != hash(struct {
					Text string
					Goal *v1.SLOIntent
				}{in.Text, in.Goal}) {
					return ErrConflict
				}
				return nil
			}
		}
		if active(*r) || r.Session.Version != in.ExpectedVersion {
			return ErrConflict
		}
		if len(r.Session.Messages) >= 40 {
			return ErrLimit
		}
		if in.Goal != nil {
			if e := model.ValidateIntent(*in.Goal); e != nil {
				return e
			}
			if !sameScope(r.Session.Scope, *in.Goal) {
				return model.Invalid("typed goal cannot alter conversation scope")
			}
		}
		r.Session.Messages = append(r.Session.Messages, v1.IntentConversationMessage{ID: in.ID, Role: "user", Text: in.Text, At: now, Actor: a})
		if r.MessageDigests == nil {
			r.MessageDigests = map[string]string{}
		}
		r.MessageDigests[in.ID] = hash(struct {
			Text string
			Goal *v1.SLOIntent
		}{in.Text, in.Goal})
		// A clarification or goal edit invalidates previous selection/proposal evidence.
		invalidateChangeSet(r)
		r.Session.Goal = clone(in.Goal)
		r.Session.Selection = nil
		r.Session.Plan = nil
		r.Session.Backtest = nil
		r.Session.State = "draft"
		touch(r, now)
		return nil
	})
	return r.Session, e
}
func (s *Service) Select(ctx context.Context, id string, in v1.ConfirmIntentBinding) (v1.IntentConversation, error) {
	a, e := actor(ctx, s.Platform.TenantID)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	p, _ := tenancy.FromContext(ctx)
	if p.Role != "owner" && p.Role != "editor" {
		return v1.IntentConversation{}, ErrForbidden
	}
	if !identifier.MatchString(id) || !in.Confirm || in.ExpectedVersion < 1 || !identifier.MatchString(in.Binding.Name) || in.Binding.ExpectedRevision < 1 {
		return v1.IntentConversation{}, model.Invalid("confirm the exact binding and conversation revision")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		if active(*r) || r.Session.Version != in.ExpectedVersion {
			return ErrConflict
		}
		if r.Session.Goal == nil || r.Session.Plan == nil || hash(*r.Session.Goal) != in.IntentDigest || r.Session.Plan.IntentDigest != in.IntentDigest {
			return ErrConflict
		}
		found := false
		for _, c := range r.Session.Plan.Candidates {
			if c.Binding.Name == in.Binding.Name && c.Binding.Revision == in.Binding.ExpectedRevision {
				found = true
			}
		}
		if !found {
			return model.Invalid("select a candidate from the saved plan after reviewing its observation point and success semantics")
		}
		invalidateChangeSet(r)
		r.Session.Selection = &v1.IntentBindingConfirmation{IntentDigest: in.IntentDigest, Binding: in.Binding, Actor: a, At: now}
		r.Session.State = "draft"
		r.Session.Backtest = nil
		touch(r, now)
		return nil
	})
	return r.Session, e
}
func (s *Service) Start(ctx context.Context, id string, in v1.CreateIntentFlowRun) (v1.IntentConversation, error) {
	a, e := actor(ctx, s.Platform.TenantID)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	if !identifier.MatchString(id) || !identifier.MatchString(in.ID) || in.ExpectedVersion < 1 || (in.Mode != "guided" && in.Mode != "model") {
		return v1.IntentConversation{}, model.Invalid("run needs an ID, expectedVersion and guided or model mode")
	}
	if in.Mode == "model" && (s.Model == nil || !in.ModelConsent) {
		return v1.IntentConversation{}, model.Invalid("model mode requires a configured model and consent to send goal and measurement metadata")
	}
	if b := in.Backtest; b != nil {
		if b.From.IsZero() || !b.To.After(b.From) || b.To.Sub(b.From) > 72*time.Hour || b.ResolutionSeconds < 1 || b.ResolutionSeconds > 3600 || b.To.Sub(b.From)%time.Duration(b.ResolutionSeconds*int64(time.Second)) != 0 || b.To.Sub(b.From)/time.Duration(b.ResolutionSeconds*int64(time.Second)) > 240 {
			return v1.IntentConversation{}, model.Invalid("provide a bounded backtest window with at most 240 cutoffs over 72 hours")
		}
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		for _, old := range r.Session.Runs {
			if old.ID == in.ID {
				if r.RunRequest == nil || hash(*r.RunRequest) != hash(in) || old.Actor.Subject != a.Subject || old.Actor.Kind != a.Kind {
					return ErrConflict
				}
				return nil
			}
		}
		if active(*r) || r.Session.Version != in.ExpectedVersion {
			return ErrConflict
		}
		body, _ := json.Marshal(r)
		if len(body) > 1<<20 || len(r.Session.Runs) >= 8 || len(r.Session.Messages) >= 38 {
			return ErrLimit
		}
		if r.Session.Goal == nil && s.Platform.IntentDrafter != nil && !in.ModelConsent {
			return model.Invalid("consent is required before sending the goal to the configured drafting model")
		}
		run := v1.IntentFlowRun{ID: in.ID, Mode: in.Mode, Status: "queued", Actor: a, CreatedAt: now, UpdatedAt: now, DeadlineAt: now.Add(5 * time.Minute), Skill: skillRef(), Steps: []v1.IntentFlowStep{}}
		if drafter := s.Platform.IntentDrafter; drafter != nil {
			run.DraftModelID = drafter.ModelID()
			if descriptor, ok := drafter.(interface{ Descriptor() rm.Descriptor }); ok {
				route := descriptor.Descriptor()
				run.DraftRoute = &route
			}
		}

		if in.Mode == "model" {
			route := s.Model.Descriptor()
			run.ModelRoute = &route
		}
		invalidateChangeSet(r)
		r.Session.Runs = append(r.Session.Runs, run)
		r.Session.State = "queued"
		r.Session.Plan = nil
		r.Session.Backtest = nil
		r.Token = ""
		r.LeaseUntil = time.Time{}
		r.Pending = nil
		r.RunRequest = &in
		r.InputDigest = hash(struct {
			Scope     v1.IntentAuthoringScope
			Goal      *v1.SLOIntent
			Selection *v1.IntentBindingConfirmation
			Messages  []v1.IntentConversationMessage
		}{r.Session.Scope, r.Session.Goal, r.Session.Selection, r.Session.Messages})
		current(r).InputDigest = r.InputDigest
		current(r).ModelConsent = in.ModelConsent
		messages := r.Session.Messages
		if len(messages) > 8 {
			messages = messages[len(messages)-8:]
		}
		history := []map[string]string{}
		for _, m := range messages {
			history = append(history, map[string]string{"role": m.Role, "text": m.Text})
		}
		var selection any
		if r.Session.Selection != nil {
			selection = map[string]any{"binding": r.Session.Selection.Binding, "intentDigest": r.Session.Selection.IntentDigest, "confirmed": true}
		}
		contextBody, _ := json.Marshal(map[string]any{"scope": r.Session.Scope, "goal": r.Session.Goal, "selection": selection, "conversation": history, "backtest": in.Backtest})
		r.Native = []rm.Message{{Role: "user", Text: string(contextBody)}}
		touch(r, now)
		return nil
	})
	return r.Session, e
}
func (s *Service) Cancel(ctx context.Context, id string, version int64) (v1.IntentConversation, error) {
	a, e := actor(ctx, s.Platform.TenantID)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	if !identifier.MatchString(id) || version < 1 {
		return v1.IntentConversation{}, model.Invalid("invalid conversation or version")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		if !active(*r) || r.Session.Version != version {
			return ErrConflict
		}
		p, _ := tenancy.FromContext(ctx)
		run := current(r)
		if p.Role != "owner" && (run.Actor.Subject != a.Subject || run.Actor.Kind != a.Kind) {
			return ErrForbidden
		}
		finish(r, "cancelled", "", "Preparation cancelled. Saved definitions were not changed.", now)
		return nil
	})
	return r.Session, e
}
func finish(r *record, status, code, message string, now time.Time) {
	run := current(r)
	run.Status = status
	run.ErrorCode = code
	run.UpdatedAt = now
	r.Session.State = status
	r.Token = ""
	r.LeaseUntil = time.Time{}
	r.Pending = nil
	if message != "" {
		r.Session.Messages = append(r.Session.Messages, v1.IntentConversationMessage{ID: run.ID + "-reply", Role: "assistant", Text: message, At: now, Actor: v1.InvestigationActor{Subject: "intent-workflow", Name: "Intent workflow", Kind: "assistant"}, RunID: run.ID})
	}
	touch(r, now)
}
