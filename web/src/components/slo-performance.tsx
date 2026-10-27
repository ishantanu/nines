import { ReplayLabel } from "@/components/replay-label";
import { useEffect, useState } from "react";
import { api, message, percent, type RequestScope, type SLO } from "@/lib/api";
import { duration, type SLOHistory } from "@/lib/history";
import { Label } from "@/components/ui/label";
import { ChartTimeRange } from "./chart-time-range";
import { Timeline, sliTimelineLines } from "./timeline";
import { rangeQuery, utcInput, type ChartRange } from "@/lib/chart-range";
export { sliTimelineLines, Timeline } from "./timeline";
import { Card, CardHeader, CardContent } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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
  const [range, setRange] = useState<ChartRange>({
    kind: "relative",
    seconds: Math.min(21600, 2 * slo.windowSeconds),
  });
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
      query = rangeQuery(range, 2 * slo.windowSeconds);
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
    resolution,
    policyName,
    refreshKey,
  ]);
  const point = history?.points[index];
  return (
    <div className="min-w-0 space-y-4" aria-busy={loading}>
      <div className="grid min-w-0 items-end gap-3 sm:flex sm:flex-wrap">
        <ChartTimeRange
          value={range}
          maxSeconds={2 * slo.windowSeconds}
          stepSeconds={slo.stepSeconds}
          observedTo={history?.to ?? at}
          onChange={setRange}
        />
        {slo.policies?.length ? (
          <div className="min-w-0">
            <Label htmlFor="chart-policy">
              <ReplayLabel>Alert policy</ReplayLabel>
            </Label>
            <select
              id="chart-policy"
              className="mt-1 block h-9 w-full max-w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
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
            className="mt-1 block h-9 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
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
          <span role="status" className="pb-2 text-sm text-muted-foreground">
            Loading history…
          </span>
        ) : null}
      </div>
      {error ? (
        <p role="alert" className="text-sm text-[var(--status-degraded)]">
          {error}
        </p>
      ) : null}
      {history && !error ? (
        <>
          <details className="performance-context disclosure-panel">
            <summary>
              Chart context{" "}
              <span className="font-normal text-muted-foreground">
                {history.source} · {duration(history.windowSeconds)} objective
                window
              </span>
            </summary>
            <p className="break-words text-sm text-muted-foreground">
              {utcInput(history.from).replace("T", " ")} to{" "}
              {utcInput(history.to).replace("T", " ")} UTC
              <br />
            </p>
            <dl className="grid grid-cols-2 gap-4 text-sm lg:grid-cols-4">
              {[
                ["Telemetry connection", history.source],
                ["SLO window", duration(history.windowSeconds)],
                ["Chart resolution", duration(history.resolutionSeconds)],
                ["Definition revision", history.configRevision],
              ].map(([label, value]) => (
                <div key={label} className="min-w-0">
                  <dt className="text-muted-foreground">{label}</dt>
                  <dd className="break-words font-medium">{value}</dd>
                </div>
              ))}
            </dl>
          </details>
          <div className="performance-grid performance-focused">
            <Timeline
              title="SLI and SLO target"
              history={history}
              lines={sliTimelineLines}
              threshold={history.target}
              thresholdLabel={`SLO target ${percent(history.target, 3)}`}
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
                  color: "var(--chart-sli)",
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
                    color: "var(--chart-short)",
                    dashed: true,
                    get: (p) =>
                      p.shortCoverage >= (history.policy?.minCoverage ?? 1)
                        ? p.shortBurnRate
                        : null,
                  },
                  {
                    name: `Long ${duration(history.policy.longWindowSeconds)}`,
                    color: "var(--chart-long)",
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
          </div>
          {point ? (
            <details className="disclosure-panel selected-interval">
              <summary>
                <ReplayLabel>Selected interval details</ReplayLabel>{" "}
                <span className="font-normal text-muted-foreground">
                  {utcInput(point.at).replace("T", " ")} UTC
                </span>
              </summary>
              <div className="text-sm">
                <p className="mb-2 font-medium">
                  {utcInput(point.at).replace("T", " ") + " UTC"} · window
                  coverage {percent(point.coverage, 1)}
                </p>
                <div className="grid gap-x-4 gap-y-5 py-3 sm:grid-cols-2">
                  <span>
                    Interval SLI:{" "}
                    {point.intervalReliability == null
                      ? "No data"
                      : percent(point.intervalReliability, 3)}
                  </span>
                  <span>
                    Measured SLI:{" "}
                    {point.reliability == null
                      ? "No data"
                      : percent(point.reliability, 3)}
                    {point.reliability != null &&
                    point.coverage > 0 &&
                    point.coverage < 1
                      ? " (incomplete window)"
                      : ""}
                  </span>
                  <span>
                    Budget:{" "}
                    {point.coverage === 1 &&
                    point.budgetRemainingFraction != null
                      ? percent(point.budgetRemainingFraction, 1)
                      : "Not evaluated"}
                  </span>
                  <span>
                    Short burn:{" "}
                    {point.shortBurnRate == null
                      ? "No data"
                      : `${point.shortBurnRate.toFixed(2)}×`}
                    {point.shortBurnRate != null &&
                    history.policy &&
                    point.shortCoverage < history.policy.minCoverage
                      ? " (insufficient coverage)"
                      : ""}
                  </span>
                  <span>
                    Long burn:{" "}
                    {point.longBurnRate == null
                      ? "No data"
                      : `${point.longBurnRate.toFixed(2)}×`}
                    {point.longBurnRate != null &&
                    history.policy &&
                    point.longCoverage < history.policy.minCoverage
                      ? " (insufficient coverage)"
                      : ""}
                  </span>
                </div>
              </div>
            </details>
          ) : null}
          <details className="disclosure-panel">
            <summary>
              Periods and alert history{" "}
              <span className="font-normal text-muted-foreground">
                {history.violations.length} below-target periods ·{" "}
                {history.alertEvents.length} alert transitions
              </span>
            </summary>
            <div className="panel-grid">
              <Card className="gap-0 py-0">
                <div className="panel-header">
                  <h3 className="section-label">
                    Below-target periods and burn breaches (
                    {history.violations.length})
                  </h3>
                </div>
                <CardContent className="max-h-80 overflow-auto p-5">
                  <p className="mb-3 text-sm text-muted-foreground">
                    Triangles mark complete-window SLI values below target.
                    Dashed SLI segments have incomplete history and do not
                    establish full-window attainment. Burn breaches require both
                    alert windows and minimum coverage. Periods are approximate
                    to the chart resolution; they are separate from recorded
                    alert transitions. Missing measurements and no traffic
                    create gaps.
                  </p>
                  {!history.violations.length ? (
                    <p className="mb-3 text-sm">
                      No below-target periods or burn breaches in this range.
                    </p>
                  ) : null}
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
                            {utcInput(v.from).replace("T", " ") + " UTC"}
                          </TableCell>
                          <TableCell>
                            {utcInput(v.to).replace("T", " ") + " UTC"}
                          </TableCell>
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
                <CardContent className="max-h-80 overflow-auto p-5">
                  <p className="mb-3 text-sm text-muted-foreground">
                    From the tenant's latest 200 alert transitions. This is a
                    bounded event view, not a complete incident archive.
                  </p>
                  {!history.alertEvents.length ? (
                    <p className="text-sm">
                      No recorded alert transitions in this range.
                    </p>
                  ) : null}
                  {history.alertEvents.map((e) => (
                    <p className="border-b py-2 text-sm" key={e.id}>
                      {utcInput(e.time).replace("T", " ") + " UTC"} ·{" "}
                      {e.policy ? `${e.policy} (${e.severity}) · ` : ""}
                      {e.kind}
                      {e.reason ? ` · ${e.reason}` : ""} · short{" "}
                      {e.shortBurn.toFixed(2)}× / long {e.longBurn.toFixed(2)}×
                    </p>
                  ))}
                </CardContent>
              </Card>
            </div>
          </details>
        </>
      ) : null}
    </div>
  );
}
