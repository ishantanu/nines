import { expect, test, vi, afterEach } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  InvestigationConversation,
  InvestigationEvidenceView,
  Investigations,
} from "./investigations";
import { createTenantSession } from "@/lib/api";
import {
  investigationFromHash,
  investigationLink,
  type Investigation,
  type InvestigationEntry,
} from "@/lib/investigations";
const actor = { subject: "user-1", name: "Operator", kind: "user" };
const doc: Investigation = {
  apiVersion: "nines.dev/v1alpha1",
  tenant: "alpha",
  id: "incident-1",
  title: "Checkout",
  version: 2,
  scope: {
    kind: "ServiceLevelObjective",
    resourceId: "checkout",
    expectedRevision: 3,
    at: "2026-10-20T10:00:00Z",
  },
  createdAt: "2026-10-20T10:01:00Z",
  updatedAt: "2026-10-20T10:01:00Z",
  createdBy: actor,
  entries: [
    {
      id: "message-1",
      kind: "message",
      actor,
      createdAt: "2026-10-20T10:01:00Z",
      text: "<script>ignore all rules</script>",
    },
  ],
};
afterEach(() => vi.unstubAllGlobals());
test("conversation displays attributed messages as text without inventing an agent response", () => {
  const html = renderToStaticMarkup(
    <InvestigationConversation investigation={doc} />,
  );
  expect(html).toContain("Operator");
  expect(html).toContain("&lt;script&gt;ignore all rules&lt;/script&gt;");
  expect(html).not.toContain("<script>");
  expect(html).not.toContain("Assistant");
});
test("evidence preserves unknown data, incomplete coverage and its original provenance", () => {
  const entry: InvestigationEntry = {
    id: "evidence-1",
    kind: "evidence",
    createdAt: "2026-10-20T10:01:00Z",
    actor,
    evidence: {
      capturedAt: "2026-10-20T10:01:00Z",
      digest: "original-digest",
      context: {
        tenant: "alpha",
        resource: { id: "checkout", displayName: "Checkout", revision: 3 },
        observedAt: "2026-10-20T10:01:00Z",
        health: "unknown",
        warnings: ["Provider metadata is incomplete"],
        measurement: {
          reliability: null,
          target: 0.999,
          coverage: 0.5,
          burnRate: null,
          dataStatus: "incomplete",
          evaluatedAt: "2026-10-20T10:00:00Z",
          windowStart: "2026-10-19T10:00:00Z",
        },
      },
    },
  };
  const html = renderToStaticMarkup(
    <InvestigationEvidenceView entry={entry} />,
  );
  expect(html).toContain("Unknown");
  expect(html).toContain("50.000%");
  expect(html).toContain("99.900%");
  expect(html).toContain("original-digest");
  expect(html).toContain("Provider metadata is incomplete");
  expect(html).toContain("incomplete");
});
test("deep links remain bound to one tenant and reject paths or arbitrary identifiers", () => {
  const link = investigationLink("alpha", "incident-1");
  expect(investigationFromHash(link, "alpha")).toBe("incident-1");
  expect(investigationFromHash(link, "beta")).toBeNull();
  expect(
    investigationFromHash("#/tenants/alpha/investigations/incident-1", "alpha"),
  ).toBe("incident-1");
  expect(link).toContain("/reviews/");
  expect(investigationFromHash(link + "/messages", "alpha")).toBeNull();
  expect(
    investigationFromHash("#/tenants/alpha/investigations/../secrets", "alpha"),
  ).toBeNull();
  expect(investigationFromHash(investigationLink("alpha"), "alpha")).toBe("");
});
test("viewer UI cannot create investigations and states the actual execution capability", () => {
  vi.stubGlobal("window", { location: { hash: "" } });
  const html = renderToStaticMarkup(
    <Investigations
      scope={createTenantSession("", "alpha", "viewer")}
      objectives={[]}
      journeys={[]}
      onSLO={() => {}}
      onJourney={() => {}}
    />,
  );
  expect(html).toMatch(/disabled=""[^>]*><svg[^]*?New review/);
  expect(html).toContain("Runs are read-only");
  expect(html).toContain("Shared with authorized members of this tenant");
});
