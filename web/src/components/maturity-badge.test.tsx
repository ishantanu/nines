// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { MaturityBadge } from "./maturity-badge";
import {
  featureOn,
  maturityFeatures,
  pageMaturity,
  pageOn,
  providerMaturity,
  providerOn,
} from "@/lib/maturity";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe("feature maturity", () => {
  it("marks experimental features and explains what remains", () => {
    act(() => root.render(<MaturityBadge feature={pageMaturity("intents")} />));
    const badge = container.querySelector(
      '[aria-label="Create from a goal is experimental"]',
    );
    expect(badge?.textContent).toContain("Experimental");
    expect(badge?.getAttribute("title")).toContain("Before beta:");
  });

  it("leaves the stable core unmarked", () => {
    act(() =>
      root.render(<MaturityBadge feature={pageMaturity("objectives")} />),
    );
    expect(container.textContent).toBe("");
  });

  it("covers the experimental pages and providers the portal shows", () => {
    const experimentalPages = maturityFeatures
      .filter((f) => f.level === "experimental" && f.page)
      .map((f) => f.page)
      .sort();
    expect(experimentalPages).toEqual([
      "agents",
      "incidents",
      "intents",
      "investigations",
      "replays",
      "workflows",
    ]);
    expect(providerMaturity("prometheus")?.level).toBe("beta");
    expect(providerMaturity("datadog")?.level).toBe("experimental");
  });
});

describe("feature gates in the portal", () => {
  it("shows the stable core always and gated features only when turned on", () => {
    expect(pageOn(undefined, "objectives")).toBe(true);
    expect(pageOn(undefined, "replays")).toBe(false);
    expect(pageOn([], "intents")).toBe(false);
    expect(pageOn(["create-from-goal"], "intents")).toBe(true);
    expect(featureOn([], "git-delivery")).toBe(true);
    expect(providerOn([], "prometheus")).toBe(true);
    expect(providerOn([], "datadog")).toBe(false);
    expect(providerOn(["datadog"], "datadog")).toBe(true);
  });
});
