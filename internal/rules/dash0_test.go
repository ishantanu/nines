package rules

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/openslo"
)

type dash0Transport func(*http.Request) (*http.Response, error)

func (f dash0Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestReviewedDash0PublicationFencesPlansCredentialsAndWrites(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	var saved *openslo.Document
	writes := 0
	http.DefaultTransport = dash0Transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.eu-west-1.aws.dash0.com" || r.URL.Query().Get("dataset") != "production" {
			t.Fatal("untrusted route or credential")
		}
		w := httptest.NewRecorder()
		if r.Method == http.MethodGet {
			if saved == nil {
				w.WriteHeader(404)
			} else if err := json.NewEncoder(w).Encode(saved); err != nil {
				t.Fatal(err)
			}
			return w.Result(), nil
		}
		var doc openslo.Document
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		if saved != nil && doc.Metadata.Labels["dash0.com/version"] != saved.Metadata.Labels["dash0.com/version"] {
			w.WriteHeader(400)
			return w.Result(), nil
		}
		writes++
		doc.Metadata.Labels["dash0.com/version"] = strconv.Itoa(writes)
		doc.APIVersion = "openslo.com/v1"
		saved = &doc
		return w.Result(), nil
	})
	s := fixture()
	s.Source = "primary"
	s.WindowSeconds = 28 * 86400
	s.GoodQuery = ""
	s.TotalQuery = ""
	s.ProviderWindow = &v1.ProviderWindow{GoodSelector: `requests_total{code="200"}`, TotalSelector: `requests_total`, ScrapeIntervalSeconds: 15}
	m := &Manager{Tenant: "team-a", Targets: map[string]Target{"primary": {Kind: "dash0", Connection: "primary", QueryURL: "https://api.eu-west-1.aws.dash0.com", Dataset: "production", Namespace: "nines-prod", BearerToken: "synthetic-publisher-key"}}}
	ctx := context.Background()
	plan, err := m.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NativeSLO == nil || plan.Group.Rules == nil || len(plan.Group.Rules) != 0 {
		t.Fatal("native plan masquerades as recording rules", plan)
	}
	if _, err = m.Deploy(ctx, s, "wrong"); !errors.Is(err, model.ErrRevisionConflict) || writes != 0 {
		t.Fatal("unreviewed deployment", err, writes)
	}
	pinned := false
	m.PublicationGuard = func(context.Context, model.SLO, Target) error { pinned = true; return nil }
	result, err := m.Deploy(ctx, s, plan.Digest)
	if err != nil || result.State != "stored" || result.Evaluation.State != "unknown" || !pinned || writes != 1 {
		t.Fatal(result, err, writes, pinned)
	}
	if _, err = m.Deploy(ctx, s, plan.Digest); !errors.Is(err, model.ErrRevisionConflict) || writes != 1 {
		t.Fatal("stale plan reused", err, writes)
	}
	plan, err = m.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	m.BeforeWrite = func() error { return model.ErrRevisionConflict }
	if _, err = m.Deploy(ctx, s, plan.Digest); !errors.Is(err, model.ErrRevisionConflict) || writes != 1 {
		t.Fatal("write guard bypass", err, writes)
	}
	m.BeforeWrite = nil
	originalTarget := m.Targets["primary"]
	changed := originalTarget
	changed.BearerToken = "rotated-key"
	m.Targets["primary"] = changed
	// The plan includes credential identity even though secrets are never returned.
	other, err := m.PlanContext(ctx, s)
	if err != nil || other.Digest == plan.Digest {
		t.Fatal("credential rotation did not invalidate plan", err)
	}
	if _, err = m.Deploy(ctx, s, plan.Digest); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("credential change reused reviewed digest", err)
	}
	m.Targets["primary"] = originalTarget
	result, err = m.Withdraw(ctx, s, plan.Digest)
	if err != nil || result.State != "withdrawn" || saved.Metadata.Annotations["dash0.com/enabled"] != "false" || result.Evaluation.State != "unknown" {
		t.Fatal(result, err)
	}
	if _, err = m.OutputIdentity(ctx, s); err == nil {
		t.Fatal("native SLO claimed recording-rule output")
	}
}
