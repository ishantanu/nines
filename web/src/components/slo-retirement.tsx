import { useEffect, useId, useState } from "react";
import { api, message, type RequestScope, type SLO } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { RetirementPlan } from "@/lib/retirement";
export function SLORetirement({
  slo,
  scope,
  onChanged,
  onDeleted,
}: {
  slo: SLO;
  scope: RequestScope;
  onChanged: () => void;
  onDeleted: () => void;
}) {
  const [plan, setPlan] = useState<RetirementPlan | null>(null);
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const id = useId();
  const owner = scope.role === "owner";
  useEffect(() => {
    setPlan(null);
    setConfirm("");
    setError("");
  }, [slo.id, slo.revision, scope]);
  async function review() {
    setBusy(true);
    setError("");
    try {
      const p = await api<RetirementPlan>(
        `/api/slos/${slo.id}/retirement/plan?action=${slo.retirement ? "delete" : "archive"}`,
        scope,
      );
      if (p.tenant !== scope.tenant || p.slo !== slo.id)
        throw new Error("Retirement scope changed. Reload the objective.");
      setPlan(p);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function apply() {
    if (!plan) return;
    setBusy(true);
    setError("");
    try {
      await api(`/api/slos/${slo.id}/retirement/apply`, scope, {
        method: "POST",
        body: JSON.stringify({
          action: plan.action,
          expectedRevision: plan.expectedRevision,
          digest: plan.digest,
          confirmName: confirm,
        }),
      });
      setPlan(null);
      if (plan.action === "delete") onDeleted();
      else onChanged();
    } catch (e) {
      setPlan(null);
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function retry() {
    setBusy(true);
    setError("");
    try {
      await api(`/api/slos/${slo.id}/retirement/retry`, scope, {
        method: "POST",
        body: "{}",
      });
      onChanged();
    } catch (e) {
      setError(message(e));
      onChanged();
    } finally {
      setBusy(false);
    }
  }
  return (
    <section
      aria-label="Objective retirement"
      aria-busy={busy}
      className="space-y-3 border-t pt-6"
    >
      <h2 className="text-base font-semibold">
        {slo.retirement ? "Archived objective" : "Retire objective"}
      </h2>
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Archival stops Nines collection and alert evaluation while preserving
          history. Provider rules can remain active until owned-rule cleanup is
          verified. Review current journey and dependency-map references before
          proceeding.
        </p>
        {slo.retirement ? (
          <div className="space-y-2 text-sm">
            <p>
              Rule cleanup: <strong>{slo.retirement.cleanup}</strong>
            </p>
            <p>{slo.retirement.message}</p>
            <p>{slo.retirement.attempts} cleanup attempts</p>
            {owner && slo.retirement.cleanup !== "complete" ? (
              <Button
                variant="outline"
                className="h-auto min-h-9 max-w-full whitespace-normal"
                disabled={busy}
                onClick={retry}
              >
                Check owned-rule cleanup
              </Button>
            ) : null}
            <details>
              <summary className="cursor-pointer">Retirement audit</summary>
              <ol className="mt-2 space-y-2 break-words">
                {slo.retirement.audit.map((e) => (
                  <li key={e.id}>
                    {e.action} · {e.at} · {e.actor} · revision {e.revision}
                  </li>
                ))}
              </ol>
            </details>
          </div>
        ) : null}
        {owner ? (
          <Button
            variant="outline"
            className="h-auto min-h-9 max-w-full whitespace-normal"
            disabled={busy}
            onClick={review}
          >
            {slo.retirement ? "Review permanent deletion" : "Review archival"}
          </Button>
        ) : (
          <p className="text-sm">A tenant owner must approve retirement.</p>
        )}
        {plan ? (
          <section
            aria-label="Retirement review"
            className="space-y-3 rounded-md border p-4"
          >
            <p className="font-medium">
              {plan.action === "archive" ? "Archive" : "Permanently delete"}{" "}
              {slo.id} · revision {plan.expectedRevision}
            </p>
            {plan.references.length ? (
              <ul className="list-inside list-disc text-sm">
                {plan.references.map((r) => (
                  <li key={`${r.kind}/${r.name}`}>
                    {r.kind === "CriticalUserJourney"
                      ? "Critical user journey"
                      : "Service dependency map"}{" "}
                    {r.name} · revision {r.revision} ·{" "}
                    {r.kind === "ServiceDependencyMap"
                      ? "map reference"
                      : r.required
                        ? "required link"
                        : "advisory link"}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm">
                No current journey or dependency-map references.
              </p>
            )}
            {plan.group ? (
              <p className="break-all text-sm">
                Owned rule group: {plan.group}
              </p>
            ) : null}
            {plan.blockers.map((b) => (
              <p role="status" key={b} className="text-sm">
                {b}
              </p>
            ))}
            <details className="text-sm">
              <summary className="cursor-pointer">
                Reviewed digest and destination
              </summary>
              <p className="break-all font-mono">{plan.digest}</p>
              <p className="break-all font-mono">
                {plan.routeDigest ?? "No managed rule destination"}
              </p>
            </details>
            {plan.action === "delete" ? (
              <div className="space-y-2">
                <p className="text-sm">
                  Permanent deletion removes the definition and retained
                  measurements. The audit receipt and reserved identifier
                  remain.
                </p>
                <Label htmlFor={id}>
                  Type {slo.id} to confirm permanent deletion
                </Label>
                <Input
                  id={id}
                  value={confirm}
                  autoComplete="off"
                  onChange={(e) => setConfirm(e.target.value)}
                />
              </div>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button
                variant={plan.action === "delete" ? "destructive" : "default"}
                className="h-auto min-h-9 max-w-full whitespace-normal"
                disabled={
                  busy ||
                  plan.blockers.length > 0 ||
                  (plan.action === "delete" && confirm !== slo.id)
                }
                onClick={apply}
              >
                {plan.action === "archive"
                  ? "Archive reviewed objective"
                  : "Permanently delete reviewed objective"}
              </Button>
              <Button
                variant="outline"
                onClick={() => setPlan(null)}
                disabled={busy}
              >
                Cancel
              </Button>
            </div>
          </section>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-[var(--status-degraded)]">
            {error}
          </p>
        ) : null}
      </div>
    </section>
  );
}
