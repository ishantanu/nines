import { useEffect, useRef, useState } from "react";
import { api, message, percent, type RequestScope, type SLO } from "@/lib/api";
import { duration } from "@/lib/history";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { type Stage } from "./integration-status";

interface Rule {
  record?: string;
  alert?: string;
  expr: string;
  for?: string;
  labels: Record<string, string>;
  annotations?: Record<string, string>;
}
interface Plan {
  digest: string;
  yaml: string;
  resourceYAML?: string;
  sloRevision: number;
  group: { name: string; interval: string; rules: Rule[] };
  target: {
    connection: string;
    kind: string;
    queryURL: string;
    rulerURL?: string;
    mimirOrgID?: string;
    namespace: string;
    directory?: string;
    reload?: boolean;
    publisher?: string;
    providerKind?: string;
  };
}
export function RecordingRules({
  slo,
  scope,
  onConnections,
  onApplied,
}: {
  slo: SLO;
  scope: RequestScope;
  onConnections: () => void;
  onApplied?: () => void;
}) {
  const applyRequest = useRef<AbortController | null>(null);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [loading, setLoading] = useState(true);
  const [deploying, setDeploying] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<RuleApplyResult | null>(null);
  const [refresh, setRefresh] = useState(0);
  const [stale, setStale] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    applyRequest.current?.abort();
    setDeploying(false);
    setLoading(true);
    setError("");
    setResult(null);
    setPlan(null);
    setStale(false);
    api<Plan>(`/api/slos/${slo.id}/rules/plan`, scope, {
      method: "POST",
      signal: controller.signal,
    })
      .then((value) => {
        if (!controller.signal.aborted) setPlan(value);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      controller.abort();
      applyRequest.current?.abort();
    };
  }, [slo.id, slo.revision, scope, refresh]);
  async function deploy(withdraw = false) {
    if (!plan || stale) return;
    const request = new AbortController();
    applyRequest.current = request;
    setDeploying(true);
    setError("");
    setResult(null);
    try {
      const response = await api<RuleApplyResult>(
        `/api/slos/${slo.id}/rules/${withdraw ? "withdraw" : "apply"}`,
        scope,
        {
          method: "POST",
          body: JSON.stringify({ expectedDigest: plan.digest }),
          signal: request.signal,
        },
      );
      if (!request.signal.aborted) {
        setResult(response);
        if (withdraw) setStale(true);
        onApplied?.();
      }
    } catch (e) {
      if (request.signal.aborted) return;
      setError(message(e));
      setStale(true);
    } finally {
      if (!request.signal.aborted) setDeploying(false);
    }
  }
  const recordings = plan?.group.rules.filter((r) => r.record) ?? [];
  const alerts = plan?.group.rules.filter((r) => r.alert) ?? [];
  const policies = slo.policies ?? (slo.policy ? [slo.policy] : []);
  const destination =
    plan?.target.kind === "operator"
      ? `${plan.target.namespace}/ServiceLevelObjective/${slo.id}`
      : plan?.target.kind === "prometheus"
        ? `${plan.target.directory?.replace(/\/$/, "")}/${plan.target.namespace}-${plan.group.name}.yaml`
        : plan
          ? `${plan.target.rulerURL?.replace(/\/$/, "")}/config/v1/rules/${encodeURIComponent(plan.target.namespace)}`
          : "";
  return (
    <div className="space-y-4">
      <div className="panel-grid">
        <Card className="gap-0 py-0">
          <div className="panel-header">
            <h2 className="section-label">Rule destination</h2>
            <Button variant="ghost" size="sm" onClick={onConnections}>
              Data sources
            </Button>
          </div>
          <CardContent className="space-y-3 p-5">
            <dl className="grid gap-4 text-sm sm:grid-cols-2">
              {Object.entries({
                Connection: plan?.target.connection ?? slo.source ?? "primary",
                Engine:
                  (plan?.target.kind === "operator"
                    ? `Kubernetes operator (${plan.target.providerKind})`
                    : plan?.target.kind) ??
                  (loading ? "Preparing plan" : "Unavailable"),
                Namespace:
                  plan?.target.namespace ??
                  (loading ? "Preparing plan" : "Unavailable"),
                ...(plan?.target.kind === "operator"
                  ? { Publisher: plan.target.publisher ?? "Unavailable" }
                  : {
                      "Mimir tenant":
                        plan?.target.mimirOrgID ??
                        (plan
                          ? "Not applicable"
                          : loading
                            ? "Preparing plan"
                            : "Unavailable"),
                    }),
              }).map(([label, value]) => (
                <div key={label}>
                  <dt className="text-xs text-muted-foreground">{label}</dt>
                  <dd className="mt-1 break-all font-medium">{value}</dd>
                </div>
              ))}
            </dl>
            <p className="text-xs leading-5 text-muted-foreground">
              The SLO's source selects the dataset and server-configured rule
              destination. Recording and alerting rules are deployed together in
              one owned rule group.
              {plan
                ? plan.target.kind === "operator"
                  ? " Reviewed apply publishes a Kubernetes SLO; the operator owns provider rule reconciliation. The namespace, publisher and service account are configured by the administrator."
                  : plan.target.kind === "mimir"
                    ? " Mimir activates the stored group through its ruler."
                    : plan.target.reload
                      ? " Automatic Prometheus reload is enabled after reviewed apply."
                      : " Automatic reload is disabled. Arrange a Prometheus reload after writing the file."
                : ""}
            </p>
          </CardContent>
        </Card>
        <Card className="gap-0 py-0">
          <div className="panel-header">
            <h2 className="section-label">Burn-rate alert policies</h2>
            <Badge variant="secondary">
              {policies.length
                ? `${policies.length} configured`
                : "No policies"}
            </Badge>
          </div>
          <CardContent className="space-y-3 p-5">
            {policies.length ? (
              <div className="space-y-4">
                {policies.map((policy, i) => (
                  <div
                    key={policy.name ?? i}
                    className="space-y-2 border-b pb-3 last:border-0"
                  >
                    <p className="text-sm font-medium">
                      {policy.name ?? "Primary"}{" "}
                      <Badge variant="outline">
                        {policy.severity ?? "critical"}
                      </Badge>
                    </p>
                    <dl className="grid grid-cols-2 gap-2 text-xs">
                      {Object.entries({
                        "Short / long window": `${duration(policy.shortWindowSeconds)} / ${duration(policy.longWindowSeconds)}`,
                        "Burn threshold": `${policy.burnRate}×`,
                        Pending: duration(policy.forSeconds),
                        Coverage: percent(policy.minCoverage, 1),
                      }).map(([label, value]) => (
                        <div key={label}>
                          <dt className="text-muted-foreground">{label}</dt>
                          <dd className="mt-1">{value}</dd>
                        </div>
                      ))}
                    </dl>
                  </div>
                ))}
                <p className="text-xs text-muted-foreground">
                  Every policy requires both burn windows and sufficient
                  coverage. Severity labels support external notification
                  routing.
                </p>
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                Add alert policies in Configuration. Recording rules can be
                deployed without alerts.
              </p>
            )}
          </CardContent>
        </Card>
      </div>
      <Card className="gap-0 py-0">
        <div className="panel-header">
          <h2 className="section-label">Generated rules</h2>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={loading || deploying}
              onClick={() => setRefresh((n) => n + 1)}
            >
              {loading ? "Preparing plan…" : "Refresh plan"}
            </Button>
            {plan ? (
              <Button
                size="sm"
                disabled={
                  loading || deploying || stale || scope.role !== "owner"
                }
                onClick={() => void deploy()}
              >
                {deploying ? "Applying..." : "Apply reviewed rules"}
              </Button>
            ) : null}
            {plan?.target.kind === "operator" ? (
              <Button
                variant="outline"
                size="sm"
                disabled={
                  loading || deploying || stale || scope.role !== "owner"
                }
                onClick={() => void deploy(true)}
              >
                Withdraw reviewed publication
              </Button>
            ) : null}
          </div>
        </div>
        <CardContent className="space-y-4 p-5">
          {error ? (
            <p
              role="alert"
              className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900"
            >
              {error}
              {stale
                ? " Refresh the plan before retrying."
                : " Review the selected connection and SLO queries in Data sources and Configuration."}
            </p>
          ) : null}
          {result ? <RuleApplyEvidence result={result} /> : null}
          {loading ? (
            <p role="status" className="empty-state">
              Preparing rule definitions and destination…
            </p>
          ) : plan ? (
            <>
              <dl className="grid gap-3 text-xs sm:grid-cols-2">
                <div>
                  <dt className="text-muted-foreground">
                    Rule group · evaluation interval
                  </dt>
                  <dd className="mt-1 break-all font-mono">
                    {plan.group.name} · {plan.group.interval}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">
                    Metrics dataset · query endpoint
                  </dt>
                  <dd className="mt-1 break-all">
                    {plan.target.queryURL}
                    {plan.target.mimirOrgID
                      ? ` · tenant ${plan.target.mimirOrgID}`
                      : ""}
                  </dd>
                </div>
                <div className="sm:col-span-2">
                  <dt className="text-muted-foreground">
                    {plan.target.kind === "operator"
                      ? "Kubernetes SLO resource"
                      : plan.target.kind === "mimir"
                        ? "Ruler namespace endpoint"
                        : "Prometheus rule file"}
                  </dt>
                  <dd className="mt-1 break-all font-mono">{destination}</dd>
                </div>
              </dl>
              <Tabs defaultValue="recording" className="min-w-0">
                <TabsList>
                  <TabsTrigger value="recording">
                    Recording rules ({recordings.length})
                  </TabsTrigger>
                  <TabsTrigger value="alerting">
                    Alerting rules ({alerts.length})
                  </TabsTrigger>
                  {plan.resourceYAML ? (
                    <TabsTrigger value="resource">Kubernetes YAML</TabsTrigger>
                  ) : null}
                  <TabsTrigger value="yaml">
                    {plan.resourceYAML ? "Rule preview YAML" : "YAML"}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="recording" className="h-96 overflow-auto">
                  <div className="h-full overflow-auto rounded-md border">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Metric</TableHead>
                          <TableHead>Window</TableHead>
                          <TableHead>Expression</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {recordings.map((rule, i) => (
                          <TableRow key={i}>
                            <TableCell className="align-top font-mono text-xs">
                              {rule.record}
                            </TableCell>
                            <TableCell className="align-top text-xs">
                              {rule.labels.window ?? "Interval"}
                            </TableCell>
                            <TableCell className="max-w-lg whitespace-normal break-words font-mono text-xs leading-5">
                              {rule.expr}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                </TabsContent>
                <TabsContent
                  value="alerting"
                  className="h-96 space-y-4 overflow-auto"
                >
                  {alerts.length ? (
                    alerts.map((rule) => (
                      <div key={rule.alert} className="space-y-3">
                        <h3 className="text-sm font-semibold">{rule.alert}</h3>
                        <p className="text-sm text-muted-foreground">
                          {rule.annotations?.summary} · pending{" "}
                          {rule.for ?? "0s"}
                        </p>
                        <pre className="code-block">{rule.expr}</pre>
                        <dl className="flex flex-wrap gap-x-5 gap-y-2 text-xs">
                          {Object.entries(rule.labels).map(([label, value]) => (
                            <div key={label}>
                              <dt className="text-muted-foreground">{label}</dt>
                              <dd className="mt-1 font-mono">{value}</dd>
                            </div>
                          ))}
                        </dl>
                      </div>
                    ))
                  ) : (
                    <p className="empty-state">
                      No burn-rate alert policy configured.
                    </p>
                  )}
                </TabsContent>
                {plan.resourceYAML ? (
                  <TabsContent value="resource" className="h-96 overflow-auto">
                    <pre className="code-block h-full max-h-full">
                      {plan.resourceYAML}
                    </pre>
                  </TabsContent>
                ) : null}
                <TabsContent value="yaml" className="h-96 overflow-auto">
                  <pre className="code-block h-full max-h-full">
                    {plan.yaml}
                  </pre>
                </TabsContent>
              </Tabs>
              {plan.resourceYAML ? (
                <p className="text-xs leading-5 text-muted-foreground">
                  Publish the Kubernetes resource through Nines so the reviewed
                  destination is recorded. A new resource receives its owner UID
                  at publication; generated rules are a preview until then.
                </p>
              ) : null}
              <p className="break-all text-xs text-muted-foreground">
                SLO revision {plan.sloRevision} · plan digest{" "}
                <span className="font-mono">{plan.digest}</span>
              </p>
              {scope.role !== "owner" ? (
                <p className="text-xs text-muted-foreground">
                  An owner role is required to deploy rules.
                </p>
              ) : null}
            </>
          ) : (
            <p className="empty-state">
              No deployable plan. Configure a rule destination and both
              interval-count queries to generate rules.
            </p>
          )}
        </CardContent>
      </Card>
      <Card className="gap-0 py-0">
        <div className="panel-header">
          <h2 className="section-label">
            Evaluation and notification delivery
          </h2>
          <Badge variant="outline">Check Integration status above</Badge>
        </div>
        <CardContent className="space-y-2 p-5 text-sm leading-6 text-muted-foreground">
          <p>
            Mimir evaluates the deployed group in its ruler and writes recorded
            series to the configured Mimir tenant. Prometheus requires the
            generated file in <code>rule_files</code> and a reload; remote-write
            routing is configured in Prometheus.
          </p>
          <p>
            Configure the ruler's Alertmanager integration and notification
            routes separately. The Nines alert activity view shows its own
            evaluator transitions; it does not ingest external ruler alerts. A
            successful deployment does not verify evaluation or delivery.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

interface RuleApplyResult {
  state: string;
  message: string;
  activation?: Stage;
  evaluation?: Stage;
}
export function RuleApplyEvidence({ result }: { result: RuleApplyResult }) {
  const saved =
    result.state === "submitted"
      ? "Published to Kubernetes"
      : result.state === "withdrawing"
        ? "Withdrawal pending"
        : result.state === "withdrawn"
          ? "Publication withdrawn"
          : result.state === "written"
            ? "File written"
            : result.state === "accepted" || result.state === "verified"
              ? "Stored in ruler"
              : result.state === "unchanged"
                ? "Configuration unchanged"
                : "Configuration result";
  const stateLabels: Record<string, string> = {
    accepted: "Accepted",
    verified: "Verified",
    failed: "Failed",
    blocked: "Blocked",
    manual: "Manual reload required",
    unknown: "Unverified",
    not_loaded: "Not loaded",
    evaluating: "Evaluating",
    pending: "Pending",
    out_of_date: "Out of date",
    stale: "Stale",
  };
  return (
    <div
      role="status"
      className="grid gap-4 rounded-md border p-4 text-sm md:grid-cols-3"
    >
      {[
        {
          title: "Configuration",
          state: result.state,
          message: result.message,
          label: saved,
        },
        {
          ...result.activation,
          title: "Backend activation",
          label:
            stateLabels[result.activation?.state ?? "unknown"] ?? "Unverified",
        },
        {
          ...result.evaluation,
          title: "Runtime evaluation",
          label:
            stateLabels[result.evaluation?.state ?? "unknown"] ?? "Unverified",
        },
      ].map((stage) => (
        <div key={stage.title} className="min-w-0 space-y-2">
          <h3 className="font-medium">{stage.title}</h3>
          <Badge
            variant={
              stage.state === "failed" || stage.state === "blocked"
                ? "destructive"
                : "outline"
            }
          >
            {stage.label}
          </Badge>
          <p className="break-words text-xs leading-5 text-muted-foreground">
            {stage.message ?? "Check integration status for current evidence."}
          </p>
          {"nextAction" in stage && stage.nextAction ? (
            <p className="text-xs leading-5">Next step: {stage.nextAction}</p>
          ) : null}
        </div>
      ))}
    </div>
  );
}
