// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { GitProposals, GitProposeButton } from "./git-delivery";
import { createTenantSession } from "@/lib/api";
import { proposalStatus, safePullURL } from "@/lib/gitops";

const mocks = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  api: mocks.api,
}));
let root: Root, host: HTMLDivElement;
const editor = createTenantSession("synthetic", "alpha", "editor");
const viewer = createTenantSession("synthetic", "alpha", "viewer");
const configured = { configured: true, repository: "acme/slos" };
const flush = () =>
  act(async () => {
    await Promise.resolve();
  });
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  mocks.api.mockReset();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

test("nothing renders when Git delivery is not configured", async () => {
  mocks.api.mockResolvedValue({ configured: false });
  await act(async () =>
    root.render(
      <GitProposeButton
        scope={editor}
        manifest={{}}
        plan={{ expectedRevision: 0, digest: "d" }}
      />,
    ),
  );
  await flush();
  expect(host.textContent).toBe("");
  await act(async () => root.render(<GitProposals scope={editor} id="x" />));
  await flush();
  expect(host.textContent).toBe("");
});

test("viewers never see the proposal action", async () => {
  mocks.api.mockResolvedValue(configured);
  await act(async () =>
    root.render(
      <GitProposeButton
        scope={viewer}
        manifest={{}}
        plan={{ expectedRevision: 0, digest: "d" }}
      />,
    ),
  );
  await flush();
  expect(host.querySelector("button")).toBeNull();
});

test("proposing sends the reviewed revision and digest and says the SLO is not applied", async () => {
  mocks.api.mockImplementation(async (path: string) => {
    if (path === "/api/gitops") return configured;
    return {
      receipt: {
        created: true,
        path: "slos/x.slo.yaml",
        pullRequest: {
          number: 7,
          url: "https://github.com/acme/slos/pull/7",
          state: "open",
          branch: "b",
          title: "t",
          createdAt: "",
        },
      },
      definitionDigest: "d",
      expectedRevision: 2,
      limitations: [],
    };
  });
  await act(async () =>
    root.render(
      <GitProposeButton
        scope={editor}
        manifest={{
          apiVersion: "nines.dev/v1alpha1",
          kind: "ServiceLevelObjective",
          metadata: { name: "x", project: "p" },
          spec: {},
        }}
        plan={{ expectedRevision: 2, digest: "abc" }}
      />,
    ),
  );
  await flush();
  const button = [...host.querySelectorAll("button")].find((b) =>
    b.textContent?.includes("Propose via Git"),
  )!;
  await act(async () => button.click());
  await flush();
  const call = mocks.api.mock.calls.find(
    (c) => c[0] === "/api/gitops/proposals",
  )!;
  expect(call[2].headers["If-Match"]).toBe('"2"');
  expect(call[2].headers["X-Nines-Plan-Digest"]).toBe("abc");
  expect(host.textContent).toContain("Pull request opened");
  expect(host.textContent).toContain("The SLO is not applied in Nines");
  expect(host.querySelector("a")?.getAttribute("href")).toBe(
    "https://github.com/acme/slos/pull/7",
  );
});

test("a merge alone is never shown as applied", async () => {
  mocks.api.mockImplementation(async (path: string) =>
    path === "/api/gitops"
      ? configured
      : path.endsWith("/drift")
        ? { state: "drifted", path: "slos/x.slo.yaml" }
        : {
            proposals: [
              {
                number: 1,
                url: "https://x/1",
                state: "merged",
                applied: true,
                title: "a",
                branch: "b",
                createdAt: "",
              },
              {
                number: 2,
                url: "https://x/2",
                state: "merged",
                applied: false,
                title: "b",
                branch: "b",
                createdAt: "",
              },
              {
                number: 3,
                url: "javascript:alert(1)",
                state: "open",
                applied: false,
                title: "c",
                branch: "b",
                createdAt: "",
              },
            ],
          },
  );
  await act(async () => root.render(<GitProposals scope={viewer} id="x" />));
  await flush();
  await flush();
  const text = host.textContent!;
  expect(text).toContain("Applied");
  expect(text).toContain("Merged, not applied");
  // Drift is reported as a difference, not as a verdict about which side is right.
  expect(text).toContain("Differs from repository");
  expect(text).toContain("Either may be the intended one");
  expect(text).toContain("A merge is not a deployment");
  // Non-https links from a provider are rendered as text, never as links.
  expect(
    [...host.querySelectorAll("a")].some((a) =>
      a.getAttribute("href")?.startsWith("javascript"),
    ),
  ).toBe(false);
});

test("status wording and link safety", () => {
  const base = {
    number: 1,
    url: "u",
    branch: "b",
    title: "t",
    createdAt: "",
  } as const;
  expect(
    proposalStatus({ ...base, state: "closed", applied: false }).label,
  ).toBe("Closed");
  expect(proposalStatus({ ...base, state: "open", applied: false }).label).toBe(
    "Open",
  );
  expect(safePullURL("https://github.com/a/b/pull/1")).toBe(true);
  expect(safePullURL("http://github.com/a/b/pull/1")).toBe(false);
  expect(safePullURL("javascript:alert(1)")).toBe(false);
});

test("journeys read their own proposal and drift routes", async () => {
  mocks.api.mockImplementation(async (path: string) =>
    path === "/api/gitops"
      ? configured
      : path.endsWith("/drift")
        ? { state: "in_sync", path: "slos/j.journey.yaml" }
        : { proposals: [] },
  );
  await act(async () =>
    root.render(<GitProposals scope={viewer} id="j" kind="journey" />),
  );
  await flush();
  await flush();
  const paths = mocks.api.mock.calls.map((c) => c[0]);
  expect(paths).toContain("/api/journeys/j/gitops/proposals");
  expect(paths).toContain("/api/journeys/j/gitops/drift");
  expect(host.textContent).toContain("In sync");
  expect(host.textContent).toContain("this journey");
});
