import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  IntegrationEvidence,
  type IntegrationStatus,
} from "./integration-status";

test("successful collection never implies deployed or evaluating rules", () => {
  const status: IntegrationStatus = {
    revision: 1,
    checkedAt: "2026-10-21T10:00:00Z",
    definition: { state: "saved", message: "Revision 1" },
    collection: { state: "collected", message: "Provider intervals collected" },
    deployment: {
      state: "not_deployed",
      message: "Review and deploy rules",
      code: "rule_file_missing",
      nextAction:
        "Review the current plan before deploying to the shared directory.",
    },
    evaluation: { state: "unknown", message: "No runtime evidence" },
  };
  const html = renderToStaticMarkup(<IntegrationEvidence status={status} />);
  expect(html).toContain("Measurements collected");
  expect(html).toContain(">Collected<");
  expect(html).toContain(">Not deployed<");
  expect(html).toContain(">Unverified<");
  expect(html).not.toContain(">Evaluating<");
  expect(html).toContain("Next step:");
  expect(html).toContain(
    "Review the current plan before deploying to the shared directory.",
  );
  expect(html).not.toContain("rule_file_missing");
});

test("evaluation errors remain visible and provider text is escaped", () => {
  const stage = { state: "failed", message: "<script>untrusted</script>" };
  const html = renderToStaticMarkup(
    <IntegrationEvidence
      status={{
        revision: 2,
        checkedAt: "now",
        definition: stage,
        collection: stage,
        deployment: stage,
        evaluation: stage,
      }}
    />,
  );
  expect(html).toContain(">Failed<");
  expect(html).toContain("&lt;script&gt;");
  expect(html).not.toContain("<script>");
});

test("healthy runtime evaluation does not imply query dataset visibility", () => {
  const healthy = { state: "evaluating", message: "Current rules evaluated" };
  const html = renderToStaticMarkup(
    <IntegrationEvidence
      status={{
        revision: 3,
        checkedAt: "2026-10-22T12:00:00Z",
        definition: { state: "saved", message: "Revision 3" },
        collection: { state: "collected", message: "Source data collected" },
        deployment: { state: "deployed", message: "Destination matches" },
        evaluation: healthy,
        ruleOutput: {
          state: "not_observed",
          message: "No marker in configured query dataset",
          code: "rule_output_missing",
          nextAction: "Check query and ruler routing",
        },
      }}
    />,
  );
  expect(html).toContain(">Evaluating<");
  expect(html).toContain("Rule output in query dataset");
  expect(html).toContain(">Not observed<");
  expect(html).toContain("Check query and ruler routing");
  expect(html).not.toContain(">Verified<");
});

test("a matching file is labelled written while evaluation remains unloaded", () => {
  const html = renderToStaticMarkup(
    <IntegrationEvidence
      status={{
        revision: 1,
        checkedAt: "2026-10-23T00:00:00Z",
        definition: { state: "saved", message: "Saved" },
        collection: { state: "empty", message: "No measurements" },
        deployment: {
          state: "written",
          message: "File matches current definition",
        },
        evaluation: {
          state: "not_loaded",
          message: "Prometheus has not loaded the group",
        },
      }}
    />,
  );
  expect(html).toContain(">File written<");
  expect(html).toContain(">Not loaded<");
  expect(html).toContain(">No measurements<");
  expect(html).not.toContain(">Deployed<");
  expect(html).not.toContain(">Evaluating<");
});
