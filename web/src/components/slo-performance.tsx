import { ReplayLabel } from "@/components/replay-label";
import { useEffect, useRef, useState } from "react";
import { api, message, percent, type RequestScope, type SLO } from "@/lib/api";
import { duration, type HistoryPoint, type SLOHistory } from "@/lib/history";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Card, CardHeader, CardContent } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
function localInput(date: Date) {
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 19);
}
interface Line {
  name: string;
  color: string;
  get: (p: HistoryPoint) => number | null;
  dashed?: boolean;
}
export const sliTimelineLines: Line[] = [
  {
    name: "Interval SLI",
    color: "#7c93b5",
    get: (p) => (p.intervalCoverage === 1 ? p.intervalReliability : null),
  },
  {
    name: "Rolling SLI",
    color: "#0f766e",
    get: (p) => (p.coverage === 1 ? p.reliability : null),
  },
  {
    name: "Rolling SLI (partial window)",
    color: "#a16207",
    dashed: true,
    get: (p) => (p.coverage > 0 && p.coverage < 1 ? p.reliability : null),
  },
];
export function Timeline({
  title,
  history,
  lines,
  threshold,
  thresholdLabel,
  index,
  onIndex,
  percentages = false,
}: {
  title: string;
  history: SLOHistory;
  lines: Line[];
  threshold: number;
  thresholdLabel: string;
  index: number;
  onIndex: (i: number) => void;
  percentages?: boolean;
}) {
  const host = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(700);
  useEffect(() => {
    if (!host.current) return;
    const observer = new ResizeObserver(([entry]) =>
      setWidth(Math.max(280, entry.contentRect.width)),
    );
    observer.observe(host.current);
    return () => observer.disconnect();
  }, []);
  const values = history.points
    .flatMap((p) => lines.map((l) => l.get(p)))
    .filter((v): v is number => v !== null && Number.isFinite(v));
  let low = Math.min(threshold, ...values),
    high = Math.max(threshold, ...values);
  const pad = Math.max((high - low) * 0.12, percentages ? 0.0001 : 0.25);
  low -= pad;
  high += pad;
  if (title === "Burn rate") low = Math.max(0, low);
  if (percentages && title === "SLI and SLO target") {
    high = Math.min(1, high);
    low = Math.max(0, low);
  }
  const left = 62,
    right = width - 12,
    top = 14,
    bottom = 170;
  const x = (i: number) =>
    left + ((right - left) * i) / Math.max(1, history.points.length - 1);
  const y = (v: number) => bottom - ((v - low) / (high - low)) * (bottom - top);
  const format = (v: number) =>
    percentages
      ? `${(v * 100).toFixed(title === "SLI and SLO target" ? 2 : 0)}%`
      : `${v.toFixed(1)}×`;
  function path(line: Line) {
    let pen = false;
    return history.points
      .map((p, i) => {
        const v = line.get(p);
        if (v === null) {
          pen = false;
          return "";
        }
        const segment = `${pen ? "L" : "M"}${x(i).toFixed(2)},${y(v).toFixed(2)}`;
        pen = true;
        return segment;
      })
      .join(" ");
  }
  return (
    <Card className="chart-panel gap-0 py-0">
      <CardHeader>
        <h3 className="text-sm font-semibold">
          <ReplayLabel>{title}</ReplayLabel>
        </h3>
        <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
          {lines.map((l) => (
            <span key={l.name} className="inline-flex items-center gap-1.5">
              <span
                className="inline-block w-4"
                style={{
                  borderTop: `2px ${l.dashed ? "dashed" : "solid"} ${l.color}`,
                }}
              />
              <ReplayLabel>{l.name}</ReplayLabel>
            </span>
          ))}
          <span className="inline-flex items-center gap-1.5">
            <span className="w-4 border-t border-dashed border-slate-500" />
            {thresholdLabel}
          </span>
        </div>
      </CardHeader>
      <CardContent>
        <div ref={host}>
          <svg
            viewBox={`0 0 ${width} 202`}
            className="block h-[202px] w-full outline-none focus-visible:ring-2 focus-visible:ring-ring"
            role="img"
            aria-label={`${title}. Use left and right arrow keys to inspect time points.`}
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
                e.preventDefault();
                onIndex(
                  Math.max(
                    0,
                    Math.min(
                      history.points.length - 1,
                      index + (e.key === "ArrowRight" ? 1 : -1),
                    ),
                  ),
                );
              }
            }}
            onPointerMove={(e) => {
              const bounds = e.currentTarget.getBoundingClientRect();
              onIndex(
                Math.max(
                  0,
                  Math.min(
                    history.points.length - 1,
                    Math.round(
                      ((((e.clientX - bounds.left) * width) / bounds.width -
                        left) /
                        (right - left)) *
                        (history.points.length - 1),
                    ),
                  ),
                ),
              );
            }}
          >
            {[0, 1, 2, 3].map((i) => {
              const value = low + ((high - low) * i) / 3;
              return (
                <g key={i}>
                  <line
                    x1={left}
                    x2={right}
                    y1={y(value)}
                    y2={y(value)}
                    stroke="#e2e8f0"
                  />
                  <text
                    x={left - 8}
                    y={y(value) + 4}
                    textAnchor="end"
                    fill="#596579"
                    fontSize={11}
                  >
                    {format(value)}
                  </text>
                </g>
              );
            })}
            <line
              x1={left}
              x2={right}
              y1={y(threshold)}
              y2={y(threshold)}
              stroke="#64748b"
              strokeDasharray="5 4"
            />
            {title === "SLI and SLO target"
              ? history.alertEvents
                  .filter(
                    (event) =>
                      event.kind === "firing" || event.kind === "resolved",
                  )
                  .map((event) => {
                    const position =
                      left +
                      ((Date.parse(event.time) - Date.parse(history.from)) /
                        Math.max(
                          1,
                          Date.parse(history.to) - Date.parse(history.from),
                        )) *
                        (right - left);
                    return (
                      <g key={event.id}>
                        <title>
                          {event.kind} · {new Date(event.time).toISOString()}
                          {event.reason ? ` · ${event.reason}` : ""}
                        </title>
                        <line
                          x1={position}
                          x2={position}
                          y1={top}
                          y2={bottom}
                          stroke={
                            event.kind === "firing" ? "#be123c" : "#15803d"
                          }
                          strokeDasharray="2 4"
                          opacity={0.55}
                        />
                        <circle
                          cx={position}
                          cy={top + 3}
                          r={3}
                          fill={event.kind === "firing" ? "#be123c" : "#15803d"}
                        />
                      </g>
                    );
                  })
              : null}
            {lines.map((line) => (
              <path
                key={line.name}
                d={path(line)}
                fill="none"
                stroke={line.color}
                strokeWidth={1.8}
                strokeDasharray={line.dashed ? "4 3" : undefined}
              />
            ))}
            {title === "SLI and SLO target"
              ? history.points.map((p, i) =>
                  p.belowTarget && p.intervalReliability !== null ? (
                    <circle
                      key={p.at}
                      cx={x(i)}
                      cy={y(p.intervalReliability)}
                      r={2.3}
                      fill="#be123c"
                    />
                  ) : null,
                )
              : null}
            {history.points.length ? (
              <line
                x1={x(index)}
                x2={x(index)}
                y1={top}
                y2={bottom}
                stroke="#94a3b8"
                strokeDasharray="3 3"
              />
            ) : null}
            <text x={left} y={196} fill="#596579" fontSize={11}>
              {new Date(history.from)
                .toISOString()
                .slice(5, 16)
                .replace("T", " ")}{" "}
              UTC
            </text>
            <text
              x={right}
              y={196}
              fill="#596579"
              fontSize={11}
              textAnchor="end"
            >
              {new Date(history.to)
                .toISOString()
                .slice(5, 16)
                .replace("T", " ")}{" "}
              UTC
            </text>
          </svg>
        </div>
      </CardContent>
    </Card>
  );
}
export function SLOPerformance({
  slo,
  at,
  scope,
  refreshKey,
}: {
  slo: SLO;
  at: string;
  scope: RequestScope;
  refreshKey: number;
}) {
  const [range, setRange] = useState(Math.min(21600, 2 * slo.windowSeconds));
  const [custom, setCustom] = useState(false);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [resolution, setResolution] = useState("0");
  const [policyName, setPolicyName] = useState(slo.policies?.[0]?.name ?? "");
  const [history, setHistory] = useState<SLOHistory | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [index, setIndex] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    let query: URLSearchParams;
    try {
      // Resolve rolling ranges in one server read, avoiding a stale status
      // cutoff racing retention or the next completed collection interval.
      query = custom
        ? new URLSearchParams({
            from: new Date(from).toISOString(),
            to: new Date(to).toISOString(),
          })
        : new URLSearchParams({
            rangeSeconds: String(Math.min(range, 2 * slo.windowSeconds)),
          });
      if (policyName) query.set("policy", policyName);
      if (resolution !== "0") query.set("resolutionSeconds", resolution);
    } catch {
      setError("Choose a valid start and end time.");
      setLoading(false);
      return () => c.abort();
    }
    api<SLOHistory>(`/api/slos/${slo.id}/history?${query}`, scope, {
      signal: c.signal,
    })
      .then((data) => {
        if (!c.signal.aborted) {
          setHistory(data);
          setIndex(data.points.length - 1);
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [
    slo.id,
    slo.windowSeconds,
    slo.revision,
    at,
    scope,
    range,
    custom,
    from,
    to,
    resolution,
    policyName,
    refreshKey,
  ]);
  const point = history?.points[index];
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <Label htmlFor="chart-range">
            <ReplayLabel>Time range</ReplayLabel>
          </Label>
          <select
            id="chart-range"
            className="mt-1 block h-9 rounded-md border bg-white px-3 text-sm"
            value={custom ? "custom" : String(range)}
            onChange={(e) => {
              if (e.target.value === "custom") {
                setFrom(localInput(new Date(new Date(at).getTime() - 3600000)));
                setTo(localInput(new Date(at)));
                setCustom(true);
              } else {
                setCustom(false);
                setRange(Number(e.target.value));
              }
            }}
          >
            {[
              Math.min(3600, 2 * slo.windowSeconds),
              Math.min(21600, 2 * slo.windowSeconds),
              Math.min(86400, 2 * slo.windowSeconds),
              Math.min(604800, 2 * slo.windowSeconds),
            ]
              .filter((v, i, a) => a.indexOf(v) === i)
              .map((s) => (
                <option key={s} value={s}>
                  Last {duration(s)}
                </option>
              ))}
            <option value="custom">Custom</option>
          </select>
        </div>
        {slo.policies?.length ? (
          <div>
            <Label htmlFor="chart-policy">
              <ReplayLabel>Alert policy</ReplayLabel>
            </Label>
            <select
              id="chart-policy"
              className="mt-1 block h-9 rounded-md border bg-background px-3 text-sm"
              value={policyName}
              onChange={(e) => setPolicyName(e.target.value)}
            >
              {slo.policies.map((p) => (
                <option key={p.name} value={p.name}>
                  {p.name} ({p.severity})
                </option>
              ))}
            </select>
          </div>
        ) : null}
        <div>
          <Label htmlFor="chart-resolution">
            <ReplayLabel>Resolution</ReplayLabel>
          </Label>
          <select
            id="chart-resolution"
            className="mt-1 block h-9 rounded-md border bg-white px-3 text-sm"
            value={resolution}
            onChange={(e) => setResolution(e.target.value)}
          >
            <option value="0">Auto</option>
            {[slo.stepSeconds, 300, 900, 3600]
              .filter(
                (n, i, a) =>
                  n >= slo.stepSeconds &&
                  n % slo.stepSeconds === 0 &&
                  a.indexOf(n) === i,
              )
              .map((n) => (
                <option value={n} key={n}>
                  {duration(n)}
                </option>
              ))}
          </select>
        </div>
        {loading ? (
          <span role="status" className="pb-2 text-xs text-muted-foreground">
            Loading history…
          </span>
        ) : null}
      </div>
      {custom ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <Label htmlFor="chart-from">
              <ReplayLabel>From (local time)</ReplayLabel>
            </Label>
            <Input
              id="chart-from"
              type="datetime-local"
              step={slo.stepSeconds}
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="chart-to">
              <ReplayLabel>To (local time)</ReplayLabel>
            </Label>
            <Input
              id="chart-to"
              type="datetime-local"
              step={slo.stepSeconds}
              value={to}
              onChange={(e) => setTo(e.target.value)}
            />
          </div>
        </div>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {history && !error ? (
        <>
          <p className="text-xs text-muted-foreground">
            Source {history.source} · SLO target {percent(history.target, 3)} ·
            rolling window {duration(history.windowSeconds)} · revision{" "}
            {history.configRevision} · {duration(history.resolutionSeconds)}{" "}
            chart buckets.{" "}
            {history.measurementMode === "provider_window"
              ? "Independent provider windows · coverage from scrape density."
              : "Coverage from interval presence."}
          </p>
          <div className="panel-grid">
            <Timeline
              title="SLI and SLO target"
              history={history}
              lines={sliTimelineLines}
              threshold={history.target}
              thresholdLabel={`Target ${percent(history.target, 3)}`}
              percentages
              index={index}
              onIndex={setIndex}
            />
            <Timeline
              title="Error budget remaining"
              history={history}
              lines={[
                {
                  name: "Remaining budget",
                  color: "#0f766e",
                  get: (p) =>
                    p.coverage === 1 ? p.budgetRemainingFraction : null,
                },
              ]}
              threshold={0}
              thresholdLabel="Budget exhausted"
              percentages
              index={index}
              onIndex={setIndex}
            />
            {history.policy ? (
              <Timeline
                title="Burn rate"
                history={history}
                lines={[
                  {
                    name: `Short ${duration(history.policy.shortWindowSeconds)}`,
                    color: "#d97706",
                    get: (p) =>
                      p.shortCoverage >= (history.policy?.minCoverage ?? 1)
                        ? p.shortBurnRate
                        : null,
                  },
                  {
                    name: `Long ${duration(history.policy.longWindowSeconds)}`,
                    color: "#4f46e5",
                    get: (p) =>
                      p.longCoverage >= (history.policy?.minCoverage ?? 1)
                        ? p.longBurnRate
                        : null,
                  },
                ]}
                threshold={history.policy.burnRate}
                thresholdLabel={`Threshold ${history.policy.burnRate}×`}
                index={index}
                onIndex={setIndex}
              />
            ) : (
              <Card className="chart-panel gap-0 py-0">
                <CardHeader>
                  <h3 className="section-label">
                    <ReplayLabel>Burn rate</ReplayLabel>
                  </h3>
                </CardHeader>
                <CardContent className="empty-state">
                  No burn-rate alert policy configured.
                </CardContent>
              </Card>
            )}
            {point ? (
              <Card className="chart-panel gap-0 py-0">
                <CardHeader>
                  <h3 className="section-label">
                    <ReplayLabel>Selected interval</ReplayLabel>
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    Move across a chart or use the arrow keys.
                  </p>
                </CardHeader>
                <CardContent className="text-xs">
                  <p className="mb-2 font-medium">
                    {new Date(point.at).toLocaleString()} · rolling coverage{" "}
                    {percent(point.coverage, 1)}
                  </p>
                  <div className="grid grid-cols-2 gap-x-4 gap-y-5 py-3">
                    <span>
                      Interval SLI: {percent(point.intervalReliability, 3)}
                    </span>
                    <span>
                      Rolling SLI: {percent(point.reliability, 3)}
                      {point.coverage < 1 ? " (partial)" : ""}
                    </span>
                    <span>
                      Budget: {percent(point.budgetRemainingFraction, 1)}
                    </span>
                    <span>
                      Short burn:{" "}
                      {point.shortBurnRate == null
                        ? "N/A"
                        : `${point.shortBurnRate.toFixed(2)}×`}
                      {history.policy &&
                      point.shortCoverage < history.policy.minCoverage
                        ? " (partial)"
                        : ""}
                    </span>
                    <span>
                      Long burn:{" "}
                      {point.longBurnRate == null
                        ? "N/A"
                        : `${point.longBurnRate.toFixed(2)}×`}
                      {history.policy &&
                      point.longCoverage < history.policy.minCoverage
                        ? " (partial)"
                        : ""}
                    </span>
                  </div>
                </CardContent>
              </Card>
            ) : null}
          </div>
          <div className="panel-grid">
            <Card className="gap-0 py-0">
              <div className="panel-header">
                <h3 className="section-label">
                  Below-target periods and burn breaches (
                  {history.violations.length})
                </h3>
              </div>
              <CardContent className="h-64 overflow-auto p-5">
                <p className="mb-3 text-xs text-muted-foreground">
                  Red dots mark observed interval buckets below target. SLO
                  violations require full rolling coverage. Burn breaches
                  require both windows and minimum coverage; they are not alert
                  episodes. Period boundaries are approximate to chart
                  resolution. History uses the current definition; missing
                  measurements and no traffic create chart gaps. Dashed amber
                  rolling SLI shows available measurements from partial windows;
                  it does not establish full-window compliance.
                </p>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>
                        <ReplayLabel>Condition</ReplayLabel>
                      </TableHead>
                      <TableHead>
                        <ReplayLabel>From</ReplayLabel>
                      </TableHead>
                      <TableHead>
                        <ReplayLabel>To</ReplayLabel>
                      </TableHead>
                      <TableHead>
                        <ReplayLabel>Buckets</ReplayLabel>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {history.violations.map((v, i) => (
                      <TableRow key={i}>
                        <TableCell>
                          {
                            (
                              {
                                interval_target: "Interval below target",
                                slo_target: "Rolling SLO violation",
                                burn_threshold: "Both burn windows breached",
                              } as Record<string, string>
                            )[v.kind]
                          }
                        </TableCell>
                        <TableCell>
                          {new Date(v.from).toLocaleString()}
                        </TableCell>
                        <TableCell>{new Date(v.to).toLocaleString()}</TableCell>
                        <TableCell>{v.buckets}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
            <Card className="gap-0 py-0">
              <div className="panel-header">
                <h3 className="section-label">
                  Recorded alert transitions ({history.alertEvents.length})
                </h3>
              </div>
              <CardContent className="h-64 overflow-auto p-5">
                <p className="mb-3 text-xs text-muted-foreground">
                  From the tenant's latest 200 alert transitions. This is a
                  bounded event view, not a complete incident archive.
                </p>
                {history.alertEvents.map((e) => (
                  <p className="border-b py-2 text-xs" key={e.id}>
                    {new Date(e.time).toLocaleString()} ·{" "}
                    {e.policy ? `${e.policy} (${e.severity}) · ` : ""}
                    {e.kind}
                    {e.reason ? ` · ${e.reason}` : ""} · short{" "}
                    {e.shortBurn.toFixed(2)}× / long {e.longBurn.toFixed(2)}×
                  </p>
                ))}
              </CardContent>
            </Card>
          </div>
        </>
      ) : null}
    </div>
  );
}
