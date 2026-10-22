import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ReviewRunResult } from "./review-runs";
import type { ReviewRun } from "@/lib/investigations";
const run: ReviewRun = {
  apiVersion: "nines.dev/v1alpha1",
  tenant: "alpha",
  id: "run-1",
  investigationId: "review-1",
  version: 7,
  reviewVersion: 1,
  scope: {
    kind: "ServiceLevelObjective",
    resourceId: "checkout",
    expectedRevision: 2,
    at: "2026-10-20T10:00:00Z",
  },
  requestedBy: { subject: "user-1", name: "Operator", kind: "user" },
  status: "completed",
  createdAt: "2026-10-20T10:00:00Z",
  updatedAt: "2026-10-20T10:00:01Z",
  deadlineAt: "2026-10-20T10:05:00Z",
  attempts: 2,
  steps: [
    { tool: "read_context", status: "completed", attempts: 1 },
    {
      tool: "assess_evidence",
      status: "completed",
      attempts: 1,
      assessment: {
        state: "insufficient_evidence",
        evidenceDigest: "saved-digest",
        findings: [
          {
            resourceId: "checkout",
            state: "insufficient_evidence",
            message:
              "No traffic was observed. Zero traffic does not establish success.",
          },
        ],
        limitations: ["Only stored context was assessed."],
      },
    },
  ],
};
test("a completed run can still have insufficient reliability evidence", () => {
  const html = renderToStaticMarkup(<ReviewRunResult run={run} />);
  expect(html).toContain("completed");
  expect(html).toContain("Insufficient evidence");
  expect(html).toContain("Zero traffic does not establish success");
  expect(html).toContain("Only stored context was assessed");
  expect(html).not.toContain("healthy");
});
test("cancelled runs show actual cancellation provenance without claiming results", () => {
  const cancelled = {
    ...run,
    status: "cancelled" as const,
    cancelledBy: { subject: "owner", name: "Workspace owner", kind: "user" },
    steps: [
      { tool: "read_context", status: "running", attempts: 1 },
      { tool: "assess_evidence", status: "pending", attempts: 0 },
    ],
  };
  const html = renderToStaticMarkup(<ReviewRunResult run={cancelled} />);
  expect(html).toContain("Cancelled by Workspace owner");
  expect(html).toContain("Uncommitted results were discarded");
  expect(html).not.toContain("Captured evidence");
  expect(html).not.toContain("Insufficient evidence");
});

test("model prose is escaped and distinct from the calculated assessment", () => {
  const html = renderToStaticMarkup(
    <ReviewRunResult
      run={{
        ...run,
        mode: "model",
        question: "Explain the captured window",
        modelRoute: {
          provider: "custom",
          model: "local-fixture",
          protocol: "chat-completions",
          local: true,
        },
        modelReview: {
          summary: "<script>unsupported cause</script>",
          evidenceDigests: ["captured-digest"],
          limitations: ["A hypothesis requires verification."],
        },
      }}
    />,
  );
  expect(html).toContain("Model interpretation");
  expect(html).toContain("Explain the captured window");
  expect(html).toContain("Insufficient evidence");
  expect(html).toContain("&lt;script&gt;");
  expect(html).not.toContain("<script>");
  expect(html).toContain("captured-digest");
  expect(html).toContain("A hypothesis requires verification");
});

test("a skill receipt proves the contract without claiming a successful SLO", () => {
  const html = renderToStaticMarkup(
    <ReviewRunResult
      run={{
        ...run,
        skill: {
          id: "measurement-review",
          version: "1.0.0",
          digest: "pinned-skill",
        },
        skillResult: {
          inputDigest: "input",
          evidenceDigests: ["saved-digest"],
          completedTools: ["read_assessment"],
          outputDigest: "exact-output",
        },
      }}
    />,
  );
  expect(html).toContain("measurement-review");
  expect(html).toContain("pinned-skill");
  expect(html).toContain("exact-output");
  expect(html).toContain("Evidence obligations and output schema verified");
  expect(html).toContain("Insufficient evidence");
  expect(html).not.toContain("healthy");
});
