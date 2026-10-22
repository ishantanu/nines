import { Activity, ArrowRight, Database, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Status } from "@/components/status";
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
  const attention = data.objectives.filter(
    (o) => healthOf(o.status) !== "healthy",
  );
  const impacted = data.journeys.filter((j) => j.health !== "healthy");
  return (
    <div className="space-y-4">
      <dl className="summary-grid">
        {[
          { label: "Objectives", value: data.objectives.length },
          { label: "Journeys", value: data.journeys.length },
          { label: "Objectives requiring attention", value: attention.length },
          { label: "Affected journeys", value: impacted.length },
        ].map((x) => (
          <div className="summary-metric" key={x.label}>
            <dt>{x.label}</dt>
            <dd>{x.value}</dd>
          </div>
        ))}
      </dl>
      <div className="panel-grid">
        <Card className="gap-0 py-0">
          <div className="panel-header">
            <h2 className="text-sm font-semibold">
              Objectives requiring attention
            </h2>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onNavigate("objectives")}
            >
              View SLOs
              <ArrowRight className="size-3.5" />
            </Button>
          </div>
          <CardContent className="panel-list p-0">
            {attention.length ? (
              attention.slice(0, 8).map((o) => (
                <button
                  key={o.slo.id}
                  className="flex w-full items-center justify-between gap-3 border-b px-5 py-4 text-left last:border-0 hover:bg-accent"
                  onClick={() => onSLO(o.slo.id)}
                >
                  <div>
                    <span className="text-sm font-medium">{o.slo.name}</span>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {o.slo.service} ·{" "}
                      {percent(o.status.budgetRemainingFraction, 1)} budget
                      remaining
                    </p>
                  </div>
                  <Status value={healthOf(o.status)} />
                </button>
              ))
            ) : (
              <p className="empty-state">
                {data.objectives.length
                  ? "No objectives require attention."
                  : "No SLOs configured. Create an objective to start monitoring."}
              </p>
            )}
          </CardContent>
        </Card>
        <Card className="gap-0 py-0">
          <div className="panel-header">
            <h2 className="text-sm font-semibold">Journey impact</h2>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onNavigate("journeys")}
            >
              View journeys
              <ArrowRight className="size-3.5" />
            </Button>
          </div>
          <CardContent className="panel-list p-0">
            {impacted.length ? (
              impacted.slice(0, 8).map((j) => (
                <button
                  key={j.resource.manifest.metadata.name}
                  className="flex w-full items-center justify-between gap-3 border-b px-5 py-4 text-left last:border-0 hover:bg-accent"
                  onClick={() => onJourney(j.resource.manifest.metadata.name)}
                >
                  <div>
                    <span className="text-sm font-medium">
                      {j.resource.manifest.spec.displayName}
                    </span>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {j.resource.manifest.spec.owner}
                    </p>
                  </div>
                  <Status value={j.health} />
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
      <Card className="gap-0 py-0">
        <div className="border-b px-5 py-4">
          <h2 className="text-sm font-semibold">Tenant configuration</h2>
        </div>
        <CardContent className="grid gap-0 p-0 md:grid-cols-3">
          {[
            {
              page: "connections" as const,
              label: "Data sources",
              detail: data.workspace.source.configured
                ? (data.workspace.source.connectionCount ?? 1) > 1
                  ? `${data.workspace.source.connectionCount} configured connections`
                  : `${data.workspace.source.kind === "mimir" ? "Mimir" : "Prometheus"} configured`
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
              className="flex items-start gap-3 border-b p-5 text-left last:border-0 hover:bg-accent md:border-b-0 md:border-r"
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
        </CardContent>
      </Card>
    </div>
  );
}
