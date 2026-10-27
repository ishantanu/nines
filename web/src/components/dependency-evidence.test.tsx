// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { DependencyEvidence } from "./dependency-evidence";
import type { DependencyEdge } from "@/lib/topology";
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
test("stale counters and partial continuity cannot be presented as instrumentation coverage", async () => {
  const edge: DependencyEdge = {
    caller: "api",
    callee: "database",
    provenance: "observed",
    critical: false,
    requests: 42,
    evidence: {
      source: "primary",
      sourceDigest: "receipt",
      metric: "traces_service_graph_request_total",
      from: "2026-10-26T00:00:00Z",
      to: "2026-10-26T00:15:00Z",
      readAt: "2026-10-26T00:15:00Z",
      freshness: "stale",
      maxSampleAgeSeconds: 45,
      sampleAgeSeconds: 300,
      instrumentationCoverage: "unknown",
      samples: {
        lastSampleAt: "2026-10-26T00:10:00Z",
        samples: 30,
        series: 1,
        sampleContinuity: 0.5,
        resolutionSeconds: 15,
      },
    },
  };
  await act(async () => root.render(<DependencyEvidence edge={edge} />));
  expect(host.textContent).toContain("Stale sample");
  expect(host.textContent).toContain("50.0% sample continuity");
  expect(host.textContent).toContain(
    "Instrumentation and trace-sampling coverage are unknown",
  );
  expect(host.textContent).not.toContain("100% coverage");
  const { default: axe } = await import("axe-core");
  const result = await axe.run(host, {
    runOnly: {
      type: "tag",
      values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"],
    },
    rules: { "color-contrast": { enabled: false } },
  });
  expect(result.violations.map((v) => v.id)).toEqual([]);
});
