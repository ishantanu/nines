import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { RuleApplyEvidence } from "./recording-rules";

test("written rules retain an explicit failed reload and unverified runtime", () => {
  const html = renderToStaticMarkup(
    <RuleApplyEvidence
      result={{
        state: "written",
        message: "Current file is saved.",
        activation: {
          state: "failed",
          message: "Prometheus rejected reload.",
          nextAction: "Enable lifecycle on the trusted destination.",
        },
        evaluation: {
          state: "not_loaded",
          message: "Current group is not loaded.",
        },
      }}
    />,
  );
  expect(html).toContain("File written");
  expect(html).toContain("Prometheus rejected reload.");
  expect(html).toContain("Enable lifecycle on the trusted destination.");
  expect(html).toContain("Current group is not loaded.");
  expect(html).not.toContain(">Deployed<");
  expect(html).not.toContain(">Ready<");
});

test("stored ruler configuration and runtime errors stay separate", () => {
  const html = renderToStaticMarkup(
    <RuleApplyEvidence
      result={{
        state: "accepted",
        message: "Ruler accepted the current group.",
        activation: {
          state: "accepted",
          message: "Ruler activation requested.",
        },
        evaluation: {
          state: "failed",
          message: "Expression evaluation failed.",
        },
      }}
    />,
  );
  expect(html).toContain("Stored in ruler");
  expect(html).toContain("Expression evaluation failed.");
  expect(html).not.toContain(">Ready<");
});

test("operator publication acknowledgement does not imply provider activation", () => {
  const html = renderToStaticMarkup(
    <RuleApplyEvidence
      result={{
        state: "submitted",
        message: "Reviewed SLO published to Kubernetes.",
        activation: {
          state: "accepted",
          message: "Operator reconciliation pending.",
        },
        evaluation: {
          state: "unknown",
          message: "No current runtime evidence.",
        },
      }}
    />,
  );
  expect(html).toContain("Published to Kubernetes");
  expect(html).toContain("Operator reconciliation pending.");
  expect(html).toContain(">Unverified<");
  expect(html).not.toContain(">Evaluating<");
});
