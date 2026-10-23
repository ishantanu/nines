import { useEffect, useState } from "react";
import { parse, stringify } from "yaml";
import { api, message, type RequestScope } from "@/lib/api";
import {
  bindingKind,
  bindingStarter,
  type BindingManifest,
  type Discovery,
  type TopologyCatalog,
  type TopologyItem,
} from "@/lib/topology";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { MeasurementEditor } from "./measurement-editor";
import { TopologyEditor } from "./topology-editor";
interface Compiled {
  yaml: string;
  digest: string;
  compilerVersion: string;
  warnings: string[];
}
export function MeasurementBindings({ scope }: { scope: RequestScope }) {
  const [health, setHealth] = useState<{
    state: string;
    message: string;
    checkedAt: string;
    fingerprint?: string;
  } | null>(null);
  const [connections, setConnections] = useState<
      { id: string; kind: string }[]
    >([]),
    [source, setSource] = useState(""),
    [metric, setMetric] = useState("http_requests_total"),
    [labels, setLabels] = useState(""),
    [discovery, setDiscovery] = useState<Discovery | null>(null),
    [names, setNames] = useState<string[]>([]),
    [truncated, setTruncated] = useState(false),
    [items, setItems] = useState<TopologyItem[]>([]),
    [cursor, setCursor] = useState(""),
    [id, setID] = useState(""),
    [binding, setBinding] = useState<BindingManifest | null>(null),
    [revision, setRevision] = useState(0),
    [compiled, setCompiled] = useState<Compiled | null>(null),
    [name, setName] = useState("api-success"),
    [target, setTarget] = useState("99.9"),
    [days, setDays] = useState("28"),
    [refresh, setRefresh] = useState(0),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    const c = new AbortController();
    setError("");
    Promise.all([
      api<{ connections: { id: string; kind: string }[] }>(
        "/api/connections",
        scope,
        { signal: c.signal },
      ),
      api<TopologyCatalog>(
        `/api/topology/resources?kind=${bindingKind}`,
        scope,
        { signal: c.signal },
      ),
    ])
      .then(([c, b]) => {
        setConnections(c.connections);
        setSource((old) =>
          c.connections.some((i) => i.id === old)
            ? old
            : (c.connections[0]?.id ?? ""),
        );
        setItems(b.items);
        setCursor(b.nextCursor ?? "");
        setID((old) =>
          b.items.some((i) => i.id === old) ? old : (b.items[0]?.id ?? ""),
        );
      })
      .catch((e) => {
        if (!c.signal.aborted && !scope.signal.aborted) setError(message(e));
      });
    return () => c.abort();
  }, [scope, refresh]);
  useEffect(() => {
    setBinding(null);
    setHealth(null);
    setCompiled(null);
    if (!id) return;
    const c = new AbortController();
    api<{ resource: { manifest: BindingManifest; revision: number } }>(
      `/api/bindings/${id}`,
      scope,
      { signal: c.signal },
    )
      .then((r) => {
        setBinding(r.resource.manifest);
        setRevision(r.resource.revision);
      })
      .catch((e) => {
        if (!c.signal.aborted && !scope.signal.aborted) setError(message(e));
      });
    return () => c.abort();
  }, [scope, id, refresh]);
  async function lookup(inspect: boolean) {
    setBusy(true);
    setError("");
    setDiscovery(null);
    try {
      if (inspect) {
        const filter = parse(labels) || {};
        if (
          typeof filter !== "object" ||
          Array.isArray(filter) ||
          Object.values(filter).some((v) => typeof v !== "string")
        )
          throw new Error(
            'Use a YAML mapping of string label values, e.g. environment: "production"',
          );
        setDiscovery(
          await api<Discovery>(
            `/api/connections/${source}/telemetry?${new URLSearchParams({ metric, labels: JSON.stringify(filter) })}`,
            scope,
          ),
        );
      } else {
        const r = await api<{ names: string[]; truncated: boolean }>(
          `/api/connections/${source}/metrics?${new URLSearchParams({ prefix: metric })}`,
          scope,
        );
        setNames(r.names);
        setTruncated(r.truncated);
      }
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function checkHealth() {
    setBusy(true);
    setError("");
    try {
      setHealth(
        await api(
          `/api/bindings/${id}/health?expectedRevision=${revision}`,
          scope,
        ),
      );
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function compile() {
    setBusy(true);
    setError("");
    setCompiled(null);
    try {
      setCompiled(
        await api<Compiled>(`/api/bindings/${id}/compile`, scope, {
          method: "POST",
          body: JSON.stringify({
            expectedRevision: revision,
            name,
            displayName: name,
            target: Number((Number(target) / 100).toPrecision(12)),
            windowSeconds: Number(days) * 86400,
            policy: {
              shortWindowSeconds: 300,
              longWindowSeconds: 3600,
              burnRate: 14.4,
              forSeconds: 120,
              minCoverage: 1,
            },
          }),
        }),
      );
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function more() {
    setBusy(true);
    try {
      const r = await api<TopologyCatalog>(
        `/api/topology/resources?kind=${bindingKind}&cursor=${encodeURIComponent(cursor)}`,
        scope,
      );
      setItems((old) => [
        ...old,
        ...r.items.filter((n) => !old.some((i) => i.id === n.id)),
      ]);
      setCursor(r.nextCursor ?? "");
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const starter = bindingStarter(
    source || "primary",
    discovery?.metric ?? metric,
  );
  if (discovery) {
    try {
      starter.spec.labels = parse(labels) || {};
    } catch {
      /* Invalid YAML is reported by inspection. */
    }
    if (discovery.metadata[0]?.type === "histogram") {
      starter.spec.measurement = "latency";
      starter.spec.thresholdSeconds = 0.3;
      delete starter.spec.successLabels;
    }
  }
  return (
    <div className="space-y-4">
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      <Card className="min-w-0 gap-0 py-0">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-4">
          <h2 className="text-sm font-semibold">Discover provider telemetry</h2>
          <MeasurementEditor
            scope={scope}
            initial={starter}
            onSaved={(name) => {
              setID(name);
              setRefresh((v) => v + 1);
            }}
          />
        </div>
        <CardContent className="space-y-4 p-5">
          {connections.length ? (
            <>
              <div className="grid items-end gap-4 md:grid-cols-[1fr_2fr]">
                <div>
                  <Label htmlFor="discovery-source">Connection</Label>
                  <Select
                    value={source}
                    onValueChange={(v) => {
                      setSource(v);
                      setDiscovery(null);
                      setNames([]);
                      setTruncated(false);
                    }}
                  >
                    <SelectTrigger
                      id="discovery-source"
                      className="mt-2 w-full"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {connections.map((c) => (
                        <SelectItem key={c.id} value={c.id}>
                          {c.id} · {c.kind}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <Label htmlFor="discovery-metric">
                    Metric name or prefix
                  </Label>
                  <Input
                    id="discovery-metric"
                    className="mt-2"
                    value={metric}
                    onChange={(e) => {
                      setMetric(e.target.value);
                      setDiscovery(null);
                      setNames([]);
                      setTruncated(false);
                    }}
                  />
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  disabled={busy || !source || !metric}
                  onClick={() => lookup(false)}
                >
                  Find names
                </Button>
                <Button
                  disabled={busy || !source || !metric}
                  onClick={() => lookup(true)}
                >
                  {busy ? "Reading…" : "Inspect metric"}
                </Button>
              </div>
              {names.length ? (
                <div className="flex max-h-48 flex-wrap gap-2 overflow-auto">
                  {names.map((n) => (
                    <Button
                      key={n}
                      size="sm"
                      variant="outline"
                      className="max-w-full truncate"
                      onClick={() => {
                        setMetric(n);
                        setNames([]);
                        setDiscovery(null);
                        setTruncated(false);
                      }}
                    >
                      {n}
                    </Button>
                  ))}
                </div>
              ) : null}
              {truncated ? (
                <p className="text-xs text-amber-800">
                  Name lookup truncated at 100. Refine the prefix.
                </p>
              ) : null}
              <div>
                <Label htmlFor="discovery-labels">
                  Population labels (YAML equality filters)
                </Label>
                <Textarea
                  id="discovery-labels"
                  className="mt-2 min-h-20 font-mono text-xs"
                  placeholder={'environment: "production"\nservice: "api"'}
                  value={labels}
                  onChange={(e) => {
                    setLabels(e.target.value);
                    setDiscovery(null);
                  }}
                />
              </div>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">
              Configure a Prometheus or Mimir connection to discover telemetry.
              This workspace currently uses manual ingestion.
            </p>
          )}
          {discovery ? (
            <div className="space-y-3 border-t pt-4">
              <h3 className="text-sm font-medium">
                {discovery.metric} · {discovery.series.length} series
              </h3>
              {discovery.metadata.map((m, i) => (
                <p key={i} className="text-sm">
                  {m.type} · {m.unit || "unit not reported"}
                  <span className="ml-2 text-muted-foreground">{m.help}</span>
                </p>
              ))}
              {!discovery.metadata.length ? (
                <p className="text-sm text-amber-800">
                  Metadata is unavailable. Compilation requires a verified
                  metric type.
                </p>
              ) : null}
              <details>
                <summary className="cursor-pointer text-sm">
                  Series labels and histogram boundaries
                </summary>
                <pre className="code-block mt-2 max-h-64 overflow-auto">
                  {stringify(discovery.series)}
                </pre>
              </details>
              {discovery.warnings.map((w) => (
                <p className="text-xs text-muted-foreground" key={w}>
                  {w}
                </p>
              ))}
              <p className="break-all font-mono text-xs text-muted-foreground">
                Evidence SHA-256 {discovery.fingerprint}
              </p>
            </div>
          ) : null}
        </CardContent>
        <div className="border-t px-5 py-3 text-xs text-muted-foreground">
          Read-only lookups use the selected workspace connection. Maximum 100
          series over the last 15 minutes.
        </div>
      </Card>
      <Card className="gap-0 py-0">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b px-5 py-4">
          <h2 className="text-sm font-semibold">
            Compile an SLO from a measurement
          </h2>
          {cursor ? (
            <Button size="sm" variant="outline" disabled={busy} onClick={more}>
              Load more measurements
            </Button>
          ) : null}
        </div>
        <CardContent className="space-y-4 p-5">
          <div>
            <Label htmlFor="measurement-binding">Measurement definition</Label>
            <Select value={id} onValueChange={setID}>
              <SelectTrigger id="measurement-binding" className="mt-2 w-full">
                <SelectValue placeholder="Select a saved measurement" />
              </SelectTrigger>
              <SelectContent>
                {items.map((i) => (
                  <SelectItem key={i.id} value={i.id}>
                    {i.displayName} · revision {i.revision}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {binding ? (
            <>
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border bg-slate-50 p-3">
                <div>
                  <p className="text-sm font-medium">
                    {binding.spec.service} · {binding.spec.measurement}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {binding.spec.source} · {binding.spec.metric} ·{" "}
                    {binding.spec.stepSeconds}s evaluation ·{" "}
                    {binding.spec.scrapeIntervalSeconds ?? "unspecified"}s
                    scrape
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {binding.spec.observationPoint}
                  </p>
                </div>
                <MeasurementEditor
                  scope={scope}
                  initial={binding}
                  existing
                  label="Edit measurement"
                  onSaved={() => setRefresh((v) => v + 1)}
                />
              </div>
              <div className="space-y-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={checkHealth}
                >
                  Check measurement contract
                </Button>
                {health ? (
                  <div role="status" className="space-y-1 text-sm">
                    <p className="font-medium">
                      Contract status: {health.state}
                    </p>
                    <p className="text-muted-foreground">{health.message}</p>
                    <p className="text-xs text-muted-foreground">
                      Checked {new Date(health.checkedAt).toLocaleString()}
                    </p>
                  </div>
                ) : null}
              </div>
              <div className="grid items-end gap-4 sm:grid-cols-3">
                <div>
                  <Label htmlFor="compiled-name">SLO identifier</Label>
                  <Input
                    id="compiled-name"
                    className="mt-2"
                    value={name}
                    onChange={(e) => {
                      setName(e.target.value);
                      setCompiled(null);
                    }}
                  />
                </div>
                <div>
                  <Label htmlFor="compiled-target">Target (%)</Label>
                  <Input
                    id="compiled-target"
                    className="mt-2"
                    type="number"
                    step="any"
                    value={target}
                    onChange={(e) => {
                      setTarget(e.target.value);
                      setCompiled(null);
                    }}
                  />
                </div>
                <div>
                  <Label htmlFor="compiled-window">Rolling window (days)</Label>
                  <Input
                    id="compiled-window"
                    className="mt-2"
                    type="number"
                    min="1"
                    max="31"
                    value={days}
                    onChange={(e) => {
                      setDays(e.target.value);
                      setCompiled(null);
                    }}
                  />
                </div>
              </div>
              <Button disabled={busy} onClick={compile}>
                Verify telemetry & compile preview
              </Button>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">
              Save a measurement definition to compile availability or classic
              histogram latency measurements.
            </p>
          )}
          {compiled ? (
            <div className="space-y-3 border-t pt-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-sm font-medium">Compiled SLO preview</h3>
                <TopologyEditor
                  scope={scope}
                  kind="ServiceLevelObjective"
                  initial={compiled.yaml}
                  label="Review & apply SLO YAML"
                  onApplied={() => setCompiled(null)}
                />
              </div>
              <pre className="code-block max-h-96 overflow-auto">
                {compiled.yaml}
              </pre>
              <p className="break-all font-mono text-xs text-muted-foreground">
                {compiled.compilerVersion} · SHA-256 {compiled.digest}
              </p>
              {compiled.warnings.map((w) => (
                <p key={w} className="text-xs text-muted-foreground">
                  {w}
                </p>
              ))}
            </div>
          ) : null}
        </CardContent>
        <div className="border-t px-5 py-3 text-xs text-muted-foreground">
          Compilation returns reviewable YAML with immutable binding provenance.
          Review burn-rate windows and rule destinations through the standard
          SLO flows.
        </div>
      </Card>
    </div>
  );
}
