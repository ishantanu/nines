// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { BudgetPolicyEditor } from "./budget-policy-editor";
import { BudgetDecisionPanel } from "./budget-decision";
import type { ErrorBudgetDecision, ErrorBudgetPolicy } from "@/lib/api";

let root: Root, host: HTMLDivElement;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

test("a response level can be added and removed without touching the rest of the policy", async () => {
  const base: ErrorBudgetPolicy = {
    owner: "o",
    runbookURL: "https://example.test/r",
    reviewBelow: 0.25,
  };
  const seen: (ErrorBudgetPolicy | undefined)[] = [];
  const Harness = ({ v }: { v: ErrorBudgetPolicy }) => (
    <BudgetPolicyEditor value={v} onChange={(n) => seen.push(n)} />
  );
  await act(async () => root.render(<Harness v={base} />));
  const add = [...host.querySelectorAll("button")].find((b) =>
    b.textContent?.includes("Add response level"),
  )!;
  await act(async () => add.click());
  expect(seen.at(-1)?.bands).toEqual([{ name: "", below: 0.5, response: "" }]);
  expect(seen.at(-1)?.owner).toBe("o");
  await act(async () =>
    root.render(<Harness v={{ ...base, bands: seen.at(-1)!.bands }} />),
  );
  const remove = [...host.querySelectorAll("button")].find((b) =>
    b.textContent?.includes("Remove level"),
  )!;
  await act(async () => remove.click());
  expect(seen.at(-1)).toEqual(base);
});

test("the decision shows the agreed response level and says it changes nothing", async () => {
  const decision = {
    state: "proceed",
    message: "ok",
    revision: 1,
    evaluatedAt: "2026-11-18T00:00:00Z",
    budgetRemainingFraction: 0.4,
    band: {
      name: "watch",
      below: 0.5,
      response: "Prioritize reliability bugs.",
    },
  } as unknown as ErrorBudgetDecision;
  await act(async () =>
    root.render(<BudgetDecisionPanel decision={decision} />),
  );
  const text = host.textContent!;
  expect(text).toContain("Response level: watch");
  expect(text).toContain("Prioritize reliability bugs.");
  expect(text).toContain("Within budget policy");
});
