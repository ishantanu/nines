import { DependencyEvidence } from "./dependency-evidence";
import { SLIValue } from "@/components/sli-value";
import { useEffect, useId, useState } from "react";
import { stringifyDefinition as stringify } from "@/lib/definition-yaml";
import { RefreshCw } from "lucide-react";
import { api, date, message, type RequestScope } from "@/lib/api";
import {
  layoutGraph,
  mapKind,
  mapStarter,
  type DependencyView,
  type TopologyCatalog,
  type TopologyItem,
} from "@/lib/topology";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Status } from "./status";
import { TopologyEditor } from "./topology-editor";
export function DependencyMaps({
  scope,
  onSLO,
  onJourney,
}: {
  scope: RequestScope;
  onSLO: (id: string) => void;
  onJourney: (id: string) => void;
}) {
  const [items, setItems] = useState<TopologyItem[]>([]),
    [cursor, setCursor] = useState(""),
    [id, setID] = useState(""),
    [view, setView] = useState<DependencyView | null>(null),
    [manifest, setManifest] = useState(""),
    [selected, setSelected] = useState(""),
    [error, setError] = useState(""),
    [refresh, setRefresh] = useState(0),
    [viewRefresh, setViewRefresh] = useState(0),
    [busy, setBusy] = useState(false),
    [observed, setObserved] = useState(true),
    [declared, setDeclared] = useState(true);
  const marker = useId().replaceAll(":", "");
  useEffect(() => {
    const c = new AbortController();
    setError("");
    api<TopologyCatalog>(`/api/topology/resources?kind=${mapKind}`, scope, {
      signal: c.signal,
    })
      .then((v) => {
        if (c.signal.aborted || scope.signal.aborted) return;
        setItems(v.items);
        setCursor(v.nextCursor ?? "");
        setID((old) =>
          v.items.some((i) => i.id === old) ? old : (v.items[0]?.id ?? ""),
        );
      })
      .catch((e) => {
        if (!c.signal.aborted && !scope.signal.aborted) setError(message(e));
      });
    return () => c.abort();
  }, [scope, refresh]);
  useEffect(() => {
    setView(null);
    setManifest("");
    setSelected("");
  }, [scope, id]);
  useEffect(() => {
    const timer = setInterval(() => setViewRefresh((v) => v + 1), 15000);
    return () => clearInterval(timer);
  }, [scope]);
  useEffect(() => {
    if (!id) return;
    const c = new AbortController();
    setBusy(true);
    setError("");
    Promise.all([
      api<DependencyView>(`/api/dependency-maps/${id}/view`, scope, {
        signal: c.signal,
      }),
      api<{ resource: { manifest: unknown } }>(
        `/api/dependency-maps/${id}`,
        scope,
        { signal: c.signal },
      ),
    ])
      .then(([v, r]) => {
        if (c.signal.aborted || scope.signal.aborted) return;
        if (v.tenant !== scope.tenant)
          throw new Error("Dependency map returned an unexpected tenant.");
        setView(v);
        setManifest(stringify(r.resource.manifest));
        setSelected((old) =>
          v.nodes.some((n) => n.name === old) ? old : (v.nodes[0]?.name ?? ""),
        );
      })
      .catch((e) => {
        if (!c.signal.aborted && !scope.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setBusy(false);
      });
    return () => c.abort();
  }, [scope, id, refresh, viewRefresh]);
  async function more() {
    setBusy(true);
    try {
      const v = await api<TopologyCatalog>(
        `/api/topology/resources?kind=${mapKind}&cursor=${encodeURIComponent(cursor)}`,
        scope,
      );
      setItems((old) => [
        ...old,
        ...v.items.filter((n) => !old.some((o) => o.id === n.id)),
      ]);
      setCursor(v.nextCursor ?? "");
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const edges =
    view?.edges.filter((e) =>
      e.provenance === "observed" ? observed : declared,
    ) ?? [];
  const nodes = view?.nodes ?? [];
  const layout = layoutGraph(nodes, edges);
  const node = nodes.find((n) => n.name === selected);
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-56">
          <Label htmlFor="dependency-map">Dependency map</Label>
          <Select value={id} onValueChange={setID}>
            <SelectTrigger id="dependency-map" className="mt-2 w-full">
              <SelectValue placeholder="Select a map" />
            </SelectTrigger>
            <SelectContent>
              {items.map((i) => (
                <SelectItem key={i.id} value={i.id}>
                  {i.displayName}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-wrap gap-2">
          {cursor ? (
            <Button size="sm" variant="outline" disabled={busy} onClick={more}>
              Load more maps
            </Button>
          ) : null}
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => setRefresh((v) => v + 1)}
          >
            <RefreshCw className="size-4" />
            Refresh map
          </Button>
          {manifest ? (
            <TopologyEditor
              scope={scope}
              kind={mapKind}
              initial={manifest}
              label="Edit map"
              onApplied={(nextID) => {
                setID(nextID);
                setRefresh((v) => v + 1);
              }}
            />
          ) : null}
          <TopologyEditor
            scope={scope}
            kind={mapKind}
            initial={stringify({
              ...mapStarter,
              spec: {
                displayName: mapStarter.spec.displayName,
                services: [],
                dependencies: [],
              },
            })}
            label="Define dependency map"
            onApplied={(nextID) => {
              setID(nextID);
              setRefresh((v) => v + 1);
            }}
          />
        </div>
      </div>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {busy && !view ? (
        <p role="status" className="empty-state">
          Loading dependency map…
        </p>
      ) : null}
      {!id && !busy ? (
        <Card className="gap-0 py-0">
          <CardContent className="empty-state">
            Define services, link their SLOs and declare caller-to-dependency
            edges in the visual editor, import Kubernetes resources or edit
            YAML. Add an observed source when Tempo service-graph metrics are
            available.
          </CardContent>
        </Card>
      ) : null}
      {view ? (
        <>
          <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(280px,1fr)]">
            <Card className="min-w-0 gap-0 py-0">
              <div className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-4">
                <h2 className="text-sm font-semibold">Service dependencies</h2>
                <div className="flex gap-2">
                  <Button
                    variant={declared ? "secondary" : "outline"}
                    size="sm"
                    aria-pressed={declared}
                    onClick={() => setDeclared((v) => !v)}
                  >
                    Declared
                  </Button>
                  <Button
                    variant={observed ? "secondary" : "outline"}
                    size="sm"
                    aria-pressed={observed}
                    onClick={() => setObserved((v) => !v)}
                  >
                    Observed
                  </Button>
                </div>
              </div>
              <CardContent className="p-0">
                <div
                  className="h-[480px] max-w-full overflow-auto bg-muted/30 focus-visible:outline-2 focus-visible:outline-ring"
                  role="region"
                  tabIndex={0}
                  aria-label="Service dependency graph"
                >
                  <div
                    className="relative"
                    style={{ width: layout.width, height: layout.height }}
                  >
                    <svg
                      className="absolute inset-0"
                      width={layout.width}
                      height={layout.height}
                      aria-hidden="true"
                    >
                      <defs>
                        <marker
                          id={marker}
                          markerWidth="8"
                          markerHeight="8"
                          refX="7"
                          refY="4"
                          orient="auto"
                        >
                          <path
                            d="M0,0 L8,4 L0,8"
                            fill="var(--muted-foreground)"
                          />
                        </marker>
                      </defs>
                      {edges.map((e, i) => {
                        const a = layout.positions.get(e.caller),
                          b = layout.positions.get(e.callee);
                        if (!a || !b) return null;
                        const x1 = a.x + 192,
                          y1 = a.y + 40,
                          x2 = b.x - 5,
                          y2 = b.y + 40;
                        return (
                          <path
                            key={`${e.caller}-${e.callee}-${i}`}
                            d={
                              e.caller === e.callee
                                ? `M${x1},${y1} C${x1 + 70},${y1 - 90} ${a.x - 70},${y1 - 90} ${a.x},${y1}`
                                : `M${x1},${y1} C${x1 + 45},${y1} ${x2 - 45},${y2} ${x2},${y2}`
                            }
                            fill="none"
                            stroke={
                              e.provenance === "observed"
                                ? "var(--chart-long)"
                                : "var(--muted-foreground)"
                            }
                            strokeWidth="1.7"
                            strokeDasharray={
                              e.provenance === "observed" ? "5 4" : undefined
                            }
                            markerEnd={`url(#${marker})`}
                          />
                        );
                      })}
                    </svg>
                    {nodes.map((n) => {
                      const p = layout.positions.get(n.name)!;
                      return (
                        <button
                          type="button"
                          key={n.name}
                          onClick={() => setSelected(n.name)}
                          aria-pressed={selected === n.name}
                          aria-label={`${n.displayName}, ${n.health}, ${n.objectives.length} SLOs`}
                          className={`absolute flex h-20 w-48 flex-col justify-center gap-2 rounded-md border bg-background px-3 text-left shadow-sm focus-visible:outline-2 focus-visible:outline-primary ${selected === n.name ? "border-primary ring-1 ring-primary" : "border-border"}`}
                          style={{ left: p.x, top: p.y }}
                        >
                          <span
                            className="truncate text-sm font-medium"
                            title={n.name}
                          >
                            {n.displayName}
                          </span>
                          <span className="flex items-center justify-between gap-1">
                            <Status value={n.health} />
                            <span className="text-xs text-muted-foreground">
                              {n.objectives.length} SLOs
                            </span>
                          </span>
                        </button>
                      );
                    })}
                  </div>
                </div>
              </CardContent>
              <div className="flex flex-wrap justify-between gap-2 border-t px-5 py-3 text-xs text-muted-foreground">
                <span>
                  {nodes.length} services · {edges.length} edges · revision{" "}
                  {view.revision}
                </span>
                <span>Solid: declared · dashed: observed</span>
              </div>
            </Card>
            <Card className="min-w-0 gap-0 py-0">
              <div className="border-b px-5 py-4">
                <h2 className="text-sm font-semibold">Service details</h2>
              </div>
              <CardContent className="h-[480px] space-y-5 overflow-auto p-5">
                {node ? (
                  <>
                    <div>
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <h3 className="break-all text-base font-semibold">
                          {node.displayName}
                        </h3>
                        <Status value={node.health} />
                      </div>
                      <p className="mt-2 break-all font-mono text-xs text-muted-foreground">
                        {node.name}
                      </p>
                      <p className="mt-2 text-xs text-muted-foreground">
                        {node.declared
                          ? "Declared service"
                          : "Observed service · no declared SLO links"}
                      </p>
                    </div>
                    <section>
                      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                        Linked SLOs
                      </h3>
                      {node.objectives.length ? (
                        node.objectives.map((o) => (
                          <div key={o.id} className="space-y-2 border-b py-3">
                            <Button
                              variant="link"
                              className="h-auto max-w-full justify-start whitespace-normal p-0 text-left"
                              onClick={() => onSLO(o.id)}
                            >
                              {o.displayName}
                            </Button>
                            <div className="flex flex-wrap items-center justify-between gap-2">
                              <SLIValue status={o} target={o.target} />
                            </div>
                            <p className="text-xs text-muted-foreground">
                              {o.dataStatus} · {o.freshness} · revision{" "}
                              {o.revision}
                            </p>
                          </div>
                        ))
                      ) : (
                        <p className="text-sm text-muted-foreground">
                          No SLO is linked to this service.
                        </p>
                      )}
                    </section>
                    {node.journeys.some((j) => j.steps?.length) ? (
                      <section>
                        <h3 className="mb-2 text-sm font-semibold">
                          Linked journey steps
                        </h3>
                        {node.journeys
                          .filter((j) => j.steps?.length)
                          .map((j) => (
                            <div key={j.id} className="mb-3">
                              <Button
                                variant="link"
                                className="h-auto whitespace-normal px-0 text-left"
                                onClick={() => onJourney(j.id)}
                              >
                                {j.displayName}
                              </Button>
                              <ul className="mt-1 list-inside list-disc space-y-1 text-sm text-muted-foreground">
                                {j.steps?.map((step) => (
                                  <li key={step.id}>{step.name}</li>
                                ))}
                              </ul>
                            </div>
                          ))}
                      </section>
                    ) : null}
                    <section>
                      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                        Potential journey impact
                      </h3>
                      {node.potentialImpact.length ? (
                        node.potentialImpact.map((j) => (
                          <Button
                            key={j.id}
                            variant="link"
                            className="block h-auto w-full whitespace-normal px-0 text-left"
                            onClick={() => onJourney(j.id)}
                          >
                            {j.displayName}
                          </Button>
                        ))
                      ) : (
                        <p className="text-sm text-muted-foreground">
                          No CUJs are linked to this service or its upstream
                          callers.
                        </p>
                      )}
                      {node.impactTruncated ? (
                        <p className="mt-2 text-xs text-muted-foreground">
                          Showing a bounded set of possible journey links. Use
                          resource search for all links.
                        </p>
                      ) : null}
                    </section>
                  </>
                ) : (
                  <p>Select a service to inspect its objectives.</p>
                )}
              </CardContent>
              <div className="border-t px-5 py-3 text-xs text-muted-foreground">
                Health reflects linked SLOs. Possible impact follows declared
                and fresh observed links.
              </div>
            </Card>
          </div>
          <Card className="gap-0 py-0">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b px-5 py-4">
              <h2 className="text-sm font-semibold">Dependency evidence</h2>
              <Badge variant="outline">
                {view.observationState.replaceAll("_", " ")}
              </Badge>
            </div>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Caller</TableHead>
                    <TableHead>Dependency</TableHead>
                    <TableHead>Evidence</TableHead>
                    <TableHead>Traffic / requirement</TableHead>
                    <TableHead>Telemetry evidence</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {edges.map((e, i) => (
                    <TableRow key={i}>
                      <TableCell>
                        <button
                          className="text-left font-medium hover:underline"
                          onClick={() => setSelected(e.caller)}
                        >
                          {e.caller}
                        </button>
                      </TableCell>
                      <TableCell>
                        <button
                          className="text-left hover:underline"
                          onClick={() => setSelected(e.callee)}
                        >
                          {e.callee}
                        </button>
                      </TableCell>
                      <TableCell>
                        <p>
                          {e.origin === "kubernetes"
                            ? "Kubernetes configuration"
                            : e.provenance === "observed"
                              ? "Observed counter"
                              : "Declared"}
                        </p>
                        <p className="text-sm text-muted-foreground">
                          {e.alignment === "matched"
                            ? "Observed match"
                            : e.alignment === "observed_only"
                              ? "Outside declared map"
                              : "No observed match"}
                        </p>
                        {e.connectionType ? ` · ${e.connectionType}` : ""}
                      </TableCell>
                      <TableCell>
                        {e.provenance === "observed"
                          ? `${e.requests?.toLocaleString(undefined, { maximumFractionDigits: 1 })} estimated requests`
                          : e.critical
                            ? "Critical dependency"
                            : "Supporting dependency"}
                        {e.description ? (
                          <p className="mt-1 text-xs text-muted-foreground">
                            {e.description}
                          </p>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <DependencyEvidence edge={e} />
                      </TableCell>
                    </TableRow>
                  ))}
                  {!edges.length ? (
                    <TableRow>
                      <TableCell colSpan={5} className="empty-state">
                        No edges match the selected evidence filters.
                      </TableCell>
                    </TableRow>
                  ) : null}
                </TableBody>
              </Table>
            </CardContent>
            <div className="space-y-1 border-t px-5 py-3 text-xs text-muted-foreground">
              {view.source ? (
                <p className="text-sm">
                  {view.matchedDeclaredEdges ?? 0} of {view.declaredEdges ?? 0}{" "}
                  declared links have observed counter evidence in this bounded
                  window. Instrumentation and trace-sampling coverage are
                  unknown.
                </p>
              ) : null}
              {view.source ? (
                <p>
                  Source {view.source} · {date(view.from ?? "")} to{" "}
                  {date(view.to ?? "")}
                </p>
              ) : null}
              <p>Read {date(view.observedAt)}</p>
              {view.truncated ? (
                <p className="font-medium text-[var(--status-at-risk)]">
                  Observed graph truncated. Narrow the configured population
                  labels.
                </p>
              ) : null}
              {view.warnings.map((w) => (
                <p key={w}>{w}</p>
              ))}
            </div>
          </Card>
        </>
      ) : null}
    </div>
  );
}
