import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { IntentAuthoring } from "./intent-authoring";
import { createTenantSession } from "@/lib/api";
import type {
  IntentConversation,
  IntentRecommendation,
} from "@/lib/intent-conversations";
import {
  IntentRecommendationReport,
  IntentDecisionControls,
  IntentAuditTimeline,
  IntentRollbackPanel,
} from "./intent-governance";

const report: IntentRecommendation = {
  summary: "Checkout success: 99.9% good events over 28 days",
  explanation: "<script>Untrusted model summary</script>",
  basis: ["Target comes from the reviewed user goal."],
  limitations: [
    "No matching historical backtest is attached.",
    "Consumed error budget cannot be restored.",
  ],
  mode: "model",
  runId: "prepare",
  modelRoute: {
    provider: "custom",
    model: "qwen",
    protocol: "openai-compatible",
    routeDigest: "route",
    local: true,
  },
  binding: { name: "requests", revision: 3, digest: "binding-digest" },
  sourceDigest: "source-digest",
  evidenceDigest: "evidence-digest",
  observedFrom: "2026-10-23T00:00:00Z",
  observedTo: "2026-10-23T00:15:00Z",
  fields: [
    {
      resource: "checkout",
      field: "spec.target",
      before: 0.99,
      after: 0.999,
      impact: "Higher target leaves less error budget.",
    },
  ],
};
const conversation: IntentConversation = {
  apiVersion: "nines.dev/v1alpha1",
  tenant: "alpha",
  id: "conversation",
  title: "Checkout",
  version: 4,
  state: "ready",
  createdAt: "now",
  updatedAt: "now",
  scope: {
    metadata: { name: "checkout", project: "test" },
    service: "api",
    source: "primary",
    population: { service: "api" },
    noDataPolicy: "unknown",
  },
  messages: [],
  runs: [],
  changeSet: {
    id: "change",
    digest: "a".repeat(64),
    state: "proposed",
    createdAt: "now",
    changes: [],
    recommendation: report,
  },
};
test("recommendation report distinguishes evidence, impact and limitations without executing model text", () => {
  const html = renderToStaticMarkup(
    <IntentRecommendationReport report={report} />,
  );
  for (const text of [
    "Why this recommendation",
    "99%",
    "99.9%",
    "Higher target leaves less error budget",
    "revision 3",
    "qwen",
    "evidence-digest",
    "No matching historical backtest",
  ])
    expect(html).toContain(text);
  expect(html).toContain("&lt;script&gt;");
  expect(html).not.toContain("<script>");
});
test("approval and rejection start disabled until a reviewer enters a reason and acknowledges the report", () => {
  const html = renderToStaticMarkup(
    <IntentDecisionControls
      scope={createTenantSession("token", "alpha", "owner")}
      conversation={conversation}
      busy={false}
      onAction={async () => {}}
    />,
  );
  expect(html).toContain("Review decision and reason");
  expect(html).toContain("I reviewed the rationale");
  expect(html).toMatch(
    /<button[^>]*disabled=""[^>]*>Approve changes<\/button>/,
  );
  expect(html).toMatch(/<button[^>]*disabled=""[^>]*>Reject changes<\/button>/);
});
test("withdrawn approvals cannot appear as applied and agents do not receive an enabled approval control", () => {
  const value = {
    ...conversation,
    changeSet: {
      ...conversation.changeSet!,
      state: "rejected" as const,
      rejectionReason: "Telemetry is incomplete.",
      rejectedBy: { name: "Reviewer" },
    },
  };
  const html = renderToStaticMarkup(
    <IntentDecisionControls
      scope={createTenantSession("agent", "alpha", "agent")}
      conversation={value}
      busy={false}
      onAction={async () => {}}
    />,
  );
  expect(html).toContain("Rejected by Reviewer");
  expect(html).toContain("Telemetry is incomplete");
  expect(html).not.toContain("Apply approved changes");
});
test("new-resource rollback remains visibly blocked and cannot expose a partial GitOps export", () => {
  const change = {
    ...conversation.changeSet!,
    state: "applied" as const,
    rollback: {
      id: "rollback",
      digest: "b".repeat(64),
      originalDigest: "a".repeat(64),
      state: "blocked" as const,
      reason: "Investigate regression",
      createdAt: "now",
      createdBy: { name: "Owner" },
      changes: [],
      blockers: [
        "No previous definition. Resource retirement is not implemented.",
      ],
    },
  };
  const html = renderToStaticMarkup(
    <IntentRollbackPanel
      scope={createTenantSession("token", "alpha", "owner")}
      conversation={conversation}
      change={change}
      busy={false}
      running={false}
      onAction={async () => {}}
      onRead={() => {}}
    />,
  );
  expect(html).toContain("Rollback unavailable. No resources changed.");
  expect(html).toContain(
    "Existing external rules require a separate Rules review",
  );
  expect(html).not.toContain("Approve rollback");
  expect(html).not.toContain("Download rollback Kubernetes YAML");
});
test("decision audit shows actor, outcome, reason, revision and hash-chain references", () => {
  const html = renderToStaticMarkup(
    <IntentAuditTimeline
      events={[
        {
          sequence: 1,
          at: "2026-10-23T00:00:00Z",
          actor: { subject: "reviewer-1", name: "Engineer", kind: "user" },
          action: "change_set.approved",
          outcome: "succeeded",
          reason: "Checked traffic exclusions and target impact.",
          changeSetDigest: "exact-digest",
          version: 4,
          hash: "event-hash",
        },
      ]}
    />,
  );
  for (const text of [
    "Engineer",
    "approved",
    "succeeded",
    "traffic exclusions",
    "conversation version 4",
    "exact-digest",
    "event-hash",
  ])
    expect(html).toContain(text);
});

test("intent definition form explains its read-only preview and approval handoff", () => {
  const html = renderToStaticMarkup(
    <IntentAuthoring
      scope={createTenantSession("fixture", "alpha", "owner")}
      journeys={[]}
      onStartConversation={() => {}}
    />,
  );
  expect(html).toContain("This form does not apply definitions.");
  expect(html).toContain("Start conversation from this goal");
  expect(html).not.toContain("Apply reviewed SLO");
  expect(html).not.toContain("Apply reviewed journey link");
});
