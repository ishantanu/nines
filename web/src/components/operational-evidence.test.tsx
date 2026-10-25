import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { BudgetDecisionPanel } from "./budget-decision";
import { MeasurementQualityPanel } from "./measurement-quality";
import {
  createTenantSession,
  type MeasurementQuality,
  type ErrorBudgetDecision,
} from "@/lib/api";
const quality: MeasurementQuality = {
  apiVersion: "nines.dev/v1alpha1",
  tenant: "alpha",
  sloId: "api",
  revision: 3,
  definitionDigest: "definition",
  sourceDigest: "source",
  observedAt: "2026-10-24T12:00:00Z",
  evaluatedAt: "2026-10-24T11:59:00Z",
  state: "limited",
  good: 999,
  total: 1000,
  minimumEvents: 2000,
  coverage: 0.8,
  freshness: {
    state: "fresh",
    ageAtObservationSeconds: 60,
    staleAfterSeconds: 135,
  },
  checks: [
    {
      code: "denominator",
      state: "warning",
      message: "Below the configured count requirement.",
    },
  ],
  limitations: [],
};
const decision: ErrorBudgetDecision = {
  apiVersion: "nines.dev/v1alpha1",
  tenant: "alpha",
  sloId: "api",
  revision: 3,
  definitionDigest: "definition",
  sourceDigest: "source",
  policyDigest: "policy",
  observedAt: quality.observedAt,
  evaluatedAt: quality.evaluatedAt,
  state: "insufficient_evidence",
  reasonCodes: ["coverage"],
  message: "Current evidence is insufficient.",
  budgetRemainingFraction: 0.5,
  qualityState: "limited",
  advisory: true,
};
test("quality separates traffic and partial coverage from confidence", () => {
  const html = renderToStaticMarkup(
    <MeasurementQualityPanel
      report={quality}
      scope={createTenantSession("key", "alpha", "viewer")}
      cohortLabels={["region"]}
    />,
  );
  expect(html).toContain("1,000 / 2,000 minimum");
  expect(html).toContain("80.00%");
  expect(html).toContain("Inspect cohorts");
  expect(html).toContain("do not establish statistical confidence");
  expect(html).toContain("Below the configured count requirement");
  expect(html).not.toContain("healthy");
});
test("quality hides evidence from a different active tenant", () => {
  expect(
    renderToStaticMarkup(
      <MeasurementQualityPanel
        report={quality}
        scope={createTenantSession("key", "beta", "viewer")}
      />,
    ),
  ).toBe("");
});
test("budget evidence remains explicit about authority and frozen timestamps", () => {
  let html = renderToStaticMarkup(<BudgetDecisionPanel decision={decision} />);
  expect(html).toContain("Insufficient evidence");
  expect(html).toContain("50.00%");
  expect(html).toContain("grants no deployment permission");
  html = renderToStaticMarkup(
    <BudgetDecisionPanel decision={decision} frozen />,
  );
  expect(html).toContain("proposal preparation");
  expect(html).toContain("Read current evidence before release");
});
