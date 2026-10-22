package investigation

import (
	"context"
	"encoding/json"
	"errors"
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

func fixture(t *testing.T, postgresMode bool) (*Service, context.Context, string) {
	t.Helper()
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	var repo Repository
	var st store.Store
	dsn := ""
	if postgresMode {
		base := os.Getenv("TEST_DATABASE_URL")
		if base == "" {
			t.Skip("set TEST_DATABASE_URL to run PostgreSQL investigation tests")
		}
		admin, err := pgxpool.New(context.Background(), base)
		if err != nil {
			t.Fatal(err)
		}
		schema := "investigation_test_" + model.NewID()
		if _, err = admin.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(base)
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
		st, err = store.NewPostgres(context.Background(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		repo, err = NewPostgres(context.Background(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			repo.Close()
			st.Close()
			if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
				t.Error(err)
				return
			}
			admin.Close()
		})
	} else {
		repo = NewMemory()
		st = store.NewMemory()
		t.Cleanup(func() { repo.Close(); st.Close() })
	}
	svc := service.New(st, nil)
	svc.TenantID = "alpha"
	svc.Now = func() time.Time { return at }
	if err := st.Create(context.Background(), model.SLO{ID: "checkout", Project: "commerce", Name: "Checkout", Service: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	ctx := tenancy.WithPrincipal(context.Background(), tenancy.Principal{TenantID: "alpha", Role: "editor", Subject: "user-1", Name: "Operator", Kind: "user"})
	return New(svc, repo), ctx, dsn
}
func input() v1.CreateInvestigation {
	return v1.CreateInvestigation{ID: "investigation-1", Title: "Checkout burn investigation", Scope: v1.InvestigationScope{Kind: v1.SLOKind, ResourceID: "checkout", ExpectedRevision: 1, At: time.Date(2026, 10, 20, 11, 59, 0, 0, time.UTC)}}
}
func TestInvestigationStoreAndServiceContract(t *testing.T) {
	for _, mode := range []bool{false, true} {
		name := "memory"
		if mode {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			s, ctx, dsn := fixture(t, mode)
			v, err := s.Create(ctx, input())
			if err != nil || v.Version != 1 || v.Tenant != "alpha" {
				t.Fatal(v, err)
			}
			v, err = s.Message(ctx, v.ID, v1.AppendInvestigationMessage{ID: "message-1", ExpectedVersion: 1, Text: "Check the measurement contract before interpreting burn."})
			if err != nil || v.Version != 2 || v.Entries[0].Actor.Subject != "user-1" {
				t.Fatal(v, err)
			}
			again, err := s.Message(ctx, v.ID, v1.AppendInvestigationMessage{ID: "message-1", ExpectedVersion: 1, Text: v.Entries[0].Text})
			if err != nil || again.Version != 2 || len(again.Entries) != 1 {
				t.Fatal("retry duplicated message", again, err)
			}
			if _, err = s.Message(ctx, v.ID, v1.AppendInvestigationMessage{ID: "message-2", ExpectedVersion: 1, Text: "stale"}); !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			if _, err = s.Message(ctx, v.ID, v1.AppendInvestigationMessage{ID: "message-1", ExpectedVersion: 2, Text: "changed"}); !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			v, err = s.Capture(ctx, v.ID, v1.CaptureInvestigationEvidence{ID: "evidence-1", ExpectedVersion: 2})
			if err != nil || v.Version != 3 || len(v.Entries) != 2 || len(v.Entries[1].Evidence.Digest) != 64 {
				t.Fatal(v, err)
			}
			original := string(v.Entries[1].Evidence.Context)
			if !strings.Contains(original, `"tenant":"alpha"`) || !strings.Contains(original, `"reliability":null`) {
				t.Fatal("missing data was converted to success", original)
			}
			v.Entries[1].Evidence.Context[0] = 'X'
			loaded, err := s.Read(ctx, v.ID)
			if err != nil || string(loaded.Entries[1].Evidence.Context) != original {
				t.Fatal("shared mutable evidence", err)
			}
			if _, err = s.Create(ctx, input()); err != nil {
				t.Fatal("create retry failed after later messages", err)
			}
			other := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "owner", Subject: "user-2"})
			if _, err = s.Create(other, input()); !errors.Is(err, ErrConflict) {
				t.Fatal("request identity aliased", err)
			}
			if err = s.Platform.Store.Update(ctx, "checkout", func(r *model.Record) error { r.SLO.Revision = 2; return nil }); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Capture(ctx, v.ID, v1.CaptureInvestigationEvidence{ID: "evidence-2", ExpectedVersion: 3}); !errors.Is(err, model.ErrRevisionConflict) {
				t.Fatal("stale resource evidence accepted", err)
			}
			replay, err := s.Capture(ctx, v.ID, v1.CaptureInvestigationEvidence{ID: "evidence-1", ExpectedVersion: 2})
			if err != nil || replay.Version != 3 || string(replay.Entries[1].Evidence.Context) != original {
				t.Fatal("replayed evidence was recaptured", err)
			}
			if mode {
				reopened, err := NewPostgres(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				saved, err := reopened.Read(ctx, v.ID)
				if err != nil || saved.Version != 3 || string(saved.Entries[1].Evidence.Context) != original {
					t.Fatal("state lost after reopen", err)
				}
			}
		})
	}
}
func TestInvestigationConcurrentAppendIsFenced(t *testing.T) {
	for _, mode := range []bool{false, true} {
		name := "memory"
		if mode {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			s, ctx, _ := fixture(t, mode)
			if _, err := s.Create(ctx, input()); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			results := make(chan error, 2)
			for _, id := range []string{"message-a", "message-b"} {
				wg.Add(1)
				go func(id string) {
					defer wg.Done()
					_, err := s.Message(ctx, "investigation-1", v1.AppendInvestigationMessage{ID: id, ExpectedVersion: 1, Text: "concurrent note"})
					results <- err
				}(id)
			}
			wg.Wait()
			close(results)
			success, conflict := 0, 0
			for err := range results {
				if err == nil {
					success++
				} else if errors.Is(err, ErrConflict) {
					conflict++
				} else {
					t.Fatal(err)
				}
			}
			if success != 1 || conflict != 1 {
				t.Fatal(success, conflict)
			}
			v, _ := s.Read(ctx, "investigation-1")
			if v.Version != 2 || len(v.Entries) != 1 {
				t.Fatal(v)
			}
		})
	}
}
func TestScopePaginationAndEntryLimits(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	for _, id := range []string{"a", "b", "c"} {
		in := input()
		in.ID = id
		if _, err := s.Create(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.List(ctx, "", 1)
	if err != nil || page.Items[0].ID != "a" || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	next, err := s.List(ctx, page.NextCursor, 1)
	if err != nil || next.Items[0].ID != "b" {
		t.Fatal(next, err)
	}
	s.Platform.TenantID = "beta"
	if _, err = s.List(ctx, page.NextCursor, 1); err == nil {
		t.Fatal("cross-tenant cursor accepted")
	}
	s.Platform.TenantID = "alpha"
	for _, change := range []func(*v1.CreateInvestigation){func(in *v1.CreateInvestigation) { in.Scope.ExpectedRevision = 0 }, func(in *v1.CreateInvestigation) { in.Scope.Kind = "workspace" }, func(in *v1.CreateInvestigation) { in.Scope.At = in.Scope.At.Add(2 * time.Hour) }, func(in *v1.CreateInvestigation) { in.Title = strings.Repeat("x", 161) }} {
		in := input()
		change(&in)
		if _, err = s.Create(ctx, in); err == nil {
			t.Fatal("invalid scope accepted")
		}
	}
	viewer := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "viewer", Subject: "viewer"})
	if _, err = s.Create(viewer, input()); err == nil {
		t.Fatal("viewer wrote investigation")
	}
	if _, err = s.Message(ctx, "a", v1.AppendInvestigationMessage{ID: "bad", ExpectedVersion: 1, Text: strings.Repeat("x", 4001)}); err == nil {
		t.Fatal("oversized message accepted")
	}
	if _, err = s.Repository.Update(ctx, "a", func(v *v1.Investigation) error {
		for i := 0; i < 200; i++ {
			v.Entries = append(v.Entries, v1.InvestigationEntry{ID: model.NewID()})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Message(ctx, "a", v1.AppendInvestigationMessage{ID: "over-limit", ExpectedVersion: 1, Text: "extra"}); err == nil {
		t.Fatal("entry cap ignored")
	}
	v, _ := s.Read(ctx, "a")
	if len(v.Entries) != 200 || v.Version != 1 {
		b, _ := json.Marshal(v)
		t.Fatal("failed update mutated stored state", string(b))
	}
}
