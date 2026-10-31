package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/tenancy"
)

const Policy = "selective-v2"
const LegacyPolicy = "masked-v1"

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type Service struct {
	Privacy    v1.ReplayPrivacy
	Platform   *service.Service
	Repository Repository
}

func New(p *service.Service, r Repository) *Service { return &Service{Platform: p, Repository: r} }
func digest(b []byte) string                        { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func (s *Service) actor(ctx context.Context) (v1.InvestigationActor, error) {
	p, ok := tenancy.FromContext(ctx)
	if !ok || p.TenantID != s.Platform.TenantID || p.Subject == "" || (p.Role != "owner" && p.Role != "editor" && p.Role != "agent") {
		return v1.InvestigationActor{}, model.Invalid("replay writes require an authorized owner, editor or agent")
	}
	return v1.InvestigationActor{Subject: p.Subject, Kind: p.Kind, Name: p.Name}, nil
}
func (s *Service) references(ctx context.Context, refs []v1.ReplayReference) ([]v1.ReplayReference, error) {
	if len(refs) > 10 {
		return nil, model.Invalid("link at most ten resources or alert events")
	}
	out := []v1.ReplayReference{}
	seen := map[string]bool{}
	for _, ref := range refs {
		if !idPattern.MatchString(ref.ID) || seen[ref.Kind+":"+ref.ID] {
			return nil, model.Invalid("invalid or duplicate replay reference")
		}
		seen[ref.Kind+":"+ref.ID] = true
		switch ref.Kind {
		case v1.SLOKind:
			r, err := s.Platform.Store.Read(ctx, ref.ID)
			if err != nil {
				return nil, err
			}
			if ref.Revision != model.ConfigRevision(r.SLO) || ref.At != nil || ref.SLOID != "" {
				return nil, model.Invalid("SLO link requires its exact current revision")
			}
		case v1.JourneyKind:
			if s.Platform.Journeys == nil {
				return nil, model.ErrUnavailable
			}
			if ref.Revision < 1 || ref.At != nil || ref.SLOID != "" {
				return nil, model.Invalid("journey link requires an exact revision")
			}
			if _, err := s.Platform.Journeys.JourneyRevision(ctx, ref.ID, ref.Revision); err != nil {
				return nil, err
			}
		case "AlertEvent":
			deliveries, err := s.Platform.Store.Deliveries(ctx)
			if err != nil {
				return nil, err
			}
			found := false
			for _, d := range deliveries {
				e := d.Event
				if e.ID == ref.ID && e.Kind == "firing" {
					if ref.Revision != e.ConfigRevision || ref.SLOID != "" && ref.SLOID != e.SLOID || ref.At != nil && !ref.At.Equal(e.Time) {
						return nil, model.Invalid("alert reference differs from the stored event")
					}
					ref.SLOID = e.SLOID
					at := e.Time
					ref.At = &at
					found = true
					break
				}
			}
			if !found {
				return nil, model.Invalid("violation link must name a stored firing alert event")
			}
		default:
			return nil, model.Invalid("unsupported replay reference kind")
		}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind+":"+out[i].ID < out[j].Kind+":"+out[j].ID })
	return out, nil
}
func (s *Service) Create(ctx context.Context, in v1.CreateSessionReplay) (v1.SessionReplay, error) {
	if s.Privacy.Disabled {
		return v1.SessionReplay{}, model.Invalid("new replay recording and sharing are disabled by the operator")
	}
	actor, err := s.actor(ctx)
	if err != nil {
		return v1.SessionReplay{}, err
	}
	principal, _ := tenancy.FromContext(ctx)
	if principal.Role != "owner" && principal.Role != "editor" {
		return v1.SessionReplay{}, model.Invalid("replay sharing consent requires an owner or editor")
	}
	if !idPattern.MatchString(in.ID) || !utf8.ValidString(in.Title) || strings.TrimSpace(in.Title) == "" || len(in.Title) > 160 || strings.ContainsRune(in.Title, 0) || !in.Consent || !in.SharingConsent || in.NoticeVersion != v1.ReplayConsentNotice || (in.Policy != Policy && in.Policy != LegacyPolicy) {
		return v1.SessionReplay{}, model.Invalid("provide a valid ID, title (1–160 bytes) and explicit recording and tenant-sharing consent to replay-sharing-v1 and a supported recording policy")
	}
	events, err := sanitize(in.Events, in.Policy == Policy)
	if err != nil {
		return v1.SessionReplay{}, model.Invalid("%v", err)
	}
	payload, _ := json.Marshal(events)
	if len(payload) > MaxBytes {
		return v1.SessionReplay{}, model.Invalid("sanitized recording exceeds two MiB")
	}
	request, _ := json.Marshal(struct {
		Title      string
		Actor      v1.InvestigationActor
		References []v1.ReplayReference
		Digest     string
	}{in.Title, actor, in.References, digest(payload)})
	requestHash := digest(request)
	if in.Policy != LegacyPolicy {
		requestHash = digest(append(request, []byte(in.Policy)...))
	}
	// Exact immutable ingestion retries remain valid after linked resources advance.
	if old, e := s.Repository.Read(ctx, in.ID); e == nil {
		if old.RequestDigest != requestHash {
			return old.Meta, ErrConflict
		}
		return old.Meta, nil
	} else if e != ErrNotFound {
		return v1.SessionReplay{}, e
	}
	refs, err := s.references(ctx, in.References)
	if err != nil {
		return v1.SessionReplay{}, err
	}
	var start, end struct {
		Timestamp int64 `json:"timestamp"`
	}
	_ = json.Unmarshal(events[0], &start)
	_ = json.Unmarshal(events[len(events)-1], &end)
	now := time.Now().UTC()
	started := time.UnixMilli(start.Timestamp).UTC()
	finished := time.UnixMilli(end.Timestamp).UTC()
	if started.Before(now.Add(-7*24*time.Hour)) || finished.After(now.Add(5*time.Minute)) {
		return v1.SessionReplay{}, model.Invalid("recording timestamps must be within the last seven days")
	}
	m := v1.SessionReplay{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, ID: in.ID, Title: in.Title, Version: 1, CreatedAt: now, ExpiresAt: now.Add(RetentionDays * 24 * time.Hour), StartedAt: started, FinishedAt: finished, DurationMillis: end.Timestamp - start.Timestamp, EventCount: len(events), SizeBytes: len(payload), Digest: digest(payload), Policy: in.Policy, RecordedBy: actor, References: refs, ConsentReceipt: &v1.EvidenceConsent{NoticeVersion: in.NoticeVersion, At: now, Actor: actor, Purpose: "record and share masked replay within this tenant"}}
	r, err := s.Repository.Create(ctx, record{Meta: m, Events: events, RequestDigest: requestHash})
	return r.Meta, err
}
func (s *Service) reader(ctx context.Context) bool {
	p, ok := tenancy.FromContext(ctx)
	return ok && p.TenantID == s.Platform.TenantID && p.Subject != "" && (p.Role == "owner" || p.Role == "editor" || p.Role == "agent" || p.Role == "viewer")
}
func (s *Service) read(ctx context.Context, id string) (record, error) {
	if !s.reader(ctx) {
		return record{}, ErrNotFound
	}
	if !idPattern.MatchString(id) {
		return record{}, model.Invalid("invalid session ID")
	}
	r, err := s.Repository.Read(ctx, id)
	p, _ := tenancy.FromContext(ctx)
	if err == nil && (r.Meta.Tenant != s.Platform.TenantID || r.Meta.Archived && p.Role != "owner") {
		return record{}, ErrNotFound
	}
	return r, err
}
func (s *Service) Read(ctx context.Context, id string) (v1.SessionReplay, error) {
	r, e := s.read(ctx, id)
	return r.Meta, e
}
func (s *Service) Events(ctx context.Context, id string) (v1.ReplayEvents, error) {
	r, e := s.read(ctx, id)
	if e != nil {
		return v1.ReplayEvents{}, e
	}
	payload, err := json.Marshal(r.Events)
	if err != nil || digest(payload) != r.Meta.Digest {
		return v1.ReplayEvents{}, ErrCorrupt
	}
	return v1.ReplayEvents{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, ID: id, Digest: r.Meta.Digest, Events: r.Events}, e
}
func (s *Service) List(ctx context.Context, kind, id string, archived bool) (v1.SessionReplayList, error) {
	out := v1.SessionReplayList{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Items: []v1.SessionReplay{}, RetentionDays: RetentionDays, MaxSessions: MaxSessions}
	if !s.reader(ctx) {
		return out, ErrNotFound
	}
	p, _ := tenancy.FromContext(ctx)
	if archived && p.Role != "owner" {
		return out, model.Invalid("archived sessions are visible to owners")
	}
	if (kind == "") != (id == "") || id != "" && !idPattern.MatchString(id) {
		return out, model.Invalid("filter by both resourceKind and resourceId")
	}
	items, err := s.Repository.List(ctx)
	if err != nil {
		return out, err
	}
	for _, m := range items {
		if m.Tenant != s.Platform.TenantID {
			return out, ErrNotFound
		}
		if m.Archived && !archived {
			continue
		}
		match := id == ""
		for _, ref := range m.References {
			if ref.Kind == kind && ref.ID == id {
				match = true
			}
		}
		if match {
			out.Items = append(out.Items, m)
		}
	}
	return out, nil
}
func (s *Service) Links(ctx context.Context, id string, in v1.UpdateReplayLinks) (v1.SessionReplay, error) {
	if _, err := s.actor(ctx); err != nil {
		return v1.SessionReplay{}, err
	}
	if _, err := s.read(ctx, id); err != nil {
		return v1.SessionReplay{}, err
	}
	prior, err := s.read(ctx, id)
	if err != nil {
		return v1.SessionReplay{}, err
	}
	fresh := []v1.ReplayReference{}
	retained := []v1.ReplayReference{}
	for _, ref := range in.References {
		known := false
		for _, old := range prior.Meta.References {
			a, _ := json.Marshal(ref)
			b, _ := json.Marshal(old)
			if string(a) == string(b) {
				retained = append(retained, old)
				known = true
				break
			}
		}
		if !known {
			fresh = append(fresh, ref)
		}
	}
	refs, err := s.references(ctx, fresh)
	if err != nil {
		return v1.SessionReplay{}, err
	}
	refs = append(refs, retained...)
	if len(refs) > 10 {
		return v1.SessionReplay{}, model.Invalid("link at most ten resources")
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		key := ref.Kind + ":" + ref.ID
		if seen[key] {
			return v1.SessionReplay{}, model.Invalid("duplicate replay reference")
		}
		seen[key] = true
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Kind+":"+refs[i].ID < refs[j].Kind+":"+refs[j].ID })
	r, err := s.Repository.Update(ctx, id, func(r *record) error {
		if r.Meta.Version != in.ExpectedVersion {
			return ErrConflict
		}
		r.Meta.References = refs
		r.Meta.Version++
		return nil
	})
	return r.Meta, err
}
func (s *Service) Archive(ctx context.Context, id string, in v1.ArchiveReplay) (v1.SessionReplay, error) {
	p, _ := tenancy.FromContext(ctx)
	if p.Role != "owner" {
		return v1.SessionReplay{}, model.Invalid("an owner is required to archive or restore sessions")
	}
	if _, err := s.read(ctx, id); err != nil {
		return v1.SessionReplay{}, err
	}
	r, err := s.Repository.Update(ctx, id, func(r *record) error {
		if r.Meta.Version != in.ExpectedVersion {
			return ErrConflict
		}
		r.Meta.Archived = in.Archived
		r.Meta.Version++
		return nil
	})
	return r.Meta, err
}
func (s *Service) RunRetention(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if visual, ok := s.Platform.Store.(interface{ PruneVisual(context.Context) error }); ok {
			if err := visual.PruneVisual(ctx); err != nil && ctx.Err() == nil {
				log.Error("visual receipt retention failed", "tenant", s.Platform.TenantID)
			}
		}
		if err := s.Repository.Prune(ctx); err != nil && ctx.Err() == nil {
			log.Error("session replay retention failed", "tenant", s.Platform.TenantID)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) Delete(ctx context.Context, id string, version int64) error {
	p, ok := tenancy.FromContext(ctx)
	if !ok || p.TenantID != s.Platform.TenantID || p.Role != "owner" {
		return model.Invalid("an owner is required to permanently delete recordings")
	}
	if version < 1 {
		return model.Invalid("expectedVersion must be positive")
	}
	if _, e := s.read(ctx, id); e != nil {
		return e
	}
	return s.Repository.Delete(ctx, id, version)
}
