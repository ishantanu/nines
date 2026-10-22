package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestTopologyStoreCASAndHistory(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var st store.Store = store.NewMemory()
			dsn := ""
			if backend == "postgres" {
				st, dsn = postgres(t)
			}
			top := st.(store.TopologyStore)
			ctx := context.Background()
			r := model.TopologyRevision{Kind: v1.BindingKind, ID: "same", Digest: "one", Manifest: json.RawMessage(`{"spec":{"displayName":"First"}}`)}
			if _, err := top.ApplyTopology(ctx, r, 0); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for _, digest := range []string{"two", "three"} {
				wg.Add(1)
				go func(d string) {
					defer wg.Done()
					next := r
					next.Digest = d
					_, err := top.ApplyTopology(ctx, next, 1)
					errs <- err
				}(digest)
			}
			wg.Wait()
			close(errs)
			ok, conflict := 0, 0
			for err := range errs {
				if err == nil {
					ok++
				} else if errors.Is(err, model.ErrRevisionConflict) {
					conflict++
				} else {
					t.Fatal(err)
				}
			}
			if ok != 1 || conflict != 1 {
				t.Fatal(ok, conflict)
			}
			old, err := top.ReadTopology(ctx, v1.BindingKind, "same", 1)
			if err != nil || old.Digest != "one" {
				t.Fatal(old, err)
			}
			old.Manifest[0] = 'x'
			again, _ := top.ReadTopology(ctx, v1.BindingKind, "same", 1)
			if again.Manifest[0] != '{' {
				t.Fatal("mutable history")
			}
			r.Kind = v1.DependencyMapKind
			if _, err = top.ApplyTopology(ctx, r, 0); err != nil {
				t.Fatal(err)
			}
			page, err := top.ListTopology(ctx, "", "", 1)
			if err != nil || len(page) != 1 {
				t.Fatal(page, err)
			}
			next, err := top.ListTopology(ctx, "", page[0].Kind+"/"+page[0].ID, 1)
			if err != nil || len(next) != 1 {
				t.Fatal(next, err)
			}
			if dsn != "" {
				reopened, err := store.NewPostgres(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				last, err := reopened.ReadTopology(ctx, v1.BindingKind, "same", 0)
				if err != nil || last.Revision != 2 {
					t.Fatal(last, err)
				}
			}
		})
	}
}
