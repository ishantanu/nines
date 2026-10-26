// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SystemOneAdvice, SystemOneConsent } from "./system-one-assessment";
import type { SystemOneAssessment } from "@/lib/intent-conversations";
let root: Root, host: HTMLDivElement;
const route = {
  provider: "typesafe",
  model: "jev-1.14.0",
  contractDigest: "contract",
};
const receipt: SystemOneAssessment = {
  status: "assessed",
  resolvedModel: "jev-1.14.0",
  inputDigest: "input-hash",
  requestDigest: "request-hash",
  inputTokens: 700,
  outputTokens: 123,
  at: "2026-10-25T00:00:00Z",
  judgments: {
    category: {
      choice: "agent",
      confidence: 0.8,
      probabilities: { agent: 0.8, none: 0.2 },
    },
    populationDefined: 0.9,
    goodEventsDefined: 0.6,
    observationPointDefined: 0.1,
  },
};
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
test("consent starts unchecked, names the destination and is disabled during a run", async () => {
  const onChange = vi.fn();
  await act(async () =>
    root.render(
      <SystemOneConsent
        route={route}
        checked={false}
        disabled={false}
        onChange={onChange}
      />,
    ),
  );
  const input = host.querySelector("input")!;
  expect(input.checked).toBe(false);
  expect(host.textContent).toContain("typesafe (jev-1.14.0)");
  expect(host.textContent).toContain("separate from model-directed");
  expect(onChange).not.toHaveBeenCalled();
  await act(async () => input.click());
  expect(onChange).toHaveBeenCalledWith(true);
  await act(async () =>
    root.render(
      <SystemOneConsent
        route={route}
        checked={false}
        disabled
        onChange={onChange}
      />,
    ),
  );
  expect(input.disabled).toBe(true);
});
test("advice distinguishes probabilities from measured attainment and includes the safe receipt", async () => {
  await act(async () =>
    root.render(<SystemOneAdvice route={route} assessment={receipt} />),
  );
  expect(host.textContent).toContain("90.0% probability");
  expect(host.textContent).toContain("10.0% probability");
  expect(host.textContent).toContain("not SLI values or measurement quality");
  expect(host.textContent).toContain("input-hash");
  expect(host.querySelectorAll("button")).toHaveLength(0);
});
test("an unavailable assessment never fabricates a healthy judgment", async () => {
  await act(async () =>
    root.render(
      <SystemOneAdvice
        route={route}
        assessment={{
          ...receipt,
          status: "unavailable",
          judgments: undefined,
          errorCode: "system_one_unavailable",
        }}
      />,
    ),
  );
  expect(host.textContent).toContain("Unavailable; preparation continues");
  expect(host.textContent).toContain("No judgment is available");
  expect(host.querySelector("dl")).toBeNull();
});
