import { useEffect, useState } from "react";
import { api, type Objective, type RequestScope } from "@/lib/api";
import type { SLOHistory } from "@/lib/history";
import { Sparkline } from "@/components/ui/notchset/sparkline";
import { BarChart } from "@/components/ui/notchset/bar-chart";
import { DonutChart } from "@/components/ui/notchset/donut-chart";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { Card, CardContent, CardHeader } from "@/components/ui/card";

export function SLOTrend({
  objective,
  scope,
  rangeSeconds = 900,
}: {
  objective: Objective;
  scope?: RequestScope;
  rangeSeconds?: number;
}) {
  const { slo, status } = objective;
  const key = `${scope?.tenant}/${slo.id}/${slo.revision ?? 1}/${status.at}/${slo.stepSeconds}/${slo.windowSeconds}/${rangeSeconds}`;
  const [result, setResult] = useState<{
    key: string;
    history?: SLOHistory;
    error?: boolean;
  } | null>(null);
  useEffect(() => {
    if (!scope) return;
    const controller = new AbortController();
    const resolution = Math.max(
      slo.stepSeconds,
      Math.ceil(rangeSeconds / 60 / slo.stepSeconds) * slo.stepSeconds,
    );
    const query = new URLSearchParams({
      rangeSeconds: String(Math.max(rangeSeconds, slo.stepSeconds)),
      resolutionSeconds: String(resolution),
    });
    api<SLOHistory>(
      `/api/slos/${encodeURIComponent(slo.id)}/history?${query}`,
      scope,
      { signal: controller.signal },
    )
      .then((history) => {
        if (controller.signal.aborted) return;
        if (
          history.sloId !== slo.id ||
          history.configRevision !== (slo.revision ?? 1)
        )
          throw new Error("History revision mismatch");
        setResult({ key, history });
      })
      .catch(() => {
        if (!controller.signal.aborted) setResult({ key, error: true });
      });
    return () => controller.abort();
  }, [scope, key, slo.id, slo.stepSeconds, slo.revision, rangeSeconds]);
  const current = result?.key === key ? result : undefined;
  if (!scope) return null;
  return (
    <span
      className="flex min-w-0 flex-col gap-1"
      aria-label={`Recent SLI trend for ${slo.name}`}
    >
      <span className="text-xs text-muted-foreground">
        Last{" "}
        {slo.stepSeconds > rangeSeconds
          ? "interval"
          : rangeSeconds < 3600
            ? `${rangeSeconds / 60}m`
            : `${rangeSeconds / 3600}h`}
      </span>
      {current?.history ? (
        <Sparkline
          label={`${slo.name}, recent SLI trend`}
          target={current.history.target ?? slo.target}
          from={current.history.from}
          to={current.history.to}
          points={current.history.points.map((p) => ({
            at: p.at,
            value: p.coverage > 0 ? p.reliability : null,
            partial: p.coverage < 1,
          }))}
        />
      ) : (
        <span className="text-xs text-muted-foreground">
          {current?.error ? "Trend unavailable" : "Loading trend…"}
        </span>
      )}
    </span>
  );
}

export function overviewMeasurements(objectives: readonly Objective[]) {
  const active = objectives.filter((o) => !o.slo.retirement);
  const coverage = [
    { label: "Complete windows", value: 0 },
    { label: "Partial windows", value: 0 },
    { label: "No measurements", value: 0 },
  ];
  const services = new Map<string, number>();
  for (const { slo, status } of active) {
    const covered = Number.isFinite(status.coverage) && status.coverage > 0;
    coverage[covered ? (status.coverage >= 1 ? 0 : 1) : 2].value++;
    if (
      status.coverage === 1 &&
      status.burnRate !== null &&
      Number.isFinite(status.burnRate) &&
      status.burnRate >= 0
    )
      services.set(
        slo.service,
        Math.max(services.get(slo.service) ?? 0, status.burnRate),
      );
  }
  const all = [...services].sort(
    (a, b) => b[1] - a[1] || a[0].localeCompare(b[0]),
  );
  return {
    coverage,
    services: all.slice(0, 6).map(([label, value]) => ({ label, value })),
    omitted: all.length > 6,
    total: active.length,
  };
}
export function OverviewCharts({
  objectives,
}: {
  objectives: readonly Objective[];
}) {
  const { coverage, services, omitted, total } =
    overviewMeasurements(objectives);
  const [active, setActive] = useState<number | null>(null);
  if (!total) return null;
  const selected = active !== null ? services[active] : null;
  return (
    <section
      className="overview-measurement-charts grid min-w-0 gap-4 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]"
      aria-label="Measurement summary"
    >
      <Card className="chart-panel min-w-0 gap-0 py-0">
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Heading level={2}>Budget burn by service</Heading>
            {selected ? (
              <Heading
                level={2}
                accessibilityLevel={3}
                className="tabular-nums"
              >
                {selected.value.toFixed(2)}×
              </Heading>
            ) : null}
          </div>
          <Text as="p" size="sm" color="secondary">
            Highest complete-window burn per service. No average across SLOs.
          </Text>
        </CardHeader>
        <CardContent className="flex min-h-0 flex-col">
          {services.length ? (
            <BarChart
              data={services}
              yMax={Math.max(1, ...services.map((s) => s.value)) * 1.15}
              format={(v) => `${v.toFixed(2)}×`}
              active={active}
              onActiveChange={setActive}
              aria-label="Highest complete-window budget burn by service"
            />
          ) : (
            <p className="py-8 text-sm text-muted-foreground">
              No complete-window burn measurements available.
            </p>
          )}
          <div className="mt-3 flex flex-wrap items-baseline justify-between gap-x-5 gap-y-2 border-t pt-3 text-xs">
            <p className="text-muted-foreground">
              1× sustainable budget rate
              {omitted ? " · Six highest services" : ""}
            </p>
            <p className="break-words tabular-nums" aria-live="polite">
              {selected
                ? `${selected.label}: ${selected.value.toFixed(2)}×`
                : "Hover or focus a service to inspect"}
            </p>
          </div>
        </CardContent>
      </Card>
      <Card className="chart-panel min-w-0 gap-0 py-0">
        <CardHeader>
          <Heading level={2}>Window coverage</Heading>
          <Text as="p" size="sm" color="secondary">
            Latest reported measurement coverage across {total} objectives. Not
            an availability score.
          </Text>
        </CardHeader>
        <CardContent className="min-w-0 p-3">
          <DonutChart
            data={coverage}
            digits={0}
            label="Objectives"
            caption="objectives"
            aria-label="Objectives by measurement-window coverage"
          />
        </CardContent>
      </Card>
    </section>
  );
}
