import { providerLabel } from "@/lib/telemetry-providers";
import { SLOProvider } from "./provider-identity";
import { useState } from "react";
import { ReplayLabel } from "@/components/replay-label";
import {
  Activity,
  ArrowRight,
  Database,
  ShieldCheck,
  ChevronDown,
  CircleCheck,
  CircleHelp,
  TriangleAlert,
  CircleX,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Status } from "@/components/status";
import { SLIValue } from "@/components/sli-value";
import { healthOf, percent, type Data } from "@/lib/api";
export function Home({
  data,
  onNavigate,
  onSLO,
  onJourney,
}: {
  data: Data;
  onNavigate: (
    page: "objectives" | "journeys" | "connections" | "access" | "clients",
  ) => void;
  onSLO: (id: string) => void;
  onJourney: (id: string) => void;
}) {
  const [reviewOnly, setReviewOnly] = useState(false);
  const active = data.objectives.filter((o) => !o.slo.retirement);
  const attention = active
    .filter((o) => healthOf(o.status) !== "healthy")
    .sort(
      (a, b) =>
        (a.status.budgetRemainingFraction ?? Infinity) -
        (b.status.budgetRemainingFraction ?? Infinity),
    );
  const impacted = data.journeys.filter((j) => j.health !== "healthy");
  const unknownJourneys = impacted.filter((j) => j.health === "unknown").length;
  const healthGroups = [
    { key: "degraded", label: "Degraded", Icon: CircleX },
    { key: "at_risk", label: "At risk", Icon: TriangleAlert },
    { key: "unknown", label: "Incomplete evidence", Icon: CircleHelp },
    { key: "healthy", label: "Healthy", Icon: CircleCheck },
  ] as const;
  const groups = healthGroups.map((group) => ({
    ...group,
    count: active.filter((o) => healthOf(o.status) === group.key).length,
  }));
  const rank = (o: (typeof active)[number]) =>
    healthGroups.findIndex((group) => group.key === healthOf(o.status));
  const visible = (reviewOnly ? attention : [...active]).sort(
    (a, b) =>
      rank(a) - rank(b) ||
      (a.status.budgetRemainingFraction ?? Infinity) -
        (b.status.budgetRemainingFraction ?? Infinity),
  );
  return (
    <div className="overview space-y-5">
      <section className="overview-brief" aria-labelledby="overview-focus">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2
              id="overview-focus"
              className="text-xl font-semibold tracking-tight"
            >
              {!active.length
                ? "Start measuring reliability"
                : attention.length
                  ? `${attention.length} ${attention.length === 1 ? "objective needs" : "objectives need"} review`
                  : "No objectives require attention"}
            </h2>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              {active.length
                ? `${active.length} active ${active.length === 1 ? "objective" : "objectives"} across ${new Set(active.map((o) => o.slo.service)).size} ${new Set(active.map((o) => o.slo.service)).size === 1 ? "service" : "services"} and ${data.journeys.length} critical user ${data.journeys.length === 1 ? "journey" : "journeys"}.`
                : "Define an SLO to start monitoring service reliability."}
            </p>
          </div>
          <Button
            size="sm"
            variant={attention.length ? "default" : "outline"}
            onClick={() =>
              attention.length ? setReviewOnly(true) : onNavigate("objectives")
            }
          >
            {attention.length ? "Review objectives" : "Open objectives"}
            <ArrowRight className="size-4" aria-hidden="true" />
          </Button>
        </div>
        {active.length ? (
          <>
            <dl className="overview-health-counts">
              {groups
                .filter((g) => g.count > 0)
                .map(({ key, label, Icon, count }) => (
                  <div key={key}>
                    <dt>
                      <Icon
                        className={`size-4 status-${key.replace("_", "-")}`}
                        aria-hidden="true"
                      />
                      <ReplayLabel>{label}</ReplayLabel>
                    </dt>
                    <dd>{count}</dd>
                  </div>
                ))}
            </dl>
          </>
        ) : null}
      </section>
      <div className="overview-columns">
        <Card className="overview-panel gap-0 py-0">
          <div className="overview-panel-header">
            <div className="overview-panel-heading">
              <div>
                <h2 className="text-sm font-semibold">
                  Service level objectives
                </h2>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onNavigate("objectives")}
              >
                View SLOs
                <ArrowRight className="size-3.5" />
              </Button>
            </div>
            <div
              className="overview-objective-filters"
              role="group"
              aria-label="Overview objective filter"
            >
              <Button
                size="sm"
                variant={!reviewOnly ? "secondary" : "ghost"}
                aria-pressed={!reviewOnly}
                onClick={() => setReviewOnly(false)}
              >
                All ({active.length})
              </Button>
              <Button
                size="sm"
                variant={reviewOnly ? "secondary" : "ghost"}
                aria-pressed={reviewOnly}
                onClick={() => setReviewOnly(true)}
              >
                Needs review ({attention.length})
              </Button>
            </div>
          </div>
          <CardContent className="overview-objective-list p-0">
            {visible.length ? (
              visible.slice(0, 6).map((o) => (
                <button
                  key={o.slo.id}
                  className="overview-resource-row overview-objective-row"
                  onClick={() => onSLO(o.slo.id)}
                >
                  <div>
                    <span className="overview-resource-name text-sm font-medium">
                      {o.slo.name}
                    </span>
                    <div className="mt-1">
                      <SLOProvider
                        slo={o.slo}
                        connections={data.workspace.connections}
                      />
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {o.slo.service} ·{" "}
                      {o.status.budgetRemainingFraction == null
                        ? "Budget estimate unavailable"
                        : `${percent(o.status.budgetRemainingFraction, 1)} budget remaining`}
                    </p>
                  </div>
                  <SLIValue status={o.status} target={o.slo.target} />
                </button>
              ))
            ) : (
              <div className="empty-state space-y-3">
                <p>
                  {active.length
                    ? "No objectives require attention."
                    : "Your active objectives will appear here."}
                </p>
                {reviewOnly ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setReviewOnly(false)}
                  >
                    Show all objectives
                  </Button>
                ) : !active.length ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => onNavigate("objectives")}
                  >
                    Open objectives
                  </Button>
                ) : null}
              </div>
            )}
          </CardContent>
        </Card>
        <Card className="overview-panel gap-0 py-0">
          <div className="overview-panel-header">
            <div className="overview-panel-heading">
              <div>
                <h2 className="text-sm font-semibold">
                  Critical user journeys
                </h2>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onNavigate("journeys")}
              >
                View journeys
                <ArrowRight className="size-3.5" aria-hidden="true" />
              </Button>
            </div>
          </div>
          <CardContent className="overview-journey-list p-0">
            {unknownJourneys ? (
              <p className="border-b px-5 py-3 text-sm text-muted-foreground">
                {unknownJourneys}{" "}
                {unknownJourneys === 1 ? "journey has" : "journeys have"}{" "}
                incomplete evidence. Missing data does not confirm service
                impact.
              </p>
            ) : null}
            {data.journeys.length ? (
              [...data.journeys]
                .sort(
                  (a, b) =>
                    healthGroups.findIndex((g) => g.key === a.health) -
                    healthGroups.findIndex((g) => g.key === b.health),
                )
                .slice(0, 6)
                .map((j) => (
                  <button
                    key={j.resource.manifest.metadata.name}
                    className="overview-resource-row"
                    onClick={() => onJourney(j.resource.manifest.metadata.name)}
                  >
                    <div>
                      <span className="overview-resource-name text-sm font-medium">
                        {j.resource.manifest.spec.displayName}
                      </span>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {j.resource.manifest.spec.owner} · {j.links.length}{" "}
                        linked SLOs
                      </p>
                    </div>
                    {j.health === "unknown" ? (
                      <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
                        <CircleHelp className="size-3.5" aria-hidden="true" />
                        Incomplete evidence
                      </span>
                    ) : (
                      <Status value={j.health} />
                    )}
                  </button>
                ))
            ) : (
              <p className="empty-state">
                {data.journeys.length
                  ? "No affected journeys."
                  : "Link objectives to a critical user journey to track impact."}
              </p>
            )}
          </CardContent>
        </Card>
      </div>
      <details className="workspace-setup rounded-md border bg-card">
        <summary className="flex flex-wrap items-center justify-between gap-3 px-5 py-4 text-sm font-medium">
          <span>Workspace setup</span>
          <span className="inline-flex items-center gap-2 text-xs font-normal text-muted-foreground">
            {data.workspace.source.configured
              ? "Telemetry configured"
              : "Manual ingestion"}{" "}
            · {data.workspace.role}
            <ChevronDown
              className="size-4 disclosure-chevron"
              aria-hidden="true"
            />
          </span>
        </summary>
        <div className="grid gap-0 border-t md:grid-cols-3">
          {[
            {
              page: "connections" as const,
              label: "Telemetry connections",
              detail: data.workspace.source.configured
                ? (data.workspace.source.connectionCount ?? 1) > 1
                  ? `${data.workspace.source.connectionCount} configured connections`
                  : `${providerLabel(data.workspace.source.kind)} configured`
                : "No connected source · manual ingestion",
              Icon: Database,
            },
            {
              page: "access" as const,
              label: "Access control",
              detail: `Your tenant role: ${data.workspace.role}`,
              Icon: ShieldCheck,
            },
            {
              page: "clients" as const,
              label: "API and clients",
              detail: "Go SDK, CLI and YAML resource API",
              Icon: Activity,
            },
          ].map((x) => (
            <button
              className="flex min-w-0 items-start gap-3 border-b p-5 text-left last:border-0 hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring focus-visible:-outline-offset-2 md:border-b-0 md:border-r"
              onClick={() => onNavigate(x.page)}
              key={x.page}
            >
              <x.Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <div>
                <span className="text-sm font-medium">{x.label}</span>
                <p className="mt-1 text-xs text-muted-foreground">{x.detail}</p>
              </div>
            </button>
          ))}
        </div>
      </details>
    </div>
  );
}
