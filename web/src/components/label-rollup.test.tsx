// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { LabelRollup, labelKeys, rollUpByLabel } from "./label-rollup";
import type { Objective } from "@/lib/api";
import { chooseOption } from "@/test-support/fields";

function objective(
  id: string,
  labels: Record<string, string> | undefined,
  state: "healthy" | "degraded" | "unknown" | "at_risk",
  budget: number | null,
  retired = false,
): Objective {
  const status = {
    alert: { phase: state === "degraded" ? "firing" : "inactive" },
    reliability: state === "unknown" ? null : 0.999,
    coverage: 1,
    burnRate: state === "at_risk" ? 2 : 0.1,
    budgetRemainingFraction: budget,
  };
  return {
    slo: { id, labels, retirement: retired ? {} : undefined },
    status,
  } as unknown as Objective;
}
const objectives = [
  objective("a", { team: "payments", tier: "1" }, "healthy", 0.8),
  objective("b", { team: "payments", tier: "2" }, "degraded", 0.1),
  objective("c", { team: "search", tier: "1" }, "unknown", null),
  objective("d", undefined, "healthy", 0.9),
  objective("e", { team: "retired" }, "healthy", 0.5, true),
];

test("roll-up counts states per value, worst first, without a combined score", () => {
  const { rows, unlabelled } = rollUpByLabel(objectives, "team");
  expect(rows.map((r) => r.value)).toEqual(["payments", "search"]);
  expect(rows[0]).toMatchObject({
    objectives: 2,
    degraded: 1,
    healthy: 1,
    lowestBudget: 0.1,
  });
  // An unmeasured budget stays unknown instead of becoming full.
  expect(rows[1].lowestBudget).toBeNull();
  expect(rows[1].unknown).toBe(1);
  // Unlabelled and archived objectives are not folded into any value.
  expect(unlabelled).toBe(1);
  expect(labelKeys(objectives)).toEqual(["team", "tier"]);
});

let root: Root, host: HTMLDivElement;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

test("the panel groups by a chosen key and explains unlabelled objectives", async () => {
  await act(async () => root.render(<LabelRollup objectives={objectives} />));
  expect(host.textContent).toContain("Reliability by label");
  expect(host.textContent).toContain("payments");
  expect(host.textContent).toContain("10.0%");
  expect(host.textContent).toContain("Unknown");
  expect(host.textContent).toContain("1 active objective has no team label");
  await chooseOption("rollup-key", "tier");
  expect(host.querySelectorAll("tbody tr")).toHaveLength(2);
});

test("nothing renders when no objective has a label", async () => {
  await act(async () =>
    root.render(
      <LabelRollup objectives={[objective("x", undefined, "healthy", 1)]} />,
    ),
  );
  expect(host.textContent).toBe("");
});
