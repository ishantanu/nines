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
    deployment: { state: "not_deployed", message: "Review and deploy rules" },
    evaluation: { state: "unknown", message: "No runtime evidence" },
  };
  const html = renderToStaticMarkup(<IntegrationEvidence status={status} />);
  expect(html).toContain("Measurements collected");
  expect(html).toContain(">Collected<");
  expect(html).toContain(">Not deployed<");
  expect(html).toContain(">Unverified<");
  expect(html).not.toContain(">Evaluating<");
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
