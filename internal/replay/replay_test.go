package replay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func recording() v1.CreateSessionReplay {
	t := time.Now().Add(-time.Minute).UnixMilli()
	raw := fmt.Sprintf(`[{"type":4,"timestamp":%d,"data":{"href":"https://evil.example/?secret=token","width":800,"height":600}},{"type":2,"timestamp":%d,"data":{"node":{"type":0,"id":1,"childNodes":[{"type":2,"id":2,"tagName":"html","attributes":{},"childNodes":[{"type":2,"id":3,"tagName":"body","attributes":{"onclick":"secret-handler","title":"secret-title"},"childNodes":[{"type":3,"id":4,"textContent":"secret@example.com"},{"type":2,"id":5,"tagName":"script","attributes":{"src":"https://evil.example/secret"},"childNodes":[{"type":3,"id":6,"textContent":"secret-script"}]}]}]}]},"initialOffset":{"left":0,"top":0}}},{"type":3,"timestamp":%d,"data":{"source":5,"id":4,"text":"secret-password","isChecked":false}},{"type":3,"timestamp":%d,"data":{"source":11,"secret":"plugin-secret"}}]`, t, t+1, t+1000, t+2000)
	var events []json.RawMessage
	_ = json.Unmarshal([]byte(raw), &events)
	return v1.CreateSessionReplay{ID: "capture-1", Title: "Checkout sample", Consent: true, Policy: Policy, References: []v1.ReplayReference{{Kind: v1.SLOKind, ID: "checkout", Revision: 1}}, Events: events}
}
func fixture(t *testing.T, pg bool) (*Service, context.Context, string) {
	t.Helper()
	ctx := context.Background()
	dsn := ""
	var repo Repository
	if pg {
		base := os.Getenv("TEST_DATABASE_URL")
		if base == "" {
			t.Skip("set TEST_DATABASE_URL for replay persistence tests")
		}
		admin, err := pgxpool.New(ctx, base)
		if err != nil {
			t.Fatal(err)
		}
		schema := "replay_test_" + model.NewID()
		if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(base)
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
		repo, err = NewPostgres(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			repo.Close()
			if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
				t.Error(err)
				return
			}
			admin.Close()
		})
	} else {
		repo = NewMemory()
	}
	st := store.NewMemory()
	if err := st.Create(ctx, model.SLO{ID: "checkout", Name: "Checkout", Project: "commerce", Service: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	platform := service.New(st, nil)
	platform.TenantID = "alpha"
	ctx = tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "owner", Subject: "owner-1", Kind: "user", Name: "Owner"})
	return New(platform, repo), ctx, dsn
}
func modes(t *testing.T, fn func(*testing.T, bool)) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) { fn(t, pg) })
	}
}
func TestSanitizerRemovesSecretsAndUnsupportedCapabilities(t *testing.T) {
	in := recording()
	events, err := Sanitize(in.Events)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(events)
	for _, value := range []string{"secret", "evil.example", "onclick", "script", "password"} {
		if strings.Contains(string(b), value) {
			t.Fatal("unmasked or executable field persisted", value)
		}
	}
	if len(events) != 3 {
		t.Fatal("plugin data retained")
	}
	bad := append([]json.RawMessage{}, in.Events...)
	bad[2] = json.RawMessage(`{"type":3,"timestamp":1,"data":{"source":5}}`)
	if _, err = Sanitize(bad); err == nil {
		t.Fatal("out-of-order events accepted")
	}
	if _, err = Sanitize(in.Events[:1]); err == nil {
		t.Fatal("snapshot-free replay accepted")
	}
	css := attributes(map[string]any{"style": "background:url(https://evil.example/secret);content:'secret';width:20px", "value": "secret"})
	if strings.Contains(fmt.Sprint(css), "secret") {
		t.Fatal(css)
	}
}
func TestReplayPersistenceIdempotencyCASAndArchive(t *testing.T) {
	modes(t, func(t *testing.T, pg bool) {
		s, ctx, dsn := fixture(t, pg)
		in := recording()
		m, err := s.Create(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		events, err := s.Events(ctx, m.ID)
		if err != nil || events.Digest != m.Digest {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(events.Events)
		if digest(payload) != m.Digest {
			t.Fatal("digest does not bind sanitized bytes")
		}
		if pg {
			s.Repository.Close()
			repo, err := NewPostgres(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			s = New(s.Platform, repo)
		}
		again, err := s.Create(ctx, in)
		if err != nil || again.CreatedAt != m.CreatedAt {
			t.Fatal("ingestion retry changed immutable recording", err)
		}
		in.Title = "changed"
		if _, err = s.Create(ctx, in); !errors.Is(err, ErrConflict) {
			t.Fatal("changed retry accepted", err)
		}
		if _, err = s.Links(ctx, m.ID, v1.UpdateReplayLinks{ExpectedVersion: 7, References: []v1.ReplayReference{}}); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		updated, err := s.Links(ctx, m.ID, v1.UpdateReplayLinks{ExpectedVersion: 1, References: []v1.ReplayReference{}})
		if err != nil || updated.Version != 2 {
			t.Fatal(err)
		}
		archived, err := s.Archive(ctx, m.ID, v1.ArchiveReplay{ExpectedVersion: 2, Archived: true})
		if err != nil || !archived.Archived {
			t.Fatal(err)
		}
		viewer := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "viewer", Subject: "viewer", Kind: "user"})
		if _, err = s.Events(viewer, m.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("archived events exposed to viewer")
		}
		if _, err = s.Archive(viewer, m.ID, v1.ArchiveReplay{ExpectedVersion: 3}); err == nil {
			t.Fatal("viewer restored a replay")
		}
		if _, err = s.Archive(ctx, m.ID, v1.ArchiveReplay{ExpectedVersion: 3}); err != nil {
			t.Fatal(err)
		}
		restored, _ := s.Events(viewer, m.ID)
		if restored.Digest != m.Digest {
			t.Fatal("archive changed event bytes")
		}
		restored.Events[0][0] = 'X'
		clean, _ := s.Events(viewer, m.ID)
		if !json.Valid(clean.Events[0]) {
			t.Fatal("returned events share mutable storage")
		}
	})
}
func TestReplayQuotaConcurrentRetryAndExpiry(t *testing.T) {
	modes(t, func(t *testing.T, pg bool) {
		s, ctx, _ := fixture(t, pg)
		in := recording()
		var wg sync.WaitGroup
		for range 6 {
			wg.Go(func() {
				if _, err := s.Create(ctx, in); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		list, err := s.List(ctx, "", "", false)
		if err != nil || len(list.Items) != 1 {
			t.Fatal(list, err)
		}
		original, err := s.Repository.Read(ctx, in.ID)
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < MaxSessions; i++ {
			r := clone(original)
			r.Meta.ID = fmt.Sprintf("slot-%d", i)
			if _, err = s.Repository.Create(ctx, r); err != nil {
				t.Fatal(err)
			}
		}
		last := clone(original)
		last.Meta.ID = "over-limit"
		if _, err = s.Repository.Create(ctx, last); !errors.Is(err, ErrLimit) {
			t.Fatal("capacity not enforced", err)
		}
		if _, err = s.Repository.Update(ctx, in.ID, func(r *record) error { r.Meta.ExpiresAt = time.Now().Add(-time.Second); return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Read(ctx, in.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("expired replay readable")
		}
		if err = s.Repository.Prune(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Repository.Create(ctx, last); err != nil {
			t.Fatal("expired capacity not released", err)
		}
	})
}
func TestReplayLinksValidateTenantRevisionAndStoredViolation(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	in := recording()
	in.References[0].ID = "other-tenant-resource"
	if _, err := s.Create(ctx, in); err == nil {
		t.Fatal("missing resource accepted")
	}
	in = recording()
	in.References[0].Revision = 99
	if _, err := s.Create(ctx, in); err == nil {
		t.Fatal("wrong revision accepted")
	}
	event := model.AlertEvent{ID: "firing-event", SLOID: "checkout", Kind: "firing", ConfigRevision: 1, Time: time.Now()}
	if err := s.Platform.Store.Update(ctx, "checkout", func(r *model.Record) error { r.Events = append(r.Events, event); return nil }); err != nil {
		t.Fatal(err)
	}
	in = recording()
	in.References = append(in.References, v1.ReplayReference{Kind: "AlertEvent", ID: event.ID, Revision: 1})
	m, err := s.Create(ctx, in)
	if err != nil || len(m.References) != 2 {
		t.Fatal(m, err)
	}
	if err := s.Platform.Store.Update(ctx, "checkout", func(r *model.Record) error { r.SLO.Revision = 2; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, in); err != nil {
		t.Fatal("retry after source revision changed failed", err)
	}
	if _, err = s.Links(ctx, in.ID, v1.UpdateReplayLinks{ExpectedVersion: 1, References: m.References}); err != nil {
		t.Fatal("verified old links could not be retained", err)
	}
	in = recording()
	in.ID = "no-consent"
	in.Consent = false
	if _, err = s.Create(ctx, in); err == nil {
		t.Fatal("implicit consent accepted")
	}
}

func TestSVGAndDisclosureStateSurviveWithoutActiveReferences(t *testing.T) {
	attrs := attributes(map[string]any{"d": "M0 50 L100 20", "viewBox": "0 0 800 300", "open": "", "data-state": "open", "fill": "url(https://secret)", "href": "https://secret", "onload": "secret", "class": nil})
	if attrs["d"] != "M0 50 L100 20" || attrs["open"] != "" || attrs["data-state"] != "open" {
		t.Fatal(attrs)
	}
	b, _ := json.Marshal(attrs)
	if strings.Contains(string(b), "secret") {
		t.Fatal(string(b))
	}
	count := 0
	n, err := node(map[string]any{"type": float64(2), "id": float64(2), "tagName": "svg", "isSVG": true, "attributes": attrs, "childNodes": []any{}}, 0, &count, false)
	if err != nil || n["tagName"] != "svg" || n["isSVG"] != true {
		t.Fatal(n, err)
	}
}
