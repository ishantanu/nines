import { SystemOneAdvice } from "./system-one-assessment";
import { useEffect, useId, useState } from "react";
import type { FormEvent } from "react";
import type { RequestScope } from "@/lib/api";
import {
  readAudit,
  readAuditRecord,
  readArchiveStatus,
  type ArchiveStatus,
  type AuditPage,
  type AuditRecord,
} from "@/lib/audit";
import { workspacePath, followLink } from "@/lib/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { IntentRecommendationReport } from "./intent-governance";

function AuditDetail({
  scope,
  record,
}: {
  scope: RequestScope;
  record: AuditRecord;
}) {
  const [full, setFull] = useState<AuditRecord>();
  const [error, setError] = useState("");
  useEffect(() => {
    const c = new AbortController();
    setFull(undefined);
    setError("");
    readAuditRecord(scope, record.id, c.signal)
      .then((result) => {
        if (!c.signal.aborted) setFull(result);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      });
    return () => c.abort();
  }, [scope, record.id]);
  const href = workspacePath(scope.tenant, "intents", record.conversation);
  return (
    <section
      className="space-y-4 rounded-md border p-4"
      aria-label="Decision details"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="text-sm font-semibold">Decision details</h3>
        <a
          className="text-sm underline"
          href={href}
          onClick={(e) => followLink(e, href)}
        >
          Open SLO intent
        </a>
      </div>
      <dl className="grid gap-3 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-muted-foreground">Actor subject</dt>
          <dd className="break-all">{record.event.actor.subject}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Identity type</dt>
          <dd>{record.event.actor.kind}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Review note</dt>
          <dd className="whitespace-pre-wrap">{record.event.reason}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Exact change digest</dt>
          <dd className="break-all font-mono text-xs">
            {record.event.changeSetDigest || "No change set referenced"}
          </dd>
        </div>
      </dl>
      <ArchiveEvidence scope={scope} recordID={record.id} />
      {record.recoveredFromConversation ? (
        <p className="text-xs text-muted-foreground">
          Recovered from a retained conversation when this journal was first
          introduced. Definition and recommendation digests are preserved;
          proposal lifecycle metadata reflects the retained object. Use decision
          events for historical approval and apply state.
        </p>
      ) : null}
      {error ? (
        <p role="alert">{error}</p>
      ) : !full ? (
        <p role="status">Loading frozen evidence…</p>
      ) : full.systemOne ? (
        full.systemOne.assessment ? (
          <SystemOneAdvice
            assessment={full.systemOne.assessment}
            route={full.systemOne.route}
          />
        ) : (
          <p className="text-sm text-muted-foreground">
            An optional semantic assessment was requested for run{" "}
            {full.systemOne.runId}. The result has its own audit event. Input
            SHA-256: {full.systemOne.inputDigest}
          </p>
        )
      ) : full.proposal ? (
        <>
          <IntentRecommendationReport report={full.proposal.recommendation} />
          <details>
            <summary className="cursor-pointer text-sm font-medium">
              Frozen proposal and definition changes
            </summary>
            <pre className="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-words text-xs">
              {JSON.stringify(full.proposal, null, 2)}
            </pre>
          </details>
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          The proposal event retains the frozen recommendation. Use its exact
          digest to locate it in this journal.
        </p>
      )}
    </section>
  );
}
function ArchiveEvidence({
  scope,
  recordID = "",
}: {
  scope: RequestScope;
  recordID?: string;
}) {
  const [status, setStatus] = useState<ArchiveStatus>();
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    setStatus(undefined);
    setError("");
    readArchiveStatus(scope, c.signal, recordID)
      .then((value) => {
        if (!c.signal.aborted) setStatus(value);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      });
    return () => c.abort();
  }, [scope, refresh, recordID]);
  return (
    <section
      aria-label={recordID ? "Archive receipt" : "External audit archive"}
      className="space-y-2 rounded-md border p-4"
    >
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold">
          {recordID ? "Archive receipt" : "External archive"}
        </h3>
        <Button
          variant="outline"
          size="sm"
          onClick={() => setRefresh((n) => n + 1)}
        >
          Refresh archive status
        </Button>
      </div>
      {error ? (
        <p role="alert" className="text-sm">
          {error}
        </p>
      ) : !status ? (
        <p role="status" className="text-sm">
          Loading archive evidence…
        </p>
      ) : !status.enabled ? (
        <p className="text-sm text-muted-foreground">
          Not configured. Decisions are retained in Nines; external immutable
          storage is not enabled.
        </p>
      ) : (
        <>
          <p className="text-sm">
            {status.verified} verified · {status.pending} pending ·{" "}
            {status.failed} awaiting retry
          </p>
          <p className="text-xs text-muted-foreground">
            {status.retained} objects within verified retention;{" "}
            {status.expired} retention periods elapsed. Status reflects the last
            readback, not continuous provider monitoring.
          </p>
          {status.receipt ? (
            <dl className="grid gap-2 text-xs sm:grid-cols-2">
              <div>
                <dt className="text-muted-foreground">Object version</dt>
                <dd className="break-all font-mono">
                  {status.receipt.versionId}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">
                  Compliance retention until
                </dt>
                <dd>{status.receipt.retainUntil}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Stored object</dt>
                <dd className="break-all">
                  {status.receipt.bucket}/{status.receipt.key}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">SHA-256</dt>
                <dd className="break-all font-mono">{status.receipt.sha256}</dd>
              </div>
            </dl>
          ) : null}
          {status.lastError ? (
            <p role="alert" className="text-sm">
              Delivery needs attention: {status.lastError}. Inspect the server
              configuration, credentials and bucket policy.
            </p>
          ) : null}
        </>
      )}
    </section>
  );
}
export function AuditLog({ scope }: { scope: RequestScope }) {
  const allowed = scope.role === "owner" || scope.role === "editor";
  const [query, setQuery] = useState(new URLSearchParams());
  const [page, setPage] = useState<AuditPage>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [selected, setSelected] = useState<AuditRecord>();
  const prefix = useId();
  useEffect(() => {
    if (!allowed) return;
    const c = new AbortController();
    setBusy(true);
    setPage(undefined);
    setSelected(undefined);
    setError("");
    readAudit(scope, query, c.signal)
      .then((result) => {
        if (!c.signal.aborted) setPage(result);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!c.signal.aborted) setBusy(false);
      });
    return () => c.abort();
  }, [scope, query, allowed]);
  if (!allowed)
    return (
      <p className="empty-state">
        Tenant-wide audit access requires an owner or editor role. Conversation
        audit records remain available with your conversation permissions.
      </p>
    );
  function search(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const next = new URLSearchParams();
    for (const [key, value] of data) {
      if (typeof value === "string" && value.trim())
        next.set(key, value.trim());
    }
    setQuery(next);
  }
  return (
    <div className="space-y-5" data-private="true">
      <ArchiveEvidence scope={scope} />
      <form
        onSubmit={search}
        className="grid items-end gap-3 rounded-md border p-4 sm:grid-cols-2 lg:grid-cols-4"
      >
        {[
          ["resource", "Resource name"],
          ["actor", "Actor subject"],
          ["action", "Action"],
          ["outcome", "Outcome"],
          ["conversation", "Conversation ID"],
          ["from", "From (RFC3339)"],
          ["to", "To (RFC3339)"],
        ].map(([key, label]) => (
          <div key={key} className="space-y-2">
            <Label htmlFor={prefix + key}>{label}</Label>
            <Input
              id={prefix + key}
              name={key}
              placeholder={
                key === "action"
                  ? "change_set.approved"
                  : key === "outcome"
                    ? "e.g. denied"
                    : undefined
              }
            />
          </div>
        ))}
        <Button type="submit" disabled={busy}>
          Search decisions
        </Button>
      </form>
      <p className="text-xs text-muted-foreground">
        Last 30 days by default. Each page contains at most 50 records. Use the
        CLI for a full NDJSON export. Review notes and frozen definitions may
        contain sensitive configuration.
      </p>
      {error ? <p role="alert">{error}</p> : null}
      {busy ? <p role="status">Loading decisions…</p> : null}
      {page ? (
        <>
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full text-left text-sm">
              <caption className="sr-only">Tenant decision audit log</caption>
              <thead className="bg-muted">
                <tr>
                  {[
                    "Time",
                    "Actor",
                    "Action",
                    "Outcome",
                    "Resources",
                    "Details",
                  ].map((label) => (
                    <th
                      key={label}
                      scope="col"
                      className="px-4 py-3 font-medium"
                    >
                      {label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {page.items.map((item) => (
                  <tr key={item.id} className="border-t align-top">
                    <td className="whitespace-nowrap px-4 py-3">
                      <time dateTime={item.event.at}>
                        {new Date(item.event.at).toLocaleString()}
                      </time>
                    </td>
                    <td className="max-w-52 break-words px-4 py-3">
                      {item.event.actor.name || item.event.actor.subject}
                    </td>
                    <td className="px-4 py-3">{item.event.action}</td>
                    <td className="px-4 py-3">{item.event.outcome}</td>
                    <td className="px-4 py-3">
                      {item.resources
                        .map((resource) => resource.name)
                        .join(", ") || "None"}
                    </td>
                    <td className="px-4 py-3">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setSelected(item)}
                        aria-label={"View decision " + item.id}
                      >
                        View
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {page.items.length === 0 ? (
              <p className="empty-state">No decisions match these filters.</p>
            ) : null}
          </div>
          <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
            <span>
              {page.items.length}{" "}
              {page.items.length === 1 ? "record" : "records"} ·{" "}
              {new Date(page.from).toLocaleDateString()} to{" "}
              {new Date(page.to).toLocaleDateString()}
            </span>
            {page.nextCursor ? (
              <Button
                variant="outline"
                onClick={() => {
                  const next = new URLSearchParams(query);
                  next.set("cursor", page.nextCursor!);
                  setQuery(next);
                }}
              >
                Next page
              </Button>
            ) : null}
          </div>
        </>
      ) : null}
      {selected ? (
        <AuditDetail key={selected.id} scope={scope} record={selected} />
      ) : null}
    </div>
  );
}
