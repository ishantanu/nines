import { SLORetirement } from "./slo-retirement";
import { useChartSize } from "@/lib/use-chart-size";
import { ReplayLabel } from "@/components/replay-label";
import { lazy, Suspense, useEffect, useState } from "react";
import { stringify } from "yaml";
import { definitionFromSLO } from "@/lib/definitions";
import { ArrowLeft, ArrowUpRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Progress } from "@/components/ui/progress";
import {
  api,
  canEdit,
  type RequestScope,
  message,
  percent,
  type Journey,
  type Objective,
  type Point,
  type Snapshot,
} from "@/lib/api";
import { SLIValue, SLOAlertState } from "./sli-value";
import { SLOIntegrationStatus } from "./integration-status";
import { BudgetDecisionPanel } from "./budget-decision";
import { MeasurementQualityPanel } from "./measurement-quality";
import { SLOHealthNotice } from "./slo-health-notice";
import { PageHeader } from "./workspace-chrome";
const SourceMeasurements = lazy(() =>
  import("./source-measurements").then((m) => ({
    default: m.SourceMeasurements,
  })),
);
const SLOPerformance = lazy(() =>
  import("./slo-performance").then((m) => ({ default: m.SLOPerformance })),
);
const ObjectiveCreate = lazy(() =>
  import("./objective-create").then((m) => ({ default: m.ObjectiveCreate })),
);
const RecordingRules = lazy(() =>
  import("./recording-rules").then((m) => ({ default: m.RecordingRules })),
);
export function Objectives({
  items,
  journeys,
  onSelect,
  onJourney,
}: {
  items: Objective[];
  journeys: Journey[];
  onSelect: (id: string) => void;
  onJourney: (id: string) => void;
}) {
  const [archived, setArchived] = useState(false);
  const visible = items.filter((o) => Boolean(o.slo.retirement) === archived);
  const sorted = [...visible].sort(
    (a, b) =>
      (a.status.budgetRemainingFraction ?? Infinity) -
      (b.status.budgetRemainingFraction ?? Infinity),
  );
  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-0">
        <div
          className="flex flex-wrap gap-2 border-b px-4 py-3"
          role="group"
          aria-label="Objective lifecycle filter"
        >
          <Button
            variant={archived ? "ghost" : "secondary"}
            aria-pressed={!archived}
            onClick={() => setArchived(false)}
          >
            Active objectives
          </Button>
          <Button
            variant={archived ? "secondary" : "ghost"}
            aria-pressed={archived}
            onClick={() => setArchived(true)}
          >
            Archived objectives
          </Button>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-6">
                <ReplayLabel>Objective / service</ReplayLabel>
              </TableHead>
              <TableHead>
                <ReplayLabel>SLI / SLO target</ReplayLabel>
              </TableHead>
              <TableHead>
                <ReplayLabel>Budget remaining</ReplayLabel>
              </TableHead>
              <TableHead>
                <ReplayLabel>Journeys</ReplayLabel>
              </TableHead>
              <TableHead>
                <ReplayLabel>Alerts</ReplayLabel>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((o) => (
              <TableRow key={o.slo.id}>
                <TableCell className="py-5 pl-6">
                  <button
                    onClick={() => onSelect(o.slo.id)}
                    className="text-left font-medium hover:text-primary"
                  >
                    {o.slo.name}
                    {o.slo.retirement ? (
                      <span className="ml-2 text-sm text-muted-foreground">
                        (Archived)
                      </span>
                    ) : null}
                    <ArrowUpRight className="ml-1 inline size-3" />
                  </button>
                  <div className="mt-1 text-xs text-muted-foreground">
                    {o.slo.project} / {o.slo.service}
                  </div>
                </TableCell>
                <TableCell>
                  <SLIValue
                    status={o.status}
                    target={o.slo.target}
                    showAlerts={false}
                  />
                </TableCell>
                <TableCell className="min-w-40">
                  <div className="mb-2 flex justify-between gap-4 font-mono text-xs">
                    <span>
                      {o.status.budgetRemainingFraction === null
                        ? "No estimate"
                        : percent(o.status.budgetRemainingFraction, 1)}
                    </span>
                    <span className="text-muted-foreground">
                      {o.status.burnRate == null
                        ? "N/A"
                        : `${o.status.burnRate.toFixed(1)}×`}
                    </span>
                  </div>
                  {o.status.budgetRemainingFraction !== null ? (
                    <Progress
                      value={Math.max(
                        0,
                        Math.min(
                          100,
                          (o.status.budgetRemainingFraction ?? 0) * 100,
                        ),
                      )}
                      aria-label={`Budget remaining for ${o.slo.name}`}
                    />
                  ) : null}
                  {o.status.reliability !== null && o.status.coverage < 1 ? (
                    <div className="mt-1 text-xs text-muted-foreground">
                      <ReplayLabel>Based on observed data</ReplayLabel>
                    </div>
                  ) : null}
                </TableCell>
                <TableCell>
                  <div className="flex max-w-48 flex-wrap gap-1">
                    {journeys
                      .filter((j) =>
                        j.links.some((l) => l.link.id === o.slo.id),
                      )
                      .map((j) => (
                        <button
                          key={j.resource.manifest.metadata.name}
                          onClick={() =>
                            onJourney(j.resource.manifest.metadata.name)
                          }
                          className="rounded border bg-slate-50 px-2 py-1 text-xs hover:border-primary"
                        >
                          {j.resource.manifest.spec.displayName}
                        </button>
                      ))}
                  </div>
                </TableCell>
                <TableCell>
                  <SLOAlertState status={o.status} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {!visible.length ? (
          <p className="empty-state">
            <ReplayLabel>No objectives match this view.</ReplayLabel>
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
function SamplePlot({ points }: { points: Point[] }) {
  const { ref, width, height } = useChartSize();
  const values = points.filter((p) => p.total > 0);
  if (!values.length) return <p className="empty-state">No traffic to plot.</p>;
  const start = Date.parse(values[0].time),
    end = Date.parse(values[values.length - 1].time);
  const ratios = values.map((p) => p.good / p.total);
  let minimum = 1;
  for (const ratio of ratios) minimum = Math.min(minimum, ratio);
  const low = Math.max(0, minimum - 0.001);
  const high = 1;
  const plot =
    values.length > 1200
      ? values.filter(
          (_, i) =>
            i % Math.ceil(values.length / 1200) === 0 ||
            i === values.length - 1,
        )
      : values;
  const coords = plot
    .map(
      (p) =>
        `${12 + ((Date.parse(p.time) - start) / Math.max(1, end - start)) * Math.max(1, width - 24)},${height - 16 - ((p.good / p.total - low) / (high - low)) * Math.max(1, height - 32)}`,
    )
    .join(" ");
  return (
    <div className="rounded-md border bg-slate-50 p-3">
      <div ref={ref} className="h-44 min-w-0 sm:h-56">
        <svg
          viewBox={`0 0 ${width} ${height}`}
          className="size-full"
          role="img"
          aria-label="Interval reliability over retained samples"
        >
          <line
            x1={12}
            y1={height - 16}
            x2={width - 12}
            y2={height - 16}
            stroke="var(--border)"
          />
          <polyline
            points={coords}
            fill="none"
            stroke="#0d9488"
            strokeWidth="2"
            vectorEffect="non-scaling-stroke"
          />
        </svg>
      </div>
      <div className="flex flex-wrap justify-between gap-2 text-xs text-muted-foreground">
        <span>{new Date(start).toLocaleString()}</span>
        <span>Scale {percent(low, 2)} to 100%</span>
        <span>{new Date(end).toLocaleString()}</span>
      </div>
    </div>
  );
}
export function ObjectiveDetail({
  objective,
  scope,
  journeys,
  onClose,
  onJourney,
  onRefresh,
  onConnections,
  onReplays,
  refreshKey = 0,
}: {
  objective: Objective | undefined;
  scope: RequestScope;
  journeys: Journey[];
  onClose: () => void;
  onJourney: (id: string) => void;
  onRefresh: () => void;
  onConnections: () => void;
  onReplays?: () => void;
  refreshKey?: number;
}) {
  const [tab, setTab] = useState("performance");
  const [points, setPoints] = useState<Point[]>([]);
  const [error, setError] = useState("");
  const [text, setText] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [at, setAt] = useState("");
  const [historical, setHistorical] = useState<Snapshot | null>(null);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const id = objective?.slo.id;
  useEffect(() => {
    setHistorical(null);
    setAt("");
    setText("");
    setNotice("");
    setError("");
    setPoints([]);
    setFrom("");
    setTo("");
  }, [id, scope]);
  useEffect(() => {
    if (!id || tab !== "samples") return;
    setLoading(true);
    const c = new AbortController();
    api<Point[]>(`/api/slos/${id}/samples`, scope, { signal: c.signal })
      .then((result) => {
        if (!c.signal.aborted) setPoints(result);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [id, scope, tab, refresh, refreshKey]);
  async function action(kind: "import" | "collect" | "history") {
    if (!id) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      if (kind === "history") {
        const result = await api<Snapshot>(
          `/api/slos/${id}/status?at=${encodeURIComponent(new Date(at).toISOString())}`,
          scope,
        );
        setHistorical(result);
      } else {
        const body: unknown =
          kind === "import"
            ? JSON.parse(text)
            : {
                from: new Date(from).toISOString(),
                to: new Date(to).toISOString(),
              };
        const result = await api<Record<string, number>>(
          `/api/slos/${id}/${kind === "import" ? "samples" : "collect"}`,
          scope,
          { method: "POST", body: JSON.stringify(body) },
        );
        onRefresh();
        setRefresh((n) => n + 1);
        setNotice(`${result.accepted ?? result.collected} samples committed.`);
      }
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const status = historical ?? objective?.status;
  return (
    <section className="resource-detail" aria-label="Objective detail">
      <Button
        variant="ghost"
        size="sm"
        onClick={onClose}
        className="mb-3 -ml-2"
      >
        <ArrowLeft className="size-4" aria-hidden="true" />
        All objectives
      </Button>
      <PageHeader
        title={objective?.slo.name ?? "Objective"}
        description={`${objective?.slo.project} / ${objective?.slo.service} · ${objective?.slo.id}`}
      >
        {objective ? <SLOAlertState status={objective.status} /> : null}
        {onReplays ? (
          <Button variant="outline" size="sm" onClick={onReplays}>
            <ReplayLabel>Session replay</ReplayLabel>
          </Button>
        ) : null}
      </PageHeader>
      {objective?.budgetDecision &&
      !objective.slo.retirement &&
      objective.slo.errorBudgetPolicy ? (
        <BudgetDecisionPanel decision={objective.budgetDecision} />
      ) : null}
      {status ? (
        <dl className="summary-grid">
          {[
            [
              "Observed SLI",
              status.reliability === null
                ? status.dataStatus === "no_traffic"
                  ? "No traffic"
                  : "No SLI data"
                : percent(status.reliability, 3),
            ],
            ["Budget left", percent(status.budgetRemainingFraction, 1)],
            [
              "Burn rate",
              status.burnRate == null
                ? "N/A"
                : `${status.burnRate.toFixed(2)}×`,
            ],
            ["Data coverage", percent(status.coverage, 1)],
          ].map(([label, value]) => (
            <div className="summary-metric" key={label}>
              <dt>
                <ReplayLabel>{label}</ReplayLabel>
              </dt>
              <dd>
                {value}
                {label === "Observed SLI" && objective ? (
                  <p className="text-xs text-muted-foreground">
                    Target {percent(objective.slo.target, 3)}
                  </p>
                ) : null}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
      {objective?.slo.retirement ? (
        <p role="status" className="rounded-md border p-4 text-sm">
          Archived. The summary is frozen at archival; retained historical
          measurements remain available. Rule cleanup:{" "}
          {objective.slo.retirement.cleanup}.
        </p>
      ) : status ? (
        <SLOHealthNotice status={status} />
      ) : null}
      <Tabs value={tab} onValueChange={setTab} className="detail-tabs">
        <TabsList className="detail-tab-list">
          <TabsTrigger value="performance">
            <ReplayLabel>Performance</ReplayLabel>
          </TabsTrigger>
          <TabsTrigger value="measurement">Measurement</TabsTrigger>
          <TabsTrigger value="rules">
            <ReplayLabel>Rules</ReplayLabel>
          </TabsTrigger>
          {!objective?.slo.providerWindow ? (
            <TabsTrigger value="samples">
              <ReplayLabel>Samples</ReplayLabel>
            </TabsTrigger>
          ) : null}
          <TabsTrigger value="history">
            <ReplayLabel>Historical status</ReplayLabel>
          </TabsTrigger>
          <TabsTrigger value="ingest">
            <ReplayLabel>Import / collect</ReplayLabel>
          </TabsTrigger>
          <TabsTrigger value="config">
            <ReplayLabel>Configuration</ReplayLabel>
          </TabsTrigger>
        </TabsList>
        <TabsContent value="performance">
          {objective &&
          !objective.slo.retirement &&
          (objective.measurementHealth?.evaluationBlocked ||
            (objective.quality && objective.quality.state !== "usable")) ? (
            <div
              role="status"
              className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-md border p-3 text-sm"
            >
              <p>
                {objective.measurementHealth?.evaluationBlocked
                  ? objective.measurementHealth.message
                  : "Measurement evidence needs review before relying on this SLI."}
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setTab("measurement")}
              >
                Review measurement
              </Button>
            </div>
          ) : null}
          {objective ? (
            <Suspense
              fallback={
                <p role="status" className="empty-state">
                  Loading performance charts…
                </p>
              }
            >
              <SLOPerformance
                key={objective.slo.id}
                slo={objective.slo}
                at={objective.status.at}
                scope={scope}
                refreshKey={refreshKey}
              />
            </Suspense>
          ) : null}
          {journeys.some((j) => j.links.some((l) => l.link.id === id)) ? (
            <section
              className="mt-5 border-t pt-4"
              aria-label="Linked critical user journeys"
            >
              <h2 className="mb-2 text-sm font-semibold">
                Linked critical user journeys
              </h2>
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                {journeys
                  .filter((j) => j.links.some((l) => l.link.id === id))
                  .map((j) => (
                    <Button
                      variant="outline"
                      size="sm"
                      key={j.resource.manifest.metadata.name}
                      onClick={() => {
                        onClose();
                        onJourney(j.resource.manifest.metadata.name);
                      }}
                    >
                      {j.resource.manifest.spec.displayName}
                      <ArrowUpRight className="size-3" />
                    </Button>
                  ))}
              </div>
            </section>
          ) : null}
        </TabsContent>
        <TabsContent value="measurement" className="space-y-4">
          {objective && !objective.slo.retirement ? (
            <Suspense
              fallback={<p role="status">Loading source measurements…</p>}
            >
              <SourceMeasurements
                scope={scope}
                slo={objective.slo}
                collection={objective.collection}
                onConnections={onConnections}
                onCollected={() => {
                  onRefresh();
                  setRefresh((n) => n + 1);
                }}
              />
            </Suspense>
          ) : null}
          {objective?.quality && !objective.slo.retirement ? (
            <MeasurementQualityPanel
              report={objective.quality}
              scope={scope}
              cohortLabels={objective.slo.quality?.cohortLabels}
            />
          ) : null}
          {objective?.measurementHealth ? (
            <Card className="gap-0 py-0">
              <CardContent className="space-y-1 p-4 text-sm" role="status">
                <p className="font-medium">
                  Measurement contract: {objective.measurementHealth.state}
                </p>
                <p className="text-muted-foreground">
                  {objective.measurementHealth.message}
                </p>
                <p className="text-xs text-muted-foreground">
                  Checked{" "}
                  {new Date(
                    objective.measurementHealth.checkedAt,
                  ).toLocaleString()}
                </p>
              </CardContent>
            </Card>
          ) : null}
        </TabsContent>
        <TabsContent value="rules" className="space-y-4">
          {objective && !objective.slo.retirement ? (
            <SLOIntegrationStatus
              id={objective.slo.id}
              revision={objective.slo.revision}
              refreshKey={refresh + refreshKey}
              scope={scope}
            />
          ) : null}

          {objective?.slo.policies?.length ? (
            <Card className="gap-0 py-0">
              <div className="panel-header">
                <h2 className="section-label">
                  <ReplayLabel>Alert policies</ReplayLabel>
                </h2>
              </div>
              <CardContent className="p-4">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>
                        <ReplayLabel>Policy</ReplayLabel>
                      </TableHead>
                      <TableHead>
                        <ReplayLabel>Severity</ReplayLabel>
                      </TableHead>
                      <TableHead>
                        <ReplayLabel>State</ReplayLabel>
                      </TableHead>
                      <TableHead>
                        <ReplayLabel>Burn threshold</ReplayLabel>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {objective.slo.policies.map((p) => (
                      <TableRow key={p.name}>
                        <TableCell>{p.name}</TableCell>
                        <TableCell>{p.severity}</TableCell>
                        <TableCell>
                          {objective.status.alert.policies?.[p.name ?? ""]
                            ?.phase ?? "Not evaluated"}
                        </TableCell>
                        <TableCell>{p.burnRate}×</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          ) : null}

          {objective ? (
            <Suspense
              fallback={
                <p role="status" className="empty-state">
                  Loading rules…
                </p>
              }
            >
              <RecordingRules
                key={`${objective.slo.id}-${objective.slo.revision ?? 1}`}
                slo={objective.slo}
                scope={scope}
                onConnections={onConnections}
                onApplied={() => setRefresh((n) => n + 1)}
              />
            </Suspense>
          ) : null}
        </TabsContent>
        <TabsContent value="samples" className="detail-panel">
          {loading ? (
            <p className="empty-state">Loading retained samples…</p>
          ) : (
            <SamplePlot points={points} />
          )}
          <p className="mt-2 text-xs text-muted-foreground">
            {points.length} retained intervals. Chart excludes zero-traffic
            intervals.
          </p>
        </TabsContent>
        <TabsContent value="history" className="detail-panel">
          <Label htmlFor="historical-at">Evaluate at a past time</Label>
          <div className="mt-2 flex gap-2">
            <Input
              id="historical-at"
              type="datetime-local"
              value={at}
              onChange={(e) => setAt(e.target.value)}
            />
            <Button disabled={!at || busy} onClick={() => action("history")}>
              Evaluate
            </Button>

            <Button variant="outline" onClick={() => setHistorical(null)}>
              Current
            </Button>
          </div>
          {historical ? (
            <p className="mt-3 text-xs text-muted-foreground">
              Evaluated at {new Date(historical.at).toLocaleString()}. Reading
              history does not change alert state.
            </p>
          ) : null}
        </TabsContent>
        <TabsContent value="ingest" className="detail-panel space-y-5">
          <div>
            <Label htmlFor="sample-import">Import interval counts (JSON)</Label>
            <Textarea
              id="sample-import"
              className="mt-2 min-h-28 font-mono text-xs"
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder={
                '{"samples":[{"time":"2026-10-18T12:00:00Z","good":999,"total":1000}]}'
              }
            />
            <Button
              className="mt-2"
              disabled={
                busy ||
                !text ||
                !canEdit(scope) ||
                !!objective?.slo.providerWindow
              }
              onClick={() => action("import")}
            >
              Import samples
            </Button>
          </div>
          <div className="border-t pt-4">
            <p className="mb-3 text-sm font-medium">
              Replay from configured metrics source
            </p>
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor="collect-from">From (exclusive)</Label>
                <Input
                  id="collect-from"
                  className="mt-2"
                  type="datetime-local"
                  value={from}
                  onChange={(e) => setFrom(e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="collect-to">To (inclusive)</Label>
                <Input
                  id="collect-to"
                  className="mt-2"
                  type="datetime-local"
                  value={to}
                  onChange={(e) => setTo(e.target.value)}
                />
              </div>
            </div>
            <Button
              className="mt-3"
              variant="outline"
              disabled={
                busy ||
                !from ||
                !to ||
                (!objective?.slo.goodQuery && !objective?.slo.providerWindow) ||
                !canEdit(scope) ||
                Boolean(objective?.slo.retirement)
              }
              onClick={() => action("collect")}
            >
              Collect & replace range
            </Button>
            <p className="mt-2 text-xs text-muted-foreground">
              Requires configured queries and a server-side Prometheus or Mimir
              connection. Use times aligned to the SLO step.
            </p>
          </div>
        </TabsContent>
        <TabsContent value="config" className="detail-panel space-y-4">
          {objective && !objective.slo.retirement && canEdit(scope) ? (
            <Suspense fallback={null}>
              <ObjectiveCreate
                scope={scope}
                resource={objective.slo}
                onCreated={() => {
                  onRefresh();
                  setRefresh((n) => n + 1);
                }}
              />
            </Suspense>
          ) : null}
          <Button
            size="sm"
            variant="outline"
            className="mb-3"
            onClick={() => {
              if (!objective) return;
              const blob = new Blob(
                [stringify(definitionFromSLO(objective.slo))],
                { type: "application/yaml" },
              );
              const url = URL.createObjectURL(blob);
              const link = document.createElement("a");
              link.href = url;
              link.download = `${objective.slo.id}.slo.yaml`;
              link.click();
              setTimeout(() => URL.revokeObjectURL(url), 1000);
            }}
          >
            Download YAML
          </Button>
          <pre className="code-block">
            {objective ? stringify(definitionFromSLO(objective.slo)) : ""}
          </pre>
          {objective ? (
            <SLORetirement
              slo={objective.slo}
              scope={scope}
              onChanged={() => {
                onRefresh();
                setRefresh((n) => n + 1);
              }}
              onDeleted={() => {
                onRefresh();
                onClose();
              }}
            />
          ) : null}
        </TabsContent>
      </Tabs>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {notice ? (
        <p role="status" className="text-sm text-emerald-700">
          {notice}
        </p>
      ) : null}
    </section>
  );
}
