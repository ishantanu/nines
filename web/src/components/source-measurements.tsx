import { useEffect, useRef, useState } from "react";
import {
  api,
  canEdit,
  message,
  type RequestScope,
  type SLO,
  type Objective,
} from "@/lib/api";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ObjectiveCreate } from "@/components/objective-create";

type Source = { id: string; kind: string };
type Metrics = {
  tenant: string;
  source: string;
  names: string[];
  truncated: boolean;
};
type Discovery = {
  tenant: string;
  source: string;
  metric: string;
  series: Record<string, string>[];
  truncated: boolean;
};
export function SourceMeasurements({
  scope,
  slo,
  onCollected,
  onConnections,
  collection,
}: {
  scope: RequestScope;
  slo: SLO;
  collection?: Objective["collection"];
  onCollected: () => void;
  onConnections: () => void;
}) {
  const source = slo.source || "primary";
  const [connections, setConnections] = useState<Source[]>([]);
  const [prefix, setPrefix] = useState("");
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [discovery, setDiscovery] = useState<Discovery | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const actionController = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    setConnections([]);
    setMetrics(null);
    setDiscovery(null);
    setNotice("");
    setError("");
    setPrefix("");
    setBusy(false);
    api<{ connections: Source[] }>("/api/connections", scope, {
      signal: controller.signal,
    })
      .then((r) => {
        if (!controller.signal.aborted) setConnections(r.connections);
      })
      .catch((e) => {
        if (!controller.signal.aborted && !scope.signal.aborted)
          setError(message(e));
      });
    return () => {
      controller.abort();
      actionController.current?.abort();
    };
  }, [scope, slo.id, slo.revision, source]);
  const selected = connections.find((c) => c.id === source);
  const configured = selected !== undefined && selected.kind !== "manual";
  const queries = !!(slo.providerWindow || (slo.goodQuery && slo.totalQuery));
  async function run(kind: "collect" | "metrics" | "inspect", metric = "") {
    if (busy) return;
    const controller = new AbortController();
    actionController.current = controller;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      if (kind === "collect") {
        const result = await api<{ collected: number }>(
          `/api/slos/${encodeURIComponent(slo.id)}/collect`,
          scope,
          {
            method: "POST",
            body: JSON.stringify({ recent: true }),
            signal: controller.signal,
          },
        );
        if (controller.signal.aborted) return;
        setNotice(
          result.collected > 0
            ? `Read ${result.collected} measurements from ${source}. SLI values and charts are refreshing.`
            : `The source responded but the saved measurement queries returned no matching samples. Browse its metrics and review the metric name, labels and scrape interval. No SLI value is available.`,
        );
        onCollected();
      } else if (kind === "metrics") {
        setMetrics(null);
        setDiscovery(null);
        const result = await api<Metrics>(
          `/api/connections/${encodeURIComponent(source)}/metrics?${new URLSearchParams({ prefix })}`,
          scope,
          { signal: controller.signal },
        );
        if (controller.signal.aborted) return;
        if (result.tenant !== scope.tenant || result.source !== source)
          throw Error("Metric source scope mismatch");
        setMetrics(result);
      } else {
        setDiscovery(null);
        const result = await api<Discovery>(
          `/api/connections/${encodeURIComponent(source)}/telemetry?${new URLSearchParams({ metric })}`,
          scope,
          { signal: controller.signal },
        );
        if (controller.signal.aborted) return;
        if (
          result.tenant !== scope.tenant ||
          result.source !== source ||
          result.metric !== metric
        )
          throw Error("Metric evidence scope mismatch");
        setDiscovery(result);
      }
    } catch (e) {
      if (!controller.signal.aborted && !scope.signal.aborted)
        setError(message(e));
    } finally {
      if (!controller.signal.aborted && !scope.signal.aborted) setBusy(false);
    }
  }
  return (
    <Card className="gap-0 py-0">
      <div className="panel-header">
        <h2 className="text-sm font-semibold">Source measurements</h2>
        <Button variant="outline" size="sm" onClick={onConnections}>
          Data sources
        </Button>
      </div>
      <CardContent className="space-y-4 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1">
            <p className="text-sm">
              Source <span className="font-medium">{source}</span>
              {selected ? ` · ${selected.kind}` : ""}
            </p>
            <p className="text-xs text-muted-foreground">
              Reads use this SLO’s saved queries and the server-configured
              source credentials. Automatic collection runs every 15 seconds.
            </p>
          </div>
          <Button
            size="sm"
            disabled={busy || !configured || !queries || !canEdit(scope)}
            onClick={() => run("collect")}
          >
            {busy ? "Reading…" : "Read latest measurements"}
          </Button>
        </div>
        {collection &&
        collection.sloRevision === (slo.revision ?? 1) &&
        collection.state === "succeeded" &&
        collection.points === 0 ? (
          <p role="status" className="text-sm">
            The latest source read returned no matching samples. Check whether
            the saved metric names and labels exist in this source using the
            metric browser below.
          </p>
        ) : null}
        {collection?.state === "failed" ? (
          <p role="status" className="text-sm text-destructive">
            The latest source read failed. Check the data source connection,
            query permissions and saved measurement.
          </p>
        ) : null}
        {!configured ? (
          <p className="text-sm text-muted-foreground">
            Configure a Prometheus or Mimir data source for this SLO to read
            metrics.
          </p>
        ) : !queries ? (
          <p className="text-sm text-muted-foreground">
            Add a structured measurement or good/total queries to this SLO
            before collection.
          </p>
        ) : null}
        {!canEdit(scope) ? (
          <p className="text-xs text-muted-foreground">
            Editors and owners can collect samples. You can browse source
            metrics with read access.
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {notice ? (
          <p role="status" className="text-sm">
            {notice}
          </p>
        ) : null}
        <details>
          <summary className="cursor-pointer text-sm font-medium">
            Saved measurement queries
          </summary>
          <dl className="mt-3 space-y-3 text-xs">
            <div>
              <dt className="font-medium">
                {slo.providerWindow
                  ? "Good-event selector"
                  : "Good-event query"}
              </dt>
              <dd className="mt-1 break-all font-mono">
                {slo.providerWindow?.goodSelector ||
                  slo.goodQuery ||
                  "Not configured"}
              </dd>
            </div>
            <div>
              <dt className="font-medium">
                {slo.providerWindow
                  ? "Total-event selector"
                  : "Total-event query"}
              </dt>
              <dd className="mt-1 break-all font-mono">
                {slo.providerWindow?.totalSelector ||
                  slo.totalQuery ||
                  "Not configured"}
              </dd>
            </div>
          </dl>
        </details>
        <details>
          <summary className="cursor-pointer text-sm font-medium">
            Browse metrics in this source
          </summary>
          <div className="mt-3 space-y-3">
            <Label htmlFor="source-metric-prefix">
              Metric prefix (optional)
            </Label>
            <div className="flex gap-2">
              <Input
                id="source-metric-prefix"
                maxLength={100}
                placeholder="Filter metric names"
                value={prefix}
                onChange={(e) => setPrefix(e.target.value)}
              />
              <Button
                variant="outline"
                disabled={
                  busy ||
                  !configured ||
                  (prefix !== "" && !/^[a-zA-Z_:][a-zA-Z0-9_:]*$/.test(prefix))
                }
                onClick={() => run("metrics")}
              >
                Browse metrics
              </Button>
            </div>
            {metrics ? (
              <div>
                <p className="mb-2 text-xs text-muted-foreground">
                  {metrics.names.length} metric{" "}
                  {metrics.names.length === 1 ? "name" : "names"}
                  {metrics.truncated
                    ? " · More available; narrow the prefix"
                    : ""}
                  . Select one to inspect its observed labels.
                </p>
                {metrics.names.length ? (
                  <div className="max-h-48 overflow-auto rounded-md border">
                    {metrics.names.map((name) => (
                      <button
                        disabled={busy}
                        key={name}
                        className="block w-full break-all border-b px-3 py-2 text-left font-mono text-xs last:border-0 hover:bg-muted"
                        onClick={() => run("inspect", name)}
                      >
                        {name}
                      </button>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm">
                    No metrics match this prefix in the selected source. Try a
                    different prefix or check its scrape/ingestion
                    configuration.
                  </p>
                )}
              </div>
            ) : null}
            {discovery ? (
              <div className="space-y-2">
                <p className="text-xs font-medium">
                  {discovery.metric} · {discovery.series.length} observed series
                  {discovery.truncated ? " (truncated)" : ""}
                </p>
                <pre className="max-h-48 overflow-auto rounded-md border p-3 text-xs">
                  {JSON.stringify(discovery.series.slice(0, 10), null, 2)}
                </pre>
                <p className="text-xs text-muted-foreground">
                  Series presence is index evidence, not an SLI calculation.
                  Review the measurement before saving; browsing never rewrites
                  the SLO.
                </p>
                {canEdit(scope) ? (
                  <ObjectiveCreate
                    scope={scope}
                    resource={slo}
                    onCreated={onCollected}
                  />
                ) : null}
              </div>
            ) : null}
          </div>
        </details>
      </CardContent>
    </Card>
  );
}
