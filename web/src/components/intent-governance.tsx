import { SystemOneAdvice } from "./system-one-assessment";
import { IntentTemplateEvidence } from "./intent-evidence";
import { IntentOutcomePanel } from "./intent-outcome";
import { BudgetDecisionPanel } from "./budget-decision";
import { useEffect, useId, useRef, useState } from "react";
import { stringifyDefinition as stringify } from "@/lib/definition-yaml";
import { api, canEdit, type RequestScope } from "@/lib/api";
import {
  conversationBase,
  type IntentConversation,
  type IntentChangeSet,
  type IntentRecommendation,
  type IntentAuditEvent,
} from "@/lib/intent-conversations";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";

type Action = (
  suffix: string,
  body: unknown,
  applied?: boolean,
) => Promise<unknown>;
function download(name: string, text: string, type: string) {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function valueText(field: string, value: unknown) {
  if (value == null) return "Not present";
  if (field === "spec.target" && typeof value === "number")
    return `${Number((value * 100).toPrecision(12))}%`;
  return stringify(value).trim();
}
export function IntentRecommendationReport({
  report,
}: {
  report?: IntentRecommendation;
}) {
  if (!report)
    return (
      <p className="text-xs text-muted-foreground">
        This earlier proposal has no frozen recommendation report. Review its
        definitions and saved preparation evidence.
      </p>
    );
  return (
    <section
      className="space-y-3 border-b pb-4"
      aria-label="Recommendation rationale and evidence"
    >
      <h3 className="text-sm font-medium">Why this recommendation</h3>
      <p className="text-sm">{report.summary}</p>
      {report.systemOne ? (
        <SystemOneAdvice
          assessment={report.systemOne}
          route={report.systemOneRoute}
        />
      ) : null}
      {report.budgetDecision ? (
        <BudgetDecisionPanel decision={report.budgetDecision} frozen />
      ) : null}
      {report.rollbackStatus === "creation_has_no_baseline" ? (
        <p className="text-xs font-medium">
          This proposal creates a resource. Configuration rollback cannot undo
          creation; retirement is not implemented.
        </p>
      ) : report.rollbackStatus === "restorable_updates" ? (
        <p className="text-xs text-muted-foreground">
          Prior definitions can be restored through a separate rollback review
          if no later edits intervene. Owner-enabled automatic rule destinations
          reconcile saved revisions; activation is verified separately.
        </p>
      ) : report.rollbackStatus === "no_definition_change" ? (
        <p className="text-xs text-muted-foreground">
          The proposed definitions already match the saved configuration.
        </p>
      ) : null}
      {report.explanation ? (
        <details className="text-xs">
          <summary className="cursor-pointer font-medium">
            Preparation summary
          </summary>
          <p className="mt-1 whitespace-pre-wrap text-xs text-muted-foreground">
            {report.explanation}
          </p>
        </details>
      ) : null}
      <ul className="list-disc space-y-1 pl-4 text-xs">
        {report.basis.map((reason, i) => (
          <li key={i}>{reason}</li>
        ))}
      </ul>
      {report.fields.map((field) => {
        const diff = (
          <>
            <div className="mt-2 grid gap-3 sm:grid-cols-2">
              <div>
                <p className="mb-1 text-muted-foreground">Before</p>
                <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-words">
                  {valueText(field.field, field.before)}
                </pre>
              </div>
              <div>
                <p className="mb-1 text-muted-foreground">Proposed</p>
                <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-words">
                  {valueText(field.field, field.after)}
                </pre>
              </div>
            </div>
            <p className="mt-2 text-muted-foreground">{field.impact}</p>
          </>
        );
        const complex =
          (field.before !== null && typeof field.before === "object") ||
          (field.after !== null && typeof field.after === "object");
        return complex ? (
          <details
            key={field.resource + field.field}
            className="rounded-md border p-3 text-xs"
          >
            <summary className="cursor-pointer font-medium">
              {field.resource} · {field.field}
            </summary>
            {diff}
          </details>
        ) : (
          <div
            key={field.resource + field.field}
            className="rounded-md border p-3 text-xs"
          >
            <p className="font-medium">
              {field.resource} · {field.field}
            </p>
            {diff}
          </div>
        );
      })}
      <p className="text-xs">
        Measurement:{" "}
        {report.measurementDigest
          ? "owned and versioned by the SLO"
          : `${report.binding.name} · revision ${report.binding.revision}`}
        . Telemetry observed {report.observedFrom} to {report.observedTo}.
      </p>
      {report.backtest ? (
        <p className="text-xs">
          Historical evidence: {report.backtest.complete} complete cutoffs,{" "}
          {report.backtest.unknown} unknown, {report.backtest.noTraffic} without
          traffic, {report.backtest.targetViolations} target violations and{" "}
          {report.backtest.burnBreaches} burn-threshold breaches. Interval:{" "}
          {report.backtest.from} to {report.backtest.to}.
        </p>
      ) : null}
      <details className="text-xs">
        <summary className="cursor-pointer font-medium">
          Model, skill and evidence provenance
        </summary>
        <dl className="mt-2 space-y-2 break-all">
          {report.requestedBy ? (
            <div>
              <dt className="text-muted-foreground">Requested by</dt>
              <dd>
                {report.requestedBy.name || report.requestedBy.subject} ·{" "}
                {report.requestedBy.kind}
              </dd>
            </div>
          ) : null}
          <div>
            <dt className="text-muted-foreground">Compiler</dt>
            <dd>{report.compilerVersion || "not recorded"}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Preparation</dt>
            <dd>
              {report.mode} · run {report.runId || "not recorded"}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Workflow model</dt>
            <dd>
              {report.modelRoute
                ? `${report.modelRoute.provider} / ${report.modelRoute.model} (${report.modelRoute.protocol})`
                : "No workflow model"}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Drafting model</dt>
            <dd>
              {report.draftRoute
                ? `${report.draftRoute.provider} / ${report.draftRoute.model}`
                : "No drafting model recorded"}
            </dd>
          </div>
          {report.skill ? (
            <div>
              <dt className="text-muted-foreground">Skill</dt>
              <dd>
                {report.skill.id} · {report.skill.version} ·{" "}
                {report.skill.digest}
              </dd>
            </div>
          ) : null}
          <div>
            <dt className="text-muted-foreground">Measurement digest</dt>
            <dd>{report.measurementDigest ?? report.binding.digest}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Source fingerprint</dt>
            <dd>{report.sourceDigest}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Telemetry evidence digest</dt>
            <dd>{report.evidenceDigest}</dd>
          </div>
        </dl>
      </details>
      <details className="text-xs">
        <summary className="cursor-pointer font-medium">
          Assumptions and limitations ({report.limitations.length})
        </summary>
        <ul className="list-disc space-y-1 pl-4 text-xs text-muted-foreground">
          {report.limitations.map((limitation, i) => (
            <li key={i}>{limitation}</li>
          ))}
        </ul>
      </details>
    </section>
  );
}
export function IntentDecisionControls({
  scope,
  conversation,
  busy,
  onAction,
}: {
  scope: RequestScope;
  conversation: IntentConversation;
  busy: boolean;
  onAction: Action;
}) {
  const change = conversation.changeSet;
  const [reason, setReason] = useState(""),
    [ack, setAck] = useState(false);
  const id = useId();
  const [exceptionReason, setExceptionReason] = useState("");
  useEffect(() => {
    setReason("");
    setExceptionReason("");
    setAck(false);
  }, [change?.digest, change?.state, conversation.id]);
  if (!change || change.state === "applied") return null;
  const review = (suffix: string) =>
    onAction(suffix, {
      expectedVersion: conversation.version,
      digest: change.digest,
      confirm: true,
      reason,
      ...(suffix === "/change-set/approve" && exceptionReason.trim()
        ? { safetyExceptionReason: exceptionReason.trim() }
        : {}),
    });
  if (change.state === "rejected")
    return (
      <div className="space-y-2 text-xs">
        <p>
          Rejected by {change.rejectedBy?.name || "an authorized reviewer"}:{" "}
          {change.rejectionReason}
        </p>
        <Button
          size="sm"
          variant="outline"
          disabled={busy || !canEdit(scope)}
          onClick={() =>
            void onAction("/change-set", {
              expectedVersion: conversation.version,
            })
          }
        >
          Prepare a new review
        </Button>
      </div>
    );
  return (
    <div className="space-y-3">
      {change.observationPolicy ? (
        <p className="text-xs text-muted-foreground">
          Apply refreshes quality and budget evidence. Post-change checks allow{" "}
          {change.observationPolicy.warmupSeconds}s warmup, then require{" "}
          {change.observationPolicy.observeSeconds}s of qualifying observations,
          with a {change.observationPolicy.deadlineSeconds}s deadline.
        </p>
      ) : null}
      {change.safetyCheck ? (
        <div className="space-y-2 rounded-md border p-3">
          <p className="text-xs font-medium">
            Last action-time check:{" "}
            {change.safetyCheck.state.replaceAll("_", " ")}
          </p>
          <p className="text-xs">{change.safetyCheck.message}</p>
          {["pending", "firing"].includes(
            change.safetyCheck.burnState ?? "",
          ) ? (
            <p className="text-xs font-medium">
              Burn-rate alert: {change.safetyCheck.burnState}.
            </p>
          ) : null}
          {change.safetyCheck.budget ? (
            <BudgetDecisionPanel decision={change.safetyCheck.budget} frozen />
          ) : null}
        </div>
      ) : null}
      {change.safetyException ? (
        <p className="text-xs">
          Corrective-work exception: {change.safetyException.reason}. Expires{" "}
          {change.safetyException.expiresAt}. A different tenant owner must
          apply it.
        </p>
      ) : null}
      {change.state === "approved" ? (
        <div className="text-xs">
          <p>
            Approved by {change.approvedBy?.name || "an authorized reviewer"} at{" "}
            {change.approvedAt}.{" "}
            {change.safetyException
              ? "A different tenant owner"
              : "A different owner or editor"}{" "}
            must apply this approval.
          </p>
          <p className="mt-1 whitespace-pre-wrap text-muted-foreground">
            {change.approvalReason ||
              "No reviewer note supplied by this client."}
          </p>
          <Button
            className="mt-3"
            size="sm"
            disabled={
              !canEdit(scope) ||
              busy ||
              (!!change.safetyException && scope.role !== "owner")
            }
            onClick={() =>
              void onAction(
                "/change-set/apply",
                {
                  expectedVersion: conversation.version,
                  digest: change.digest,
                  confirm: true,
                },
                true,
              )
            }
          >
            Apply approved changes
          </Button>
        </div>
      ) : null}
      <div>
        <Label htmlFor={id}>
          {change.state === "approved"
            ? "Reason to withdraw approval"
            : "Review decision and reason"}
        </Label>
        <Textarea
          id={id}
          className="mt-2"
          value={reason}
          maxLength={1000}
          disabled={busy || !canEdit(scope)}
          onChange={(e) => setReason(e.target.value)}
          placeholder="Record the evidence reviewed, accepted trade-offs or concerns."
        />
      </div>
      {change.state === "proposed" && scope.role === "owner" ? (
        <details className="space-y-2 text-xs">
          <summary className="cursor-pointer font-medium">
            Approve corrective work despite adverse evidence
          </summary>
          <p className="mt-2 text-muted-foreground">
            Use only after reviewing the risks of exhausted budgets or
            insufficient measurements. This 30-minute exception requires a
            different owner to apply and cannot bypass source, revision or
            access checks.
          </p>
          <Label htmlFor={id + "-exception"}>
            Corrective-work justification
          </Label>
          <Textarea
            id={id + "-exception"}
            value={exceptionReason}
            maxLength={1000}
            disabled={busy}
            onChange={(e) => setExceptionReason(e.target.value)}
          />
        </details>
      ) : null}
      {change.state === "proposed" ? (
        <Label className="flex items-start gap-2 text-xs">
          <input
            type="checkbox"
            checked={ack}
            disabled={!canEdit(scope) || busy}
            onChange={(e) => setAck(e.target.checked)}
          />
          <span>
            I reviewed the rationale, target, window, measurement, limitations
            and all resource changes.
          </span>
        </Label>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {change.state === "proposed" ? (
          <Button
            size="sm"
            disabled={!canEdit(scope) || busy || !ack || !reason.trim()}
            onClick={() => void review("/change-set/approve")}
          >
            Approve changes
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="outline"
          disabled={!canEdit(scope) || busy || !reason.trim()}
          onClick={() => void review("/change-set/reject")}
        >
          {change.state === "approved" ? "Withdraw approval" : "Reject changes"}
        </Button>
      </div>
    </div>
  );
}
export function IntentAuditTimeline({
  events,
}: {
  events: IntentAuditEvent[];
}) {
  return (
    <ol className="space-y-3">
      {events.map((event) => (
        <li
          key={event.sequence}
          className="border-l-2 border-border pl-3 text-xs"
        >
          <p className="font-medium">
            {event.action.replaceAll("_", " ").replace(".", ": ")} ·{" "}
            {event.outcome}
          </p>
          <p className="mt-1 text-muted-foreground">
            {event.actor.name || event.actor.subject} · {event.actor.kind} ·{" "}
            {event.at} · conversation version {event.version}
          </p>
          <p className="mt-1 whitespace-pre-wrap">{event.reason}</p>
          <details className="mt-1">
            <summary className="cursor-pointer text-muted-foreground">
              Actor and integrity references
            </summary>
            <p className="mt-1 break-all font-mono">
              Actor: {event.actor.subject}
              <br />
              Change: {event.changeSetDigest || "not recorded"}
              <br />
              Event: {event.hash}
              <br />
              Previous: {event.previousHash || "first event"}
            </p>
          </details>
        </li>
      ))}
    </ol>
  );
}
export function IntentRollbackPanel({
  scope,
  conversation,
  change,
  busy,
  running,
  onAction,
  onRead,
}: {
  scope: RequestScope;
  conversation: IntentConversation;
  change: IntentChangeSet;
  busy: boolean;
  running: boolean;
  onAction: Action;
  onRead: (fn: () => Promise<unknown>) => void;
}) {
  const rollback = change.rollback;
  const [reason, setReason] = useState(""),
    [ack, setAck] = useState(false),
    [namespace, setNamespace] = useState("");
  const controller = useRef(new AbortController());
  const id = useId();
  useEffect(() => {
    const c = new AbortController();
    controller.current = c;
    return () => c.abort();
  }, [scope, conversation.id]);
  useEffect(() => {
    setReason("");
    setAck(false);
  }, [rollback?.digest, rollback?.state]);
  const request = (suffix: string, digest: string, applied = false) =>
    onAction(
      suffix,
      { expectedVersion: conversation.version, digest, confirm: true, reason },
      applied,
    );
  async function exportKubernetes() {
    if (!rollback) return;
    const signal = controller.current.signal;
    const result = await api<{
      tenant: string;
      changeSetDigest: string;
      namespace: string;
      yaml: string;
    }>(
      conversationBase +
        "/" +
        encodeURIComponent(conversation.id) +
        "/change-set/rollback/kubernetes?" +
        new URLSearchParams({ namespace, digest: rollback.digest }),
      scope,
      { signal },
    );
    if (
      result.tenant !== scope.tenant ||
      result.changeSetDigest !== rollback.digest ||
      result.namespace !== namespace
    )
      throw new Error("Rollback export scope mismatch");
    if (!signal.aborted && !scope.signal.aborted)
      download(
        "nines-rollback.kubernetes.yaml",
        result.yaml,
        "application/yaml",
      );
  }
  return (
    <section
      className="space-y-3 rounded-md border p-3"
      aria-label={`Configuration rollback for ${change.id}`}
    >
      <IntentOutcomePanel
        report={
          rollback?.state === "applied" ? rollback.outcome : change.outcome
        }
        scope={scope}
        conversation={conversation.id}
      />
      <Button
        size="sm"
        variant="outline"
        disabled={busy || !canEdit(scope)}
        onClick={() =>
          void onAction("/change-set/observe", {
            expectedVersion: conversation.version,
            digest:
              rollback?.state === "applied" ? rollback.digest : change.digest,
            confirm: true,
          })
        }
      >
        Check runtime and record outcome
      </Button>
      <h3 className="text-sm font-medium">Configuration rollback</h3>
      <p className="break-all text-xs text-muted-foreground">
        Original change: {change.digest}
      </p>
      <p className="text-xs text-muted-foreground">
        Restores earlier definitions as new revisions. Later edits block
        rollback. Owner-enabled automatic rule destinations reconcile the
        restored revision; manual destinations need a separate Rules apply.
        Consumed error budget cannot be restored.
      </p>
      {!rollback || rollback.state === "rejected" ? (
        <>
          <Label htmlFor={id}>Why is rollback needed?</Label>
          <Textarea
            id={id}
            value={reason}
            maxLength={1000}
            disabled={busy || !canEdit(scope)}
            onChange={(e) => setReason(e.target.value)}
          />
          <Button
            size="sm"
            variant="outline"
            disabled={busy || running || !canEdit(scope) || !reason.trim()}
            onClick={() => void request("/change-set/rollback", change.digest)}
          >
            Prepare rollback review
          </Button>
        </>
      ) : (
        <>
          <p className="text-xs">
            Status: {rollback.state} · prepared by {rollback.createdBy.name} at{" "}
            {rollback.createdAt}
          </p>
          <p className="whitespace-pre-wrap text-xs">{rollback.reason}</p>
          {rollback.blockers?.length ? (
            <div role="status" className="space-y-2 text-xs">
              <p className="font-medium">
                Rollback unavailable. No resources changed.
              </p>
              {rollback.blockers.map((blocker) => (
                <p key={blocker}>{blocker}</p>
              ))}
            </div>
          ) : null}
          {rollback.changes.map((change) => (
            <details className="text-xs" key={change.kind + change.name}>
              <summary className="cursor-pointer font-medium">
                Restore {change.name} · expected current revision{" "}
                {change.expectedRevision}
              </summary>
              <p className="mt-2 font-medium">Applied definition</p>
              <pre className="mt-1 max-h-48 overflow-auto rounded-md border p-2">
                {stringify(change.previous)}
              </pre>
              <p className="mt-2 font-medium">Restore definition</p>
              <pre className="mt-1 max-h-48 overflow-auto rounded-md border p-2">
                {stringify(change.manifest)}
              </pre>
            </details>
          ))}
          <p className="break-all font-mono text-xs">
            Rollback SHA-256 {rollback.digest}
          </p>
          {rollback.state === "proposed" || rollback.state === "approved" ? (
            <>
              {rollback.state === "approved" ? (
                <>
                  <p className="text-xs">
                    Approved by {rollback.approvedBy?.name} at{" "}
                    {rollback.approvedAt}: {rollback.approvalReason}. A
                    different owner or editor must apply this rollback.
                  </p>
                  <Button
                    size="sm"
                    disabled={busy || running || !canEdit(scope)}
                    onClick={() =>
                      void request(
                        "/change-set/rollback/apply",
                        rollback.digest,
                        true,
                      )
                    }
                  >
                    Apply approved configuration rollback
                  </Button>
                </>
              ) : (
                <Label className="flex items-start gap-2 text-xs">
                  <input
                    type="checkbox"
                    checked={ack}
                    disabled={busy || !canEdit(scope)}
                    onChange={(e) => setAck(e.target.checked)}
                  />
                  <span>
                    I reviewed the restore diff and the telemetry connection
                    rule delivery policy.
                  </span>
                </Label>
              )}
              <Label htmlFor={id}>
                {rollback.state === "approved"
                  ? "Reason to withdraw rollback approval"
                  : "Rollback review decision and reason"}
              </Label>
              <Textarea
                id={id}
                maxLength={1000}
                value={reason}
                disabled={busy || !canEdit(scope)}
                onChange={(e) => setReason(e.target.value)}
              />
              <div className="flex flex-wrap gap-2">
                {rollback.state === "proposed" ? (
                  <Button
                    size="sm"
                    disabled={
                      busy ||
                      running ||
                      !canEdit(scope) ||
                      !ack ||
                      !reason.trim()
                    }
                    onClick={() =>
                      void request(
                        "/change-set/rollback/approve",
                        rollback.digest,
                      )
                    }
                  >
                    Approve rollback
                  </Button>
                ) : null}
                <Button
                  size="sm"
                  variant="outline"
                  disabled={
                    busy || running || !canEdit(scope) || !reason.trim()
                  }
                  onClick={() =>
                    void request("/change-set/rollback/reject", rollback.digest)
                  }
                >
                  Reject or withdraw rollback
                </Button>
              </div>
            </>
          ) : null}
          {rollback.state === "applied" ? (
            <div className="space-y-1 text-xs">
              <p>
                Configuration restored by {rollback.appliedBy?.name} at{" "}
                {rollback.appliedAt}.
              </p>
              {rollback.applied?.map((resource) => (
                <p key={resource.name}>
                  {resource.name}: revision {resource.revision}
                </p>
              ))}
              <p className="text-muted-foreground">
                Check rule deployment under the telemetry connection policy:
                automatic reconciliation when enabled, otherwise reviewed apply.
                Recollect coverage for the restored configuration.
              </p>
            </div>
          ) : null}
          {rollback.state !== "blocked" ? (
            <details className="text-xs">
              <summary className="cursor-pointer font-medium">
                Standalone operator / GitOps rollback export
              </summary>
              <div className="mt-2 space-y-2">
                <Label htmlFor={`${id}-namespace`}>Kubernetes namespace</Label>
                <Input
                  id={`${id}-namespace`}
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                />
                <p className="text-muted-foreground">
                  Downloads desired definitions for a separate GitOps review.
                  For portal-managed operator resources, apply configuration
                  rollback here, then review deployment in the SLO’s Rules tab.
                </p>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={
                    busy || !/^[a-z0-9][a-z0-9-]{0,62}$/.test(namespace)
                  }
                  onClick={() => onRead(exportKubernetes)}
                >
                  Download rollback Kubernetes YAML
                </Button>
              </div>
            </details>
          ) : null}
        </>
      )}
    </section>
  );
}
export function IntentGovernanceHistory({
  scope,
  conversation,
  busy,
  running,
  onAction,
  onRead,
}: {
  scope: RequestScope;
  conversation: IntentConversation;
  busy: boolean;
  running: boolean;
  onAction: Action;
  onRead: (fn: () => Promise<unknown>) => void;
}) {
  const controller = useRef(new AbortController());
  useEffect(() => {
    const c = new AbortController();
    controller.current = c;
    return () => c.abort();
  }, [scope, conversation.id]);
  async function exportAudit() {
    const signal = controller.current.signal;
    const base =
      conversationBase + "/" + encodeURIComponent(conversation.id) + "/audit";
    type Page = {
      tenant: string;
      conversation: string;
      integrity: string;
      events: IntentAuditEvent[];
      nextAfter?: number;
      through?: number;
    };
    const result = await api<Page>(base, scope, { signal });
    const validate = (page: Page) => {
      if (page.tenant !== scope.tenant || page.conversation !== conversation.id)
        throw new Error("Audit export scope mismatch");
    };
    validate(result);
    let after = result.nextAfter;
    const through = result.through;
    const seen = new Set<number>();
    while (after) {
      if (seen.has(after) || result.events.length >= 10000)
        throw new Error(
          "Audit export limit reached; use nines audit export for the complete journal.",
        );
      seen.add(after);
      const page = await api<Page>(
        base + "?after=" + after + "&through=" + through,
        scope,
        { signal },
      );
      validate(page);
      if (page.through !== through)
        throw new Error("Audit export snapshot changed");
      if (page.integrity !== "verified") result.integrity = "invalid";
      result.events.push(...page.events);
      after = page.nextAfter;
    }
    delete result.nextAfter;
    if (!signal.aborted && !scope.signal.aborted)
      download(
        "nines-" + conversation.id + ".audit.json",
        JSON.stringify(result, null, 2),
        "application/json",
      );
  }
  const changes = [
    ...(conversation.changeSetHistory ?? []),
    ...(conversation.changeSet ? [conversation.changeSet] : []),
  ];
  return (
    <div className="space-y-4">
      {changes
        .filter((change) => change.state === "applied")
        .map((change) => (
          <IntentRollbackPanel
            key={change.id}
            scope={scope}
            conversation={conversation}
            change={change}
            busy={busy}
            running={running}
            onAction={onAction}
            onRead={onRead}
          />
        ))}
      <section
        className="space-y-3 rounded-md border p-3"
        aria-label="Intent change audit trail"
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-medium">Decision audit trail</h3>
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => onRead(exportAudit)}
          >
            Download audit JSON
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          Records who proposed, approved, rejected, applied or restored changes.
          Recent events are shown here. The export retrieves every decision page
          and includes retained conversation definitions and an integrity check.
          Audit records cannot be cleared through the application.
        </p>
        {conversation.audit?.length ? (
          <div className="space-y-3">
            <IntentAuditTimeline events={conversation.audit.slice(-5)} />
            {conversation.audit.length > 5 ? (
              <details className="text-xs">
                <summary className="cursor-pointer font-medium">
                  Earlier recent decisions ({conversation.audit.length - 5})
                </summary>
                <div className="mt-3">
                  <IntentAuditTimeline
                    events={conversation.audit.slice(0, -5)}
                  />
                </div>
              </details>
            ) : null}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            No change-control events recorded yet. Earlier decisions remain in
            saved change-set metadata.
          </p>
        )}
      </section>
    </div>
  );
}
