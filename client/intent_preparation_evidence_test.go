package client_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentEvidenceAttachmentClientHTTPAndRBAC(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	s.TenantID = "alpha"
	flow := intentflow.New(s, intentflow.NewMemory())
	h, err := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Alpha", Service: s, IntentConversations: flow, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	owner, err := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	input, err := testtelemetry.EvidenceInput(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	goal := input.Plan.Intent
	v, err := owner.CreateIntentConversation(ctx, v1.CreateIntentConversation{ID: "receipt-http", Title: "Receipt acceptance", Text: goal.Spec.Description, Goal: &goal, Scope: v1.IntentAuthoringScope{Metadata: goal.Metadata, Service: goal.Spec.Service, Source: goal.Spec.Source, Population: goal.Spec.Measurement.Labels, NoDataPolicy: goal.Spec.NoDataPolicy}})
	if err != nil {
		t.Fatal(err)
	}
	for phase := 0; phase < 2; phase++ {
		runID := []string{"discovery", "compile"}[phase]
		v, err = owner.StartIntentFlowRun(ctx, v.ID, v1.CreateIntentFlowRun{ID: runID, ExpectedVersion: v.Version, Mode: "guided"})
		if err != nil {
			t.Fatal(err)
		}
		steps := 1 + phase
		for i := 0; i < steps; i++ {
			if found, err := flow.WorkOne(ctx); err != nil || !found {
				t.Fatalf("worker: %v %v", found, err)
			}
		}
		v, err = owner.IntentConversation(ctx, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if phase == 0 {
			v, err = owner.ConfirmIntentBinding(ctx, v.ID, v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, MeasurementDigest: v.Plan.Compiled.MeasurementDigest, Confirm: true})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	in := v1.RecordIntentEvidence{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, ReadTelemetry: true}
	viewer, _ := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if _, err = viewer.RecordIntentEvidence(ctx, v.ID, in); err == nil {
		t.Fatal("viewer collected evidence")
	}
	agent, _ := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "agent"})
	saved, err := agent.RecordIntentEvidence(ctx, v.ID, in)
	if err != nil || saved.PreparationEvidence == nil || saved.PreparationEvidence.Report.Tenant != "alpha" || saved.PreparationEvidence.CapturedVersion != v.Version || saved.PreparationEvidence.RequestedBy.Kind != "api_token" {
		t.Fatalf("missing scoped receipt: %v %#v", err, saved.PreparationEvidence)
	}
	if _, err = agent.RecordIntentEvidence(ctx, v.ID, in); err == nil {
		t.Fatal("stale retry accepted")
	}
	frozen, err := owner.PrepareIntentChangeSet(ctx, saved.ID, saved.Version)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := owner.IntentReviewArtifact(ctx, saved.ID)
	if err != nil || artifact.ChangeSet.Recommendation.ReviewPacket == nil || artifact.ChangeSet.Recommendation.ReviewPacket.Goal.Spec.Target != goal.Spec.Target || artifact.ChangeSet.Digest != frozen.ChangeSet.Digest {
		t.Fatal("SDK lost the server-frozen review packet", err)
	}
	clarified, err := owner.AppendIntentConversation(ctx, v.ID, v1.AppendIntentConversation{ID: "sdk-clarification", ExpectedVersion: frozen.Version, Text: "Retry attempts are measured separately.", PreserveGoal: true})
	if err != nil || clarified.Goal == nil || clarified.Goal.Spec.Target != goal.Spec.Target || clarified.Selection != nil || clarified.ChangeSet != nil {
		t.Fatal("SDK clarification lost typed facts or retained approval", err)
	}

}
