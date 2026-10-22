import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { WorkflowDetail } from "./workflows";
import { TenantSelector } from "./tenant-selector";
import {
  createTenantSession,
  workflowDecision,
  type Role,
  type WorkflowRecord,
} from "@/lib/api";

const proposal: WorkflowRecord = {
  id: "proposal-1",
  kind: "propose_journey",
  state: "awaiting_approval",
  version: 4,
  skill: { id: "journey-change", version: "1.0.0", digest: "skill-digest" },
  note: "Investigate checkout latency",
  createdAt: "2026-10-18T10:00:00Z",
  updatedAt: "2026-10-18T10:00:00Z",
  findings: [
    {
      code: "coverage",
      severity: "warning",
      resource: "checkout",
      message: "Coverage is incomplete",
    },
  ],
  proposal: {
    action: "update",
    expectedRevision: 2,
    digest: "reviewed-proposal-digest",
    manifest: {
      apiVersion: "nines.dev/v1alpha1",
      kind: "CriticalUserJourney",
      metadata: { name: "checkout", project: "commerce" },
      spec: {
        displayName: "Checkout",
        description: "Complete purchase",
        owner: "commerce",
        criticality: "critical",
        steps: [],
        slos: [],
      },
    },
  },
};
function renderProposal(role: Role) {
  return renderToStaticMarkup(
    <WorkflowDetail
      workflow={proposal}
      scope={createTenantSession("", "commerce-tenant", role)}
      onChanged={() => {}}
    />,
  );
}
describe("workflow review controls", () => {
  it("resumes an approved change with its persisted reason and current version", () => {
    const approved: WorkflowRecord = {
      ...proposal,
      state: "approved",
      version: 5,
      decision: {
        action: "approve",
        reason: "Checked rollout evidence",
        at: "2026-10-18T10:05:00Z",
      },
    };
    expect(
      workflowDecision(approved, "approve", "accidentally edited reason"),
    ).toEqual({
      expectedVersion: 5,
      digest: "reviewed-proposal-digest",
      reason: "Checked rollout evidence",
    });
    const html = renderToStaticMarkup(
      <WorkflowDetail
        workflow={approved}
        scope={createTenantSession("", "commerce-tenant", "owner")}
        onChanged={() => {}}
      />,
    );
    expect(html).toContain("Resume approved apply");
    expect(html).toContain("Checked rollout evidence");
    expect(html).not.toContain('id="decision-reason"');
    expect(html).not.toContain("Reject proposal");
  });
  it("shows owners the exact manifest, revision, digest and tenant before approval", () => {
    const html = renderProposal("owner");
    expect(html).toContain("reviewed-proposal-digest");
    expect(html).toContain("current revision 2");
    expect(html).toContain("commerce-tenant");
    expect(html).toContain("Approve &amp; apply revision");
    expect(html).toContain("Coverage is incomplete");
    expect(html).toMatch(/disabled=""[^>]*>Reject proposal/);
  });
  it.each(["editor", "agent", "viewer"] as const)(
    "keeps %s proposal inspection available without decision controls",
    (role) => {
      const html = renderProposal(role);
      expect(html).toContain("reviewed-proposal-digest");
      expect(html).not.toContain("Approve &amp; apply revision");
      expect(html).not.toContain("Reject proposal");
    },
  );
});
it("shows the selected tenant in an accessible switcher trigger", () => {
  const html = renderToStaticMarkup(
    <TenantSelector
      tenants={[
        { id: "alpha", name: "Commerce production", role: "owner" },
        { id: "beta", name: "Sandbox", role: "viewer" },
      ]}
      selected="alpha"
      onChange={() => {}}
    />,
  );
  expect(html).toContain('aria-label="Switch tenant"');
  expect(html).toContain("Commerce production");
});
