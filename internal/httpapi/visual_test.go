package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/client"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/systemone"
	"github.com/ishantanu/nines/internal/tenancy"
)

type visualStub struct {
	calls atomic.Int32
	err   error
}

func (v *visualStub) Descriptor() v1.SystemOneRoute { return v.VisualDescriptor() }
func (v *visualStub) Assess(context.Context, systemone.Input) (v1.SystemOneAssessment, error) {
	return v1.SystemOneAssessment{}, nil
}
func (v *visualStub) VisualEnabled() bool { return true }
func (v *visualStub) VisualDescriptor() v1.SystemOneRoute {
	return v1.SystemOneRoute{Provider: "typesafe", Model: systemone.DefaultModel, ContractDigest: "fixture"}
}
func (v *visualStub) AssessVisual(_ context.Context, in systemone.VisualInput) (v1.VisualAssessment, error) {
	v.calls.Add(1)
	return v1.VisualAssessment{Status: "assessed", ResolvedModel: systemone.DefaultModel, Judgments: &v1.VisualJudgments{SuccessVisible: .9, ErrorVisible: .1, EvidenceSufficient: .8}}, v.err
}
func visualHandler(t *testing.T) (http.Handler, *visualStub, []*store.Memory) {
	t.Helper()
	stub := &visualStub{}
	workspaces := []Workspace{}
	stores := []*store.Memory{}
	for _, id := range []string{"alpha", "beta"} {
		st := store.NewMemory()
		stores = append(stores, st)
		s := service.New(st, nil)
		s.TenantID = id
		j := model.Journey{APIVersion: v1.APIVersion, Kind: v1.JourneyKind, Metadata: model.ResourceMetadata{Name: "checkout", Project: "p"}}
		if _, e := st.ApplyJourney(context.Background(), model.JourneyRevision{Manifest: j, Digest: model.JourneyDigest(j), AppliedAt: time.Now()}, 0); e != nil {
			t.Fatal(e)
		}
		flow := intentflow.New(s, intentflow.NewMemory())
		flow.SystemOne = stub
		workspaces = append(workspaces, Workspace{ID: id, Name: id, Service: s, IntentConversations: flow, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "editor", Role: "editor"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}})
	}
	h, e := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	return h, stub, stores
}
func visualInput(t *testing.T) v1.CreateVisualEvaluation {
	t.Helper()
	var imageBytes bytes.Buffer
	_ = png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 280, 280)))
	return v1.CreateVisualEvaluation{RouteDigest: model.DefinitionDigest((&visualStub{}).VisualDescriptor()), ID: model.NewID(), JourneyRevision: 1, SuccessCriterion: "Synthetic confirmation is visible", ImageBase64: base64.StdEncoding.EncodeToString(imageBytes.Bytes()), Consent: true, NoticeVersion: v1.VisualConsentNotice}
}
func visualJSON(t *testing.T, in any) string {
	t.Helper()
	b, e := json.Marshal(in)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestVisualConsentScopeIdempotencyAndHumanReview(t *testing.T) {
	h, stub, stores := visualHandler(t)
	in := visualInput(t)
	path := "/api/journeys/checkout/visual-evaluations"
	for _, role := range []string{"viewer", "agent"} {
		if w := invRequest(h, "POST", path, visualJSON(t, in), role, "alpha"); w.Code != 403 {
			t.Fatal(role, w.Code)
		}
	}
	for _, edit := range []func(*v1.CreateVisualEvaluation){func(v *v1.CreateVisualEvaluation) { v.Consent = false }, func(v *v1.CreateVisualEvaluation) { v.NoticeVersion = "old" }, func(v *v1.CreateVisualEvaluation) { v.RouteDigest = "stale" }, func(v *v1.CreateVisualEvaluation) { v.ImageBase64 = "invalid" }, func(v *v1.CreateVisualEvaluation) { v.JourneyRevision = 999 }} {
		bad := in
		edit(&bad)
		if w := invRequest(h, "POST", path, visualJSON(t, bad), "owner", "alpha"); w.Code < 400 {
			t.Fatal("invalid consent/input accepted", w.Code)
		}
	}
	if stub.calls.Load() != 0 {
		t.Fatal("rejected input reached provider")
	}
	var receipt v1.VisualEvaluation
	for _, want := range []int{201, 200} {
		w := invRequest(h, "POST", path, visualJSON(t, in), "owner", "alpha")
		if w.Code != want || json.Unmarshal(w.Body.Bytes(), &receipt) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if stub.calls.Load() != 1 || receipt.Consent.Actor.Subject == "" || receipt.Consent.NoticeVersion != v1.VisualConsentNotice || receipt.Assessment.Status != "assessed" {
		t.Fatal("bad consent receipt", receipt)
	}
	if raw := visualJSON(t, receipt); strings.Contains(raw, in.ImageBase64) || strings.Contains(raw, in.SuccessCriterion) {
		t.Fatal("private visual input persisted")
	}
	in.SuccessCriterion = "Changed criterion"
	if w := invRequest(h, "POST", path, visualJSON(t, in), "owner", "alpha"); w.Code != 409 {
		t.Fatal("changed request ID reused", w.Code)
	}
	w := invRequest(h, "GET", path, "", "viewer", "beta")
	if w.Code != 200 || strings.Contains(w.Body.String(), `"items":[]`) == false {
		t.Fatal("cross tenant receipt", w.Code, w.Body.String())
	}
	review := v1.ReviewVisualEvaluation{RequestDigest: receipt.RequestDigest, Outcome: "inconclusive", Reason: "The screenshot cannot confirm backend completion."}
	reviewPath := path + "/" + receipt.ID + "/review"
	if w = invRequest(h, "POST", reviewPath, visualJSON(t, review), "agent", "alpha"); w.Code != 403 {
		t.Fatal("agent reviewed own output")
	}
	if w = invRequest(h, "POST", reviewPath, visualJSON(t, review), "editor", "beta"); w.Code != 404 {
		t.Fatal("cross tenant review")
	}
	if w = invRequest(h, "POST", reviewPath, visualJSON(t, review), "editor", "alpha"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	review.Outcome = "success"
	if w = invRequest(h, "POST", reviewPath, visualJSON(t, review), "editor", "alpha"); w.Code != 409 {
		t.Fatal("review rewritten")
	}
	runs, e := stores[0].Runs(context.Background(), "checkout")
	if e != nil || len(runs) != 0 {
		t.Fatal("model judgment became run evidence")
	}
}
func TestVisualProviderFailureIsDurableAndNotRetried(t *testing.T) {
	h, stub, stores := visualHandler(t)
	stub.err = errors.New("private provider diagnostic")
	in := visualInput(t)
	for range 2 {
		w := invRequest(h, "POST", "/api/journeys/checkout/visual-evaluations", visualJSON(t, in), "owner", "alpha")
		if w.Code != 200 && w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "private provider diagnostic") {
			t.Fatal("raw provider error exposed")
		}
	}
	r, e := stores[0].ReadVisual(context.Background(), in.ID)
	if e != nil || r.Assessment.Status != "unavailable" || stub.calls.Load() != 1 {
		t.Fatal(r, e, stub.calls.Load())
	}
}

func TestVisualGoClientUsesScopedEnvelopes(t *testing.T) {
	h, _, _ := visualHandler(t)
	server := httptest.NewServer(h)
	defer server.Close()
	api, e := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "owner"})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	caps, e := api.VisualCapabilities(ctx)
	if e != nil || !caps.Enabled || len(caps.RouteDigest) != 64 {
		t.Fatal(caps, e)
	}
	rows, e := api.VisualEvaluations(ctx, "checkout")
	if e != nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
	in := visualInput(t)
	r, e := api.AssessVisual(ctx, "checkout", in)
	if e != nil || r.ID != in.ID {
		t.Fatal(r, e)
	}
	rows, e = api.VisualEvaluations(ctx, "checkout")
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	r, e = api.ReviewVisual(ctx, "checkout", r.ID, v1.ReviewVisualEvaluation{RequestDigest: r.RequestDigest, Outcome: "inconclusive", Reason: "Inspected original evidence."})
	if e != nil || r.Review == nil {
		t.Fatal(r, e)
	}
}

func TestVisualStaleRouteConsentNeverCallsProvider(t *testing.T) {
	h, stub, _ := visualHandler(t)
	in := visualInput(t)
	in.RouteDigest = "stale"
	w := invRequest(h, "POST", "/api/journeys/checkout/visual-evaluations", visualJSON(t, in), "owner", "alpha")
	if w.Code != 400 || stub.calls.Load() != 0 {
		t.Fatal("stale provider consent reached model", w.Code, stub.calls.Load())
	}
}
