import { useEffect, useState } from "react";
import { api, message, type RequestScope } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

export interface Stage {
  state: string;
  message: string;
  code?: string;
  nextAction?: string;
  observedAt?: string;
}
export interface IntegrationStatus {
  revision: number;
  checkedAt: string;
  definition: Stage;
  collection: Stage;
  deployment: Stage;
  evaluation: Stage;
  ruleOutput?: Stage;
  automation?: Stage & {
    enabled: boolean;
    paused?: boolean;
    revision: number;
    nextAttemptAt?: string;
  };
}
const stages = [
  ["definition", "SLO saved"],
  ["collection", "Measurements collected"],
  ["deployment", "Rule configuration"],
  ["evaluation", "Rules evaluating"],
] as const;
const labels: Record<string, string> = {
  manual: "Manual",
  applying: "Applying",
  paused: "Paused",
  accepted: "Accepted",
  saved: "Saved",
  collected: "Collected",
  deployed: "Configured",
  written: "File written",
  stored: "Stored in ruler",
  configured: "Resource configured",
  submitted: "Awaiting operator",
  withdrawing: "Withdrawal pending",
  evaluating: "Evaluating",
  not_collected: "Not collected",
  not_configured: "Not configured",
  not_deployed: "Not deployed",
  not_loaded: "Not loaded",
  out_of_date: "Out of date",
  unknown: "Unverified",
  empty: "No measurements",
  failed: "Failed",
  stale: "Stale",
  pending: "Pending",
  blocked: "Blocked",
  verified: "Verified",
  not_observed: "Not observed",
};
export function IntegrationEvidence({ status }: { status: IntegrationStatus }) {
  return (
    <div className="grid grid-cols-1 divide-y rounded-md border md:grid-cols-2 md:divide-y-0 xl:grid-cols-4">
      {stages.map(([key, title]) => {
        const stage = status[key];
        return (
          <div key={key} className="min-w-0 space-y-2 p-4">
            <h3 className="text-sm font-medium">{title}</h3>
            <Badge
              variant={
                stage.state === "failed" || stage.state === "blocked"
                  ? "destructive"
                  : "outline"
              }
            >
              {labels[stage.state] ?? "Unverified"}
            </Badge>
            <p className="break-words text-xs leading-5 text-muted-foreground">
              {stage.message}
            </p>
            {stage.nextAction ? (
              <p className="break-words text-xs leading-5">
                <span className="font-medium">Next step: </span>
                {stage.nextAction}
              </p>
            ) : null}
            {stage.observedAt ? (
              <p className="text-xs text-muted-foreground">
                Observed {new Date(stage.observedAt).toLocaleString()}
              </p>
            ) : null}
          </div>
        );
      })}
      {status.automation ? (
        <div className="col-span-full min-w-0 space-y-2 border-t p-4">
          <h3 className="text-sm font-medium">Automatic rule deployment</h3>
          <Badge
            variant={
              status.automation.state === "failed" ||
              status.automation.state === "blocked"
                ? "destructive"
                : "outline"
            }
          >
            {status.automation.state === "configured"
              ? "Applied"
              : (labels[status.automation.state] ?? status.automation.state)}
          </Badge>
          <p className="text-xs leading-5 text-muted-foreground">
            {status.automation.message}
          </p>
          {status.automation.nextAction ? (
            <p className="text-xs leading-5">{status.automation.nextAction}</p>
          ) : null}
        </div>
      ) : null}
      {status.ruleOutput ? (
        <div className="col-span-full min-w-0 space-y-2 border-t p-4">
          <h3 className="text-sm font-medium">Rule output in query dataset</h3>
          <Badge variant="outline">
            {labels[status.ruleOutput.state] ?? "Unverified"}
          </Badge>
          <p className="break-words text-xs leading-5 text-muted-foreground">
            {status.ruleOutput.message}
          </p>
          {status.ruleOutput.nextAction ? (
            <p className="break-words text-xs leading-5">
              <span className="font-medium">Next step: </span>
              {status.ruleOutput.nextAction}
            </p>
          ) : null}
          {status.ruleOutput.observedAt ? (
            <p className="text-xs text-muted-foreground">
              Stored sample observed{" "}
              {new Date(status.ruleOutput.observedAt).toLocaleString()}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
export function SLOIntegrationStatus({
  id,
  revision,
  scope,
  refreshKey = 0,
}: {
  id: string;
  revision?: number;
  scope: RequestScope;
  refreshKey?: number;
}) {
  const [evidence, setEvidence] = useState<{
    scope: RequestScope;
    id: string;
    revision?: number;
    value: IntegrationStatus;
  } | null>(null);
  const status =
    evidence?.scope === scope &&
    evidence.id === id &&
    evidence.revision === revision
      ? evidence.value
      : null;
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [busy, setBusy] = useState(true);
  const [changing, setChanging] = useState(false);
  async function toggleAutomation() {
    if (!status?.automation) return;
    setChanging(true);
    setError("");
    try {
      await api(`/api/slos/${id}/rules/automation`, scope, {
        method: "POST",
        body: JSON.stringify({
          expectedRevision: status.revision,
          paused: !status.automation.paused,
        }),
      });
      if (!scope.signal.aborted) setRefresh((n) => n + 1);
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setChanging(false);
    }
  }
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    setEvidence(null);
    async function check() {
      setBusy(true);
      try {
        const value = await api<IntegrationStatus>(
          `/api/slos/${id}/integration`,
          scope,
          { signal: controller.signal },
        );
        if (!controller.signal.aborted) {
          setEvidence({ scope, id, revision, value });
          setError("");
        }
      } catch (e) {
        if (!controller.signal.aborted) {
          setEvidence(null);
          setError(message(e));
        }
      } finally {
        if (!controller.signal.aborted) {
          setBusy(false);
          timer = setTimeout(check, 15000);
        }
      }
    }
    void check();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [id, revision, scope, refreshKey, refresh]);
  return (
    <Card className="gap-0 py-0">
      <div className="panel-header">
        <h2 className="section-label">Integration status</h2>
        <Button
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={() => setRefresh((n) => n + 1)}
        >
          {busy ? "Checking..." : "Check status"}
        </Button>
      </div>
      <CardContent className="space-y-3 p-4" aria-live="polite">
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {status ? (
          <div className="space-y-3">
            <IntegrationEvidence status={status} />
            {scope.role === "owner" && status.automation?.enabled ? (
              <Button
                variant="outline"
                size="sm"
                disabled={busy || changing}
                onClick={() => void toggleAutomation()}
              >
                {changing
                  ? "Updating..."
                  : status.automation.paused
                    ? "Resume automatic deployment"
                    : "Pause automatic deployment"}
              </Button>
            ) : null}
          </div>
        ) : !error ? (
          <p className="text-sm text-muted-foreground">
            Checking saved definition, collection, deployment and external
            evaluation...
          </p>
        ) : null}
        <p className="text-xs leading-5 text-muted-foreground">
          Nines collection and external rule evaluation are independent. Rule
          health does not verify traffic coverage or Alertmanager delivery.
        </p>
        {status ? (
          <p className="text-xs text-muted-foreground">
            Revision {status.revision} · checked{" "}
            {new Date(status.checkedAt).toLocaleString()}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
