import { useEffect, useId, useRef, useState } from "react";
import { api, message, percent, type RequestScope, type SLO } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ProviderIdentity } from "./provider-identity";
import { Timeline } from "./timeline";
import type { SLOHistory } from "@/lib/history";

interface Results {
  provider: string;
  source: string;
  revision: number;
  nativeID: string;
  state: string;
  from: string;
  to: string;
  checkedAt: string;
  target: number;
  attainment: number | null;
  budgetRemaining: number | null;
  budgetBasis: string;
  points: { time: string; attainment: number | null }[];
  warnings: string[];
}
export function NativeSLOResults({
  slo,
  scope,
  provider,
}: {
  slo: SLO;
  scope: RequestScope;
  provider: string;
}) {
  const id = useId();
  const initialEnd = Math.floor((Date.now() - 120000) / 300000) * 300000;
  const [from, setFrom] = useState(
    new Date(initialEnd - 86400000).toISOString().slice(0, 16),
  );
  const [to, setTo] = useState(new Date(initialEnd).toISOString().slice(0, 16));
  const [data, setData] = useState<Results | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [index, setIndex] = useState(0);
  const request = useRef<AbortController | null>(null);
  useEffect(() => {
    request.current?.abort();
    setData(null);
    setError("");
    setLoading(false);
    return () => request.current?.abort();
  }, [scope, slo.id, slo.revision, provider]);
  async function read() {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setError("");
    setData(null);
    const start = Date.parse(from + "Z"),
      end = Date.parse(to + "Z");
    if (
      !Number.isFinite(start) ||
      !Number.isFinite(end) ||
      end <= start ||
      end > Date.now() ||
      end - start > 31 * 86400000
    ) {
      setError("Choose a past UTC range of up to 31 days.");
      return;
    }
    setLoading(true);
    try {
      const q = new URLSearchParams({
        from: new Date(start).toISOString(),
        to: new Date(end).toISOString(),
      });
      const next = await api<Results>(
        `/api/slos/${slo.id}/native-results?${q}`,
        scope,
        { signal: controller.signal },
      );
      if (!controller.signal.aborted) {
        if (next.revision !== slo.revision)
          throw Error(
            "The SLO changed. Refresh this page before reading results.",
          );
        setData(next);
        setIndex(Math.max(0, next.points.length - 1));
      }
    } catch (e) {
      if (!controller.signal.aborted) setError(message(e));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }
  const history: SLOHistory | undefined = data
    ? {
        source: data.source,
        sloId: slo.id,
        configRevision: data.revision,
        target: data.target,
        windowSeconds: slo.windowSeconds,
        stepSeconds: 60,
        resolutionSeconds: 60,
        from: data.from,
        to: data.to,
        points: data.points.map((p) => ({
          at: p.time,
          reliability: null,
          intervalReliability: p.attainment,
          budgetRemainingFraction: null,
          burnRate: null,
          shortBurnRate: null,
          longBurnRate: null,
          shortCoverage: 0,
          longCoverage: 0,
          coverage: 0,
          intervalCoverage: 0,
          belowTarget: false,
          targetViolated: false,
          burnThresholdBreached: false,
        })),
        violations: [],
        alertEvents: [],
        alertEventsBounded: false,
      }
    : undefined;
  return (
    <section
      className="min-w-0 space-y-4 border-t pt-5"
      aria-labelledby={`${id}-heading`}
      aria-busy={loading}
    >
      <div className="space-y-2">
        <h3 id={`${id}-heading`} className="text-base font-semibold">
          Native provider results
        </h3>
        <ProviderIdentity
          kind={provider}
          connection={slo.source || "primary"}
        />
        <p className="max-w-prose text-sm leading-6 text-muted-foreground">
          Read the published current revision on demand. These results are
          separate from Nines measurements and do not verify alert delivery.
          Dates use UTC.
        </p>
      </div>
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          void read();
        }}
      >
        <div className="min-w-0 flex-1 space-y-1">
          <Label htmlFor={`${id}-from`}>From (UTC)</Label>
          <Input
            id={`${id}-from`}
            type="datetime-local"
            required
            value={from}
            onChange={(e) => {
              request.current?.abort();
              setLoading(false);
              setData(null);
              setFrom(e.target.value);
            }}
          />
        </div>
        <div className="min-w-0 flex-1 space-y-1">
          <Label htmlFor={`${id}-to`}>To (UTC)</Label>
          <Input
            id={`${id}-to`}
            type="datetime-local"
            required
            value={to}
            onChange={(e) => {
              request.current?.abort();
              setLoading(false);
              setData(null);
              setTo(e.target.value);
            }}
          />
        </div>
        <Button type="submit" variant="outline" disabled={loading}>
          {loading ? "Reading provider…" : "Read provider results"}
        </Button>
      </form>
      {provider === "newrelic" ? (
        <p className="text-xs leading-5 text-muted-foreground">
          New Relic uses at most 300 buckets. Requested bounds are clipped
          inward to complete intervals. Returned dates show the effective range.
        </p>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {data ? (
        <>
          <p className="break-words text-sm text-muted-foreground">
            Range {data.from} to {data.to} (UTC) · Native SLO {data.nativeID} ·
            Revision {data.revision} · Read{" "}
            {new Date(data.checkedAt).toLocaleString()}
          </p>
          <dl className="grid gap-4 sm:grid-cols-3">
            <div>
              <dt className="text-sm text-muted-foreground">
                Range attainment
              </dt>
              <dd className="mt-1 font-semibold tabular-nums">
                {data.attainment === null
                  ? "No native SLI data"
                  : percent(data.attainment)}
              </dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">SLO target</dt>
              <dd className="mt-1 font-semibold tabular-nums">
                {percent(data.target)}
              </dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">
                Range budget remaining
              </dt>
              <dd className="mt-1 font-semibold tabular-nums">
                {data.budgetRemaining === null
                  ? "Unavailable"
                  : percent(data.budgetRemaining)}
              </dd>
              <p className="mt-1 text-xs text-muted-foreground">
                {data.budgetBasis === "provider_reported"
                  ? "Reported by Datadog"
                  : "Calculated from native counts"}
              </p>
            </div>
          </dl>
          <ul className="max-w-prose list-disc space-y-2 pl-5 text-sm leading-6 text-muted-foreground">
            {data.warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
          {history && data.points.some((p) => p.attainment !== null) ? (
            <Timeline
              title="Native interval attainment"
              history={history}
              lines={[
                {
                  name: "Native interval attainment",
                  color: "var(--chart-sli)",
                  get: (p) => p.intervalReliability,
                },
              ]}
              threshold={data.target}
              thresholdLabel="SLO target (whole window)"
              index={index}
              onIndex={setIndex}
              percentages
            />
          ) : (
            <p role="status" className="text-sm">
              No native measurement events in this range. Try a range with
              traffic after native SLO publication.
            </p>
          )}
        </>
      ) : null}
    </section>
  );
}
