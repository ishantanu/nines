import { SLOTypeBadges } from "./slo-type-badge";
import type { Workspace, Objective, Journey, RequestScope } from "@/lib/api";
import { percent } from "@/lib/api";
import { duration } from "@/lib/history";
import { AnimatedCounter } from "./ui/notchset/animated-counter";
import { SLOMetricCard } from "./ui/notchset/blocks/metric-card";
import { SLOTrend } from "./overview-charts";
import { SLOProvider } from "./provider-identity";
import { SLOAlertState } from "./sli-value";
import { Button } from "./ui/button";
import { ArrowUpRight, Waypoints } from "lucide-react";

export function SLOCards({
  items,
  compact = false,
  connections,
  journeys,
  scope,
  rangeSeconds,
  onSelect,
  onJourney,
}: {
  items: Objective[];
  compact?: boolean;
  connections?: Workspace["connections"];
  journeys: Journey[];
  scope?: RequestScope;
  rangeSeconds: number;
  onSelect: (id: string) => void;
  onJourney: (id: string) => void;
}) {
  const Heading = compact ? "h3" : "h2";
  return (
    <div className="grid min-w-0 gap-4 sm:grid-cols-2 xl:grid-cols-3">
      {items.map((o) => {
        const measured =
          o.status.reliability !== null &&
          Number.isFinite(o.status.reliability);
        const budget = o.status.budgetRemainingFraction;
        const linked = journeys.filter((j) =>
          j.links.some((l) => l.link.id === o.slo.id),
        );
        return (
          <SLOMetricCard
            key={o.slo.id}
            compact={compact}
            label={
              <>
                <Heading className="break-words text-base font-semibold">
                  <button
                    className="min-h-11 text-left hover:underline focus-visible:outline-2 focus-visible:outline-ring"
                    onClick={() => onSelect(o.slo.id)}
                  >
                    {o.slo.name}
                    {o.slo.retirement ? " (Archived)" : ""}
                  </button>
                </Heading>
                <span className="break-words text-sm text-muted-foreground">
                  {o.slo.project} / {o.slo.service}
                </span>
                <SLOTypeBadges slo={o.slo} />
                <SLOProvider slo={o.slo} connections={connections} />
              </>
            }
            value={
              <span
                className={
                  measured && o.status.reliability! < o.slo.target
                    ? "status-degraded"
                    : undefined
                }
              >
                {measured ? (
                  <AnimatedCounter
                    value={o.status.reliability!}
                    format={(value) => `${percent(value, 3)} SLI`}
                  />
                ) : o.status.dataStatus === "no_traffic" ? (
                  "No traffic"
                ) : (
                  "No SLI data"
                )}
              </span>
            }
            sub={
              <>
                <div>
                  {percent(o.slo.target, 3)} target ·{" "}
                  {duration(o.slo.windowSeconds)} window
                </div>
                <div>
                  {o.status.coverage > 0 && o.status.coverage < 0.001
                    ? "<0.1%"
                    : percent(o.status.coverage, 1)}{" "}
                  coverage
                  {measured
                    ? o.status.coverage < 1
                      ? " · Partial window"
                      : " · Full window"
                    : ""}
                </div>
              </>
            }
            trend={
              scope && !o.slo.retirement ? (
                <SLOTrend
                  objective={o}
                  scope={scope}
                  rangeSeconds={rangeSeconds}
                />
              ) : (
                <p className="text-sm text-muted-foreground">
                  {o.slo.retirement
                    ? "Archived objective"
                    : "Connect to view SLI history"}
                </p>
              )
            }
          >
            <dl className="grid grid-cols-2 gap-3 text-sm">
              <div>
                <dt className="text-muted-foreground">Budget remaining</dt>
                <dd className="mt-1 font-medium tabular-nums">
                  {budget !== null && Number.isFinite(budget) ? (
                    <AnimatedCounter
                      value={budget}
                      format={(value) => percent(value, 1)}
                    />
                  ) : (
                    "No estimate"
                  )}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Budget burn</dt>
                <dd className="mt-1 font-medium tabular-nums">
                  {o.status.burnRate !== null &&
                  Number.isFinite(o.status.burnRate) ? (
                    <AnimatedCounter
                      value={o.status.burnRate}
                      format={(value) => `${value.toFixed(2)}×`}
                    />
                  ) : (
                    "No estimate"
                  )}
                </dd>
              </div>
            </dl>
            {measured && o.status.coverage < 1 && (
              <p className="mt-2 text-xs text-muted-foreground">
                Based on observed data
              </p>
            )}
            <div className="mt-3">
              <SLOAlertState status={o.status} />
            </div>
            {!compact && linked.length > 0 && (
              <section
                className="mt-3"
                aria-label="Journeys that depend on this SLO"
              >
                <p className="mb-1 text-xs text-muted-foreground">
                  Journeys that depend on this SLO
                </p>
                <div className="flex flex-wrap gap-1">
                  {linked.map((j) => (
                    <Button
                      key={j.resource.manifest.metadata.name}
                      variant="outline"
                      size="sm"
                      className="h-auto min-h-11 max-w-full whitespace-normal text-left"
                      title={`Open the critical user journey "${j.resource.manifest.spec.displayName}"`}
                      onClick={() =>
                        onJourney(j.resource.manifest.metadata.name)
                      }
                    >
                      <Waypoints className="size-3.5 shrink-0" aria-hidden />
                      {j.resource.manifest.spec.displayName}
                      <ArrowUpRight className="size-3 shrink-0" aria-hidden />
                    </Button>
                  ))}
                </div>
              </section>
            )}
          </SLOMetricCard>
        );
      })}
    </div>
  );
}
