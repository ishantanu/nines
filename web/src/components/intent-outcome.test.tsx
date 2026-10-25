import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { IntentOutcomePanel } from "./intent-outcome";
import { createTenantSession } from "@/lib/api";
import type { IntentOutcome } from "@/lib/intent-conversations";
const report: IntentOutcome = {
  tenant: "alpha",
  conversation: "review",
  changeSetDigest: "digest",
  operation: "apply",
  operationDigest: "digest",
  sloId: "api",
  revision: 2,
  state: "regressed",
  code: "observed_attainment_decreased",
  message: "Observed attainment decreased; review causality before rollback.",
  startedAt: "2026-10-25T10:00:00Z",
  checkedAt: "2026-10-25T10:05:00Z",
  warmupUntil: "2026-10-25T10:02:00Z",
  deadlineAt: "2026-10-25T11:07:00Z",
  reliability: 0.99,
  baselineReliability: 0.999,
  comparison: "comparable_rolling_windows",
  rollbackStatus: "separate_review_required",
  limitations: [
    "Rolling windows can overlap; correlation does not establish causality.",
  ],
  evaluation: { state: "evaluating", message: "Current rules are evaluating." },
};
test("outcome shows measured regression and explicit rollback limits", () => {
  const html = renderToStaticMarkup(
    <IntentOutcomePanel
      report={report}
      scope={createTenantSession("key", "alpha", "viewer")}
      conversation="review"
    />,
  );
  expect(html).toContain("Post-change verification");
  expect(html).toContain("99.000%");
  expect(html).toContain("99.900%");
  expect(html).toContain("Current rules are evaluating.");
  expect(html).toContain("separate human review");
  expect(html).toContain("correlation does not establish causality");
});
test("outcome hides cross-tenant and cross-conversation evidence", () => {
  for (const [tenant, conversation] of [
    ["beta", "review"],
    ["alpha", "other"],
  ])
    expect(
      renderToStaticMarkup(
        <IntentOutcomePanel
          report={report}
          scope={createTenantSession("key", tenant, "viewer")}
          conversation={conversation}
        />,
      ),
    ).toBe("");
});
