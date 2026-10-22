import { useEffect, useRef, useState } from "react";
import { api, message, percent, type RequestScope } from "@/lib/api";
import { durationSeconds, type SLOHistory } from "@/lib/history";
import {
  backtestTime,
  type IntentDocument,
  type IntentBacktest,
} from "@/lib/intent";
import { Timeline } from "./slo-performance";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
export function IntentBacktestPanel({
  scope,
  intent,
  binding,
  step,
}: {
  scope: RequestScope;
  intent: IntentDocument;
  binding: { name: string; revision: number };
  step: number;
}) {
  const resolution = Math.ceil(Math.max(step, 300) / step) * step;
  const cutoff =
    Math.floor(Date.now() / (step * 1000)) * step * 1000 - step * 1000;
  const [from, setFrom] = useState(() =>
    new Date(
      cutoff - Math.floor(21600 / resolution) * resolution * 1000,
    ).toISOString(),
  );
  const [to, setTo] = useState(() => new Date(cutoff).toISOString());
  const [res, setResolution] = useState(`${resolution}s`);
  const [result, setResult] = useState<IntentBacktest | null>(null),
    [index, setIndex] = useState(0);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  useEffect(() => () => request.current?.abort(), []);
  async function run() {
    setError("");
    setResult(null);
    setBusy(true);
    const c = new AbortController();
    request.current = c;
    try {
      const r = await api<IntentBacktest>("/api/intents/backtest", scope, {
        method: "POST",
        signal: c.signal,
        body: JSON.stringify({
          plan: {
            intent,
            binding: { name: binding.name, expectedRevision: binding.revision },
          },
          from: backtestTime(from),
          to: backtestTime(to),
          resolutionSeconds: durationSeconds(res),
        }),
      });
      setResult(r);
      setIndex(0);
    } catch (e) {
      if (!c.signal.aborted && !scope.signal.aborted) setError(message(e));
    } finally {
      if (!c.signal.aborted) setBusy(false);
    }
  }
  const history: SLOHistory | null = result
    ? {
        source: intent.spec.source,
        sloId: intent.metadata.name,
        configRevision: 0,
        target: intent.spec.target,
        windowSeconds: intent.spec.windowSeconds,
        stepSeconds: step,
        resolutionSeconds: result.resolutionSeconds,
        from: result.from,
        to: result.to,
        policy: intent.spec.policy,
        violations: [],
        alertEvents: [],
        alertEventsBounded: false,
        points: result.points.map((p) => ({
          ...p,
          intervalReliability: null,
          budgetRemainingFraction: null,
          burnRate: null,
          shortBurnRate: p.shortBurn,
          longBurnRate: p.longBurn,
          intervalCoverage: 0,
          belowTarget: false,
          burnThresholdBreached: p.burnThresholdExceeded,
        })),
      }
    : null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Historical validation</CardTitle>
        <p className="text-sm text-muted-foreground">
          Sample full rolling windows before applying. Limited to 72 hours and
          240 cutoffs; no samples, alerts or definitions are written.
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        <fieldset disabled={busy} className="grid gap-4 lg:grid-cols-3">
          <div className="grid gap-2">
            <Label htmlFor="backtest-from">From (RFC 3339)</Label>
            <Input
              id="backtest-from"
              value={from}
              onChange={(e) => {
                setFrom(e.target.value);
                setResult(null);
              }}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="backtest-to">To (RFC 3339)</Label>
            <Input
              id="backtest-to"
              value={to}
              onChange={(e) => {
                setTo(e.target.value);
                setResult(null);
              }}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="backtest-resolution">Sampling resolution</Label>
            <Input
              id="backtest-resolution"
              value={res}
              onChange={(e) => {
                setResolution(e.target.value);
                setResult(null);
              }}
            />
          </div>
        </fieldset>
        <Button variant="outline" onClick={run} disabled={busy}>
          {busy ? "Querying history…" : "Run historical validation"}
        </Button>
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {result && history ? (
          <>
            <dl className="grid grid-cols-2 gap-4 border-y py-4 text-sm lg:grid-cols-5">
              {[
                ["Complete cutoffs", result.completeCutoffs],
                ["Unknown cutoffs", result.unknownCutoffs],
                ["No-traffic cutoffs", result.noTrafficCutoffs],
                ["Target violations", result.targetViolationCutoffs],
                ["Burn crossings", result.burnThresholdCutoffs],
              ].map(([label, value]) => (
                <div key={label}>
                  <dt className="text-muted-foreground">{label}</dt>
                  <dd className="mt-1 text-xl font-semibold tabular-nums">
                    {value}
                  </dd>
                </div>
              ))}
            </dl>
            <Timeline
              title="SLI and SLO target"
              history={history}
              lines={[
                {
                  name: "Full-window SLI",
                  color: "#0f766e",
                  get: (p) => (p.coverage === 1 ? p.reliability : null),
                },
              ]}
              threshold={intent.spec.target}
              thresholdLabel="SLO target"
              index={index}
              onIndex={setIndex}
              percentages
            />
            {intent.spec.policy ? (
              <Timeline
                title="Burn rate"
                history={history}
                lines={[
                  {
                    name: "Short window",
                    color: "#b45309",
                    get: (p) =>
                      p.shortCoverage >= intent.spec.policy!.minCoverage
                        ? p.shortBurnRate
                        : null,
                  },
                  {
                    name: "Long window",
                    color: "#be123c",
                    get: (p) =>
                      p.longCoverage >= intent.spec.policy!.minCoverage
                        ? p.longBurnRate
                        : null,
                  },
                ]}
                threshold={intent.spec.policy.burnRate}
                thresholdLabel="Burn threshold"
                index={index}
                onIndex={setIndex}
              />
            ) : null}
            <p className="text-xs text-muted-foreground">
              Incomplete observations appear as graph gaps. Counts refer to
              sampled cutoffs, not incidents or unique events.
            </p>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Selected cutoff</TableHead>
                  <TableHead>SLI</TableHead>
                  <TableHead>Coverage</TableHead>
                  <TableHead>Data status</TableHead>
                  <TableHead>Short / long burn</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {result.points[index] ? (
                  <TableRow>
                    <TableCell>{result.points[index].at}</TableCell>
                    <TableCell>
                      {percent(result.points[index].reliability, 3)}
                    </TableCell>
                    <TableCell>
                      {percent(result.points[index].coverage)}
                    </TableCell>
                    <TableCell>{result.points[index].dataStatus}</TableCell>
                    <TableCell>
                      {result.points[index].shortBurn?.toFixed(2) ?? "N/A"} /{" "}
                      {result.points[index].longBurn?.toFixed(2) ?? "N/A"}
                    </TableCell>
                  </TableRow>
                ) : null}
              </TableBody>
            </Table>
            {result.warnings.map((w) => (
              <p key={w} className="text-xs text-muted-foreground">
                {w}
              </p>
            ))}
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}
