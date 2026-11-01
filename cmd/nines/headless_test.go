package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/githubreview"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

type headlessTelemetry struct{ *testtelemetry.Source }

func (s *headlessTelemetry) CollectWindows(ctx context.Context, slo model.SLO, from, to time.Time) ([]model.WindowObservation, error) {
	return s.CollectWindowRange(ctx, slo, from, to, slo.StepSeconds)
}

// This uses the real Cobra commands, Go client, tenant middleware, workflow and
// compiler. Only the telemetry backend is synthetic; no browser/model is used.
func TestHeadlessCLIReviewedChangeAndRollback(t *testing.T) {
	ctx := context.Background()
	platform, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	platform.TenantID = "alpha"
	platform.Collector = &headlessTelemetry{source}
	flow := intentflow.New(platform, intentflow.NewMemory())
	handler, err := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Headless acceptance", Service: platform, IntentConversations: flow, Grants: []tenancy.Grant{{Token: "headless-owner", Role: "owner"}, {Token: "headless-applier", Role: "editor"}, {Token: "headless-agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "alpha")
	dir := t.TempDir()
	execute := func(token, action string, input any, out any) error {
		t.Helper()
		t.Setenv("NINES_API_KEY", token)
		t.Setenv("API_TOKEN", "")
		args := []string{"intent", "conversations", action}
		if action != "create" {
			args = append(args, "headless-change")
		}
		if input != nil {
			b, e := json.Marshal(input)
			if e != nil {
				t.Fatal(e)
			}
			file := filepath.Join(dir, "request.json")
			if e = os.WriteFile(file, b, 0600); e != nil {
				t.Fatal(e)
			}
			args = append(args, "-f", file)
		}
		args = append(args, "--agent")
		var output bytes.Buffer
		if e := run(args, &output); e != nil {
			return e
		}
		if out != nil {
			if e := json.Unmarshal(output.Bytes(), out); e != nil {
				t.Fatalf("%s: %s: %v", action, output.String(), e)
			}
		}
		return nil
	}
	var v v1.IntentConversation
	call := func(token, action string, in any) {
		t.Helper()
		if err := execute(token, action, in, &v); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	scope := v1.IntentAuthoringScope{Metadata: v1.Metadata{Name: "headless-objective", Project: "test"}, Service: "api", Source: "primary", Population: map[string]string{"service": "api"}, NoDataPolicy: "unknown"}
	goal := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: scope.Metadata, Spec: v1.IntentSpec{Description: "99.9% of API attempts succeed over 1 hour", Service: scope.Service, Source: scope.Source, Population: scope.Population, NoDataPolicy: scope.NoDataPolicy, Outcome: "availability", Target: .999, WindowSeconds: 3600}}
	call("headless-agent", "create", v1.CreateIntentConversation{ID: "headless-change", Title: "Review API availability", Text: goal.Spec.Description, Scope: scope, Goal: &goal})
	work := func(n int) {
		t.Helper()
		for range n {
			found, e := flow.WorkOne(ctx)
			if !found || e != nil {
				t.Fatal(found, e)
			}
		}
		call("headless-agent", "show", nil)
	}
	call("headless-agent", "run", v1.CreateIntentFlowRun{ID: "discover", ExpectedVersion: v.Version, Mode: "guided"})
	work(1)
	var status v1.IntentWorkflowStatus
	if err := execute("headless-agent", "status", nil, &status); err != nil || status.NextAction != "confirm" || !status.HumanRequired {
		t.Fatal(status, err)
	}
	confirm := v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, Binding: v1.IntentSelection{Name: "requests", ExpectedRevision: 1}, Confirm: true}
	if err := execute("headless-agent", "confirm", confirm, nil); err == nil {
		t.Fatal("agent confirmed human measurement review")
	}
	call("headless-owner", "confirm", confirm)
	call("headless-agent", "run", v1.CreateIntentFlowRun{ID: "compile", ExpectedVersion: v.Version, Mode: "guided"})
	work(2)
	// Seed the existing production definition and usable telemetry as fixture setup.
	// The tested change then updates it and restores it as a new revision.
	b, _ := json.Marshal(v.Plan.Compiled.Manifest)
	old, e := model.ParseDefinition(b)
	if e != nil {
		t.Fatal(e)
	}
	old.SLO.Spec.Target = .99
	old.SLO.Spec.Intent.Target = .99
	if _, e = platform.ApplyDefinition(ctx, old, 0); e != nil {
		t.Fatal(e)
	}
	end := platform.Now().UTC().Truncate(time.Minute)
	if _, e = platform.Collect(ctx, scope.Metadata.Name, end.Add(-2*time.Minute), end); e != nil {
		t.Fatal(e)
	}
	if _, e = platform.CheckMeasurement(ctx, scope.Metadata.Name); e != nil {
		t.Fatal(e)
	}
	call("headless-agent", "run", v1.CreateIntentFlowRun{ID: "recompile", ExpectedVersion: v.Version, Mode: "guided"})
	work(2)
	call("headless-agent", "changes", map[string]int64{"expectedVersion": v.Version})
	review := func(digest, reason string) v1.ReviewIntentChangeSet {
		return v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: digest, Confirm: true, Reason: reason}
	}
	var artifact v1.IntentReviewArtifact
	if err := execute("headless-agent", "report", nil, &artifact); err != nil || artifact.ChangeSet.Digest != v.ChangeSet.Digest || artifact.AuditIntegrity != "verified" || artifact.Status.NextAction != "approve" {
		t.Fatal(artifact, err)
	}
	var githubOutput bytes.Buffer
	if err := run([]string{"intent", "conversations", "github-plan", "headless-change", "--repository", "owner/repo", "--pull-request", "7", "--head", strings.Repeat("c", 40), "--agent"}, &githubOutput); err != nil {
		t.Fatal(err)
	}
	var githubPlan githubreview.Plan
	if err := json.Unmarshal(githubOutput.Bytes(), &githubPlan); err != nil || githubPlan.Validate() != nil || githubPlan.Version != v.Version || githubPlan.ChangeSetDigest != v.ChangeSet.Digest {
		t.Fatal("prepared server evidence did not produce the exact GitHub review plan", err)
	}
	digest := v.ChangeSet.Digest
	if err := execute("headless-agent", "approve", review(digest, "Agent cannot approve"), nil); err == nil {
		t.Fatal("agent approved")
	}
	call("headless-owner", "approve", review(digest, "Reviewed the target, event population, evidence and rollback boundary."))
	if err := execute("headless-owner", "apply", review(digest, ""), nil); err == nil {
		t.Fatal("approver applied own change")
	}
	if err := execute("headless-applier", "apply", review(strings.Repeat("0", 64), ""), nil); err == nil {
		t.Fatal("unreviewed digest applied")
	}
	apply := review(digest, "")
	call("headless-applier", "apply", apply)
	call("headless-applier", "apply", apply) // Lost-response retry must not create another revision.
	check := func(target float64, revision int64) {
		t.Helper()
		actual, e := platform.Store.Read(ctx, scope.Metadata.Name)
		if e != nil || actual.SLO.Target != target || actual.SLO.Revision != revision {
			t.Fatal(actual, e)
		}
	}
	check(.999, 2)
	var verification v1.IntentChangeSetVerification
	if e = execute("headless-agent", "verify", nil, &verification); e != nil || verification.SourceState != "matches" || verification.Resources[0].State != "matches" {
		t.Fatal(verification, e)
	}
	if e = execute("headless-agent", "status", nil, &status); e != nil || status.Phase != "observing" {
		t.Fatal(status, e)
	}
	call("headless-owner", "observe", review(digest, "Inspect fresh post-change evidence."))
	if v.ChangeSet.Outcome == nil || v.ChangeSet.Outcome.State == "verified" {
		t.Fatal("configuration apply was treated as runtime success")
	}
	call("headless-owner", "rollback-plan", review(digest, "Restore the reviewed previous target while runtime evidence is investigated."))
	restore := v.ChangeSet.Rollback.Digest
	if err := execute("headless-agent", "rollback-approve", review(restore, "Agent cannot approve restore"), nil); err == nil {
		t.Fatal("agent approved rollback")
	}
	call("headless-owner", "rollback-approve", review(restore, "Reviewed exact previous definition and revision fences."))
	apply = review(restore, "")
	call("headless-applier", "rollback-apply", apply)
	call("headless-applier", "rollback-apply", apply)
	check(.99, 3)
	if e = execute("headless-agent", "verify", nil, &verification); e != nil || verification.Operation != "rollback" || verification.Resources[0].State != "matches" {
		t.Fatal(verification, e)
	}
	var audit v1.IntentAuditLog
	if e = execute("headless-owner", "audit", nil, &audit); e != nil || audit.Integrity != "verified" {
		t.Fatal(audit, e)
	}
	if v.ChangeSet.Rollback.AppliedBy == nil || v.ChangeSet.Rollback.ApprovedBy == nil || v.ChangeSet.Rollback.AppliedBy.Subject == v.ChangeSet.Rollback.ApprovedBy.Subject {
		t.Fatal("missing independent rollback receipt")
	}
}
