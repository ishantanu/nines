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
  healthOf,
  message,
  percent,
  type Journey,
  type Objective,
  type Point,
  type Snapshot,
} from "@/lib/api";
import { Status } from "./status";
import { SLOIntegrationStatus } from "./integration-status";
import { PageHeader } from "./workspace-chrome";
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
  const sorted = [...items].sort(
    (a, b) =>
      (a.status.budgetRemainingFraction ?? Infinity) -
      (b.status.budgetRemainingFraction ?? Infinity),
  );
  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-6">Objective / service</TableHead>
              <TableHead>Target / observed</TableHead>
              <TableHead>Budget remaining</TableHead>
              <TableHead>Journeys</TableHead>
              <TableHead>Status</TableHead>
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
                    <ArrowUpRight className="ml-1 inline size-3" />
                  </button>
                  <div className="mt-1 text-xs text-muted-foreground">
                    {o.slo.project} / {o.slo.service}
                  </div>
                </TableCell>
                <TableCell>
                  <div className="font-mono text-sm">
                    {percent(o.status.reliability, 3)}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    Target {percent(o.slo.target, 2)}
                  </div>
                </TableCell>
                <TableCell className="min-w-40">
                  <div className="mb-2 flex justify-between gap-4 font-mono text-xs">
                    <span>{percent(o.status.budgetRemainingFraction, 1)}</span>
                    <span className="text-muted-foreground">
                      {o.status.burnRate == null
                        ? "N/A"
                        : `${o.status.burnRate.toFixed(1)}×`}
                    </span>
                  </div>
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
                  <div className="mt-1 text-xs text-muted-foreground">
                    {percent(o.status.coverage, 1)} coverage
                  </div>
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
                  <Status value={healthOf(o.status)} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {!items.length ? (
          <p className="empty-state">No objectives match this view.</p>
        ) : null}
      </CardContent>
    </Card>
  );
}
function SamplePlot({ points }: { points: Point[] }) {
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
        `${20 + ((Date.parse(p.time) - start) / Math.max(1, end - start)) * 760},${120 - ((p.good / p.total - low) / (high - low)) * 100}`,
    )
    .join(" ");
  return (
    <div className="rounded-md border bg-slate-50 p-3">
      <svg
        viewBox="0 0 800 140"
        className="w-full"
        role="img"
        aria-label="Interval reliability over retained samples"
      >
        <line x1="20" y1="120" x2="780" y2="120" stroke="#cbd5e1" />
        <polyline
          points={coords}
          fill="none"
          stroke="#0d9488"
          strokeWidth="2"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
      <div className="flex justify-between text-xs text-muted-foreground">
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
      <PageHeader
        title={objective?.slo.name ?? "Objective"}
        description={`${objective?.slo.project} / ${objective?.slo.service} · ${objective?.slo.id}`}
      >
        <div className="flex flex-wrap gap-2">
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
        {onReplays ? (
          <Button variant="outline" size="sm" onClick={onReplays}>
            Session replay
          </Button>
        ) : null}
        <Button variant="outline" size="sm" onClick={onClose}>
          <ArrowLeft className="size-4" />
          Back
        </Button>
        {objective ? <Status value={healthOf(objective.status)} /> : null}
      </PageHeader>
      {objective ? (
        <SLOIntegrationStatus
          id={objective.slo.id}
          revision={objective.slo.revision}
          scope={scope}
        />
      ) : null}
      {objective?.slo.policies?.length ? (
        <Card className="gap-0 py-0">
          <div className="panel-header">
            <h2 className="section-label">Alert policies</h2>
          </div>
          <CardContent className="p-4">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Policy</TableHead>
                  <TableHead>Severity</TableHead>
                  <TableHead>State</TableHead>
                  <TableHead>Burn threshold</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {objective.slo.policies.map((p) => (
                  <TableRow key={p.name}>
                    <TableCell>{p.name}</TableCell>
                    <TableCell>{p.severity}</TableCell>
                    <TableCell>
                      {objective.status.alert.policies?.[p.name ?? ""]?.phase ??
                        "Not evaluated"}
                    </TableCell>
                    <TableCell>{p.burnRate}×</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
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
              {new Date(objective.measurementHealth.checkedAt).toLocaleString()}
            </p>
          </CardContent>
        </Card>
      ) : null}
      {status ? (
        <dl className="summary-grid">
          {[
            ["Observed", percent(status.reliability, 3)],
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
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      <Tabs value={tab} onValueChange={setTab} className="detail-tabs">
        <TabsList className="detail-tab-list">
          <TabsTrigger value="performance">Performance</TabsTrigger>
          <TabsTrigger value="rules">Rules</TabsTrigger>
          {!objective?.slo.providerWindow ? (
            <TabsTrigger value="samples">Samples</TabsTrigger>
          ) : null}
          <TabsTrigger value="history">Historical status</TabsTrigger>
          <TabsTrigger value="ingest">Import / collect</TabsTrigger>
          <TabsTrigger value="config">Configuration</TabsTrigger>
        </TabsList>
        <TabsContent value="performance">
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
        </TabsContent>
        <TabsContent value="rules">
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
                !canEdit(scope)
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
          {objective && canEdit(scope) ? (
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
