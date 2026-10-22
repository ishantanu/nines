package investigation

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	Model          rm.Model
	AuthorizeWatch func(context.Context, v1.InvestigationActor) error
	watchSlots     chan struct{}
	Platform       *service.Service
	Repository     Repository
	AuthorizeRun   func(context.Context, v1.InvestigationActor) error
}

func New(platform *service.Service, repo Repository) *Service {
	return &Service{Platform: platform, Repository: repo, watchSlots: make(chan struct{}, 8)}
}
func validText(s string, max int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && strings.TrimSpace(s) != "" && len(s) <= max
}
func (s *Service) actor(ctx context.Context) (v1.InvestigationActor, error) {
	p, ok := tenancy.FromContext(ctx)
	if !ok || p.TenantID != s.Platform.TenantID || p.Subject == "" || p.Role != "owner" && p.Role != "editor" && p.Role != "agent" {
		return v1.InvestigationActor{}, model.Invalid("a tenant-authorized writing principal is required")
	}
	return v1.InvestigationActor{Subject: p.Subject, Name: p.Name, Kind: p.Kind}, nil
}
func (s *Service) Read(ctx context.Context, id string) (v1.Investigation, error) {
	if !identifier.MatchString(id) {
		return v1.Investigation{}, model.Invalid("invalid investigation ID")
	}
	v, err := s.Repository.Read(ctx, id)
	if err == nil && v.Tenant != s.Platform.TenantID {
		return v1.Investigation{}, ErrNotFound
	}
	return v, err
}
func (s *Service) List(ctx context.Context, cursor string, limit int) (v1.InvestigationList, error) {
	out := v1.InvestigationList{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Items: []v1.InvestigationSummary{}}
	if limit < 1 || limit > 50 {
		return out, model.Invalid("limit must be 1–50")
	}
	after := ""
	if len(cursor) > 300 {
		return out, model.Invalid("invalid investigation cursor")
	}
	if cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(b) > 200 {
			return out, model.Invalid("invalid investigation cursor")
		}
		parts := strings.Split(string(b), "\x00")
		if len(parts) != 2 || parts[0] != s.Platform.TenantID || !identifier.MatchString(parts[1]) {
			return out, model.Invalid("cursor belongs to another scope or is invalid")
		}
		after = parts[1]
	}
	rows, err := s.Repository.List(ctx, after, limit+1)
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(s.Platform.TenantID + "\x00" + rows[limit-1].ID))
		rows = rows[:limit]
	}
	out.Items = rows
	return out, nil
}
func (s *Service) context(ctx context.Context, scope v1.InvestigationScope) (json.RawMessage, error) {
	options := v1.ContextOptions{ExpectedRevision: scope.ExpectedRevision, At: scope.At}
	var value any
	switch scope.Kind {
	case v1.SLOKind:
		v, err := s.Platform.SLOContext(ctx, scope.ResourceID, options)
		if err != nil {
			return nil, err
		}
		value = v
	case v1.JourneyKind:
		options.Limit = 10
		v, err := s.Platform.JourneyContext(ctx, scope.ResourceID, options)
		if err != nil {
			return nil, err
		}
		value = v
	default:
		return nil, model.Invalid("scope kind must be ServiceLevelObjective or CriticalUserJourney")
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(b) > 64<<10 {
		return nil, model.Invalid("context exceeds the 64 KiB evidence limit; narrow the resource scope")
	}
	return b, nil
}
func (s *Service) Create(ctx context.Context, in v1.CreateInvestigation) (v1.Investigation, error) {
	actor, err := s.actor(ctx)
	if err != nil {
		return v1.Investigation{}, err
	}
	if !identifier.MatchString(in.ID) || !identifier.MatchString(in.Scope.ResourceID) || !validText(in.Title, 160) || in.Scope.ExpectedRevision < 1 || in.Scope.At.IsZero() || in.Scope.At.Nanosecond() != 0 || in.Scope.At.After(s.Platform.Now()) || in.Scope.At.Before(s.Platform.Now().Add(-31*24*time.Hour)) {
		return v1.Investigation{}, model.Invalid("provide valid IDs/title, a positive revision and an explicit whole-second evidence cutoff within the last 31 days")
	}
	in.Scope.At = in.Scope.At.UTC()
	if old, err := s.Repository.Read(ctx, in.ID); err == nil {
		candidate := v1.Investigation{Tenant: s.Platform.TenantID, InvestigationSummary: v1.InvestigationSummary{Title: in.Title, Scope: in.Scope, CreatedBy: actor}}
		if !sameCreate(old, candidate) {
			return v1.Investigation{}, ErrConflict
		}
		return old, nil
	} else if err != ErrNotFound {
		return v1.Investigation{}, err
	}
	if _, err = s.context(ctx, in.Scope); err != nil {
		return v1.Investigation{}, err
	}
	now := s.Platform.Now().UTC()
	return s.Repository.Create(ctx, v1.Investigation{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, InvestigationSummary: v1.InvestigationSummary{ID: in.ID, Title: in.Title, Version: 1, Scope: in.Scope, CreatedBy: actor, CreatedAt: now, UpdatedAt: now}, Entries: []v1.InvestigationEntry{}})
}
func replay(v v1.Investigation, id, kind, text, subject string) (bool, error) {
	for _, e := range v.Entries {
		if e.ID == id {
			if e.Kind != kind || e.Text != text || e.Actor.Subject != subject {
				return false, ErrConflict
			}
			return true, nil
		}
	}
	return false, nil
}
func (s *Service) append(ctx context.Context, id string, expected int64, entry v1.InvestigationEntry) (v1.Investigation, error) {
	return s.Repository.Update(ctx, id, func(v *v1.Investigation) error {
		if v.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		done, err := replay(*v, entry.ID, entry.Kind, entry.Text, entry.Actor.Subject)
		if err != nil || done {
			return err
		}
		if expected != v.Version {
			return ErrConflict
		}
		if len(v.Entries) >= 200 {
			return model.Invalid("investigation entry limit reached; start a new investigation")
		}
		if entry.Evidence != nil {
			n := 0
			for _, e := range v.Entries {
				if e.Evidence != nil {
					n++
				}
			}
			if n >= 20 {
				return model.Invalid("investigation evidence limit reached")
			}
		}
		v.Entries = append(v.Entries, entry)
		v.Version++
		v.UpdatedAt = entry.CreatedAt
		b, err := json.Marshal(v)
		if err == nil && len(b) > 2<<20 {
			return model.Invalid("investigation exceeds the 2 MiB storage limit")
		}
		return err
	})
}
func (s *Service) Message(ctx context.Context, id string, in v1.AppendInvestigationMessage) (v1.Investigation, error) {
	actor, err := s.actor(ctx)
	if err != nil {
		return v1.Investigation{}, err
	}
	if !identifier.MatchString(id) || !identifier.MatchString(in.ID) || in.ExpectedVersion < 1 || !validText(in.Text, 4000) {
		return v1.Investigation{}, model.Invalid("message requires valid IDs, expectedVersion and 1–4000 bytes of UTF-8 text")
	}
	return s.append(ctx, id, in.ExpectedVersion, v1.InvestigationEntry{ID: in.ID, Kind: "message", Actor: actor, CreatedAt: s.Platform.Now().UTC(), Text: in.Text})
}
func (s *Service) Capture(ctx context.Context, id string, in v1.CaptureInvestigationEvidence) (v1.Investigation, error) {
	actor, err := s.actor(ctx)
	if err != nil {
		return v1.Investigation{}, err
	}
	if !identifier.MatchString(in.ID) || in.ExpectedVersion < 1 {
		return v1.Investigation{}, model.Invalid("capture requires an ID and positive expectedVersion")
	}
	old, err := s.Read(ctx, id)
	if err != nil {
		return old, err
	}
	done, err := replay(old, in.ID, "evidence", "", actor.Subject)
	if err != nil || done {
		return old, err
	}
	if old.Version != in.ExpectedVersion {
		return old, ErrConflict
	}
	b, err := s.context(ctx, old.Scope)
	if err != nil {
		return old, err
	}
	digest := sha256.Sum256(b)
	now := s.Platform.Now().UTC()
	return s.append(ctx, id, in.ExpectedVersion, v1.InvestigationEntry{ID: in.ID, Kind: "evidence", CreatedAt: now, Actor: actor, Evidence: &v1.InvestigationEvidence{CapturedAt: now, Digest: hex.EncodeToString(digest[:]), Context: b}})
}
