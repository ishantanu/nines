import { useEffect, useState } from "react";
import { FileText, Plus, RefreshCw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { canEdit, message, type Objective, type RequestScope } from "@/lib/api";
import {
  badEventsText,
  budgetText,
  declareIncident,
  detectionLag,
  formatTime,
  listIncidents,
  openedBy,
  toolName,
  nextStates,
  stateLabels,
  transitionIncident,
  worstImpact,
  type Incident,
  type IncidentState,
} from "@/lib/incidents";
import { createInvestigation, findIncidentReview } from "@/lib/investigations";
import { navigateTo, useResourceRoute, workspacePath } from "@/lib/navigation";
import { cn } from "@/lib/utils";

function StateBadge({ state }: { state: IncidentState }) {
  return (
    <Badge
      variant={
        state === "detected"
          ? "destructive"
          : state === "resolved"
            ? "outline"
            : "secondary"
      }
    >
      {stateLabels[state]}
    </Badge>
  );
}

export function IncidentDetail({
  incident,
  objectives,
  writable,
  busy,
  onTransition,
  onReview,
  reviewsOn = true,
  onDefineSLO,
  onSLO,
  onJourney,
}: {
  incident: Incident;
  objectives: Objective[];
  writable: boolean;
  busy: boolean;
  onTransition: (state: IncidentState) => void;
  onReview: () => void;
  reviewsOn?: boolean;
  onDefineSLO?: () => void;
  onSLO: (id: string) => void;
  onJourney: (id: string) => void;
}) {
  const name = (id: string) =>
    objectives.find((o) => o.slo.id === id)?.slo.name || id;
  return (
    <div className="space-y-6 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="section-label break-words">{incident.title}</h2>
            <StateBadge state={incident.state} />
          </div>
          <p className="text-xs text-muted-foreground">
            {openedBy(incident)} · {formatTime(incident.openedAt)} · revision{" "}
            {incident.revision}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {nextStates[incident.state].map((s) => (
            <Button
              key={s}
              size="sm"
              variant={s === "resolved" ? "default" : "outline"}
              disabled={!writable || busy}
              onClick={() => onTransition(s)}
            >
              Mark {stateLabels[s].toLowerCase()}
            </Button>
          ))}
          {reviewsOn ? (
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={onReview}
            >
              <FileText className="size-4" />
              Start review
            </Button>
          ) : null}
        </div>
      </div>

      {!incident.detectedBySLO ? (
        <div
          role="note"
          className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900"
        >
          <p className="font-medium">Not detected by any SLO</p>
          <p className="mt-1">
            No SLO alert fired for this incident, so Nines could not see it
            coming and cannot measure its impact. Define an SLO for what users
            experienced so the next one is detected.
          </p>
          {onDefineSLO ? (
            <Button
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={onDefineSLO}
            >
              Define an SLO
            </Button>
          ) : null}
        </div>
      ) : null}

      {incident.signals?.length ? (
        <section aria-labelledby="incident-signals">
          <h3 id="incident-signals" className="mb-2 text-sm font-medium">
            Alerts from other tools
          </h3>
          <ul className="space-y-2 text-sm">
            {incident.signals.map((s) => (
              <li key={s.source + s.key}>
                <span className="font-medium">{s.title}</span>{" "}
                <span className="text-xs text-muted-foreground">
                  {toolName(s.source)} ·{" "}
                  {s.firing
                    ? `firing since ${formatTime(s.firingSince)}`
                    : `resolved ${formatTime(s.clearedAt)}`}
                </span>
                {s.url ? (
                  <a
                    href={s.url}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="ml-2 text-xs underline"
                  >
                    Open in {toolName(s.source)}
                  </a>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section aria-labelledby="incident-impact">
        <h3 id="incident-impact" className="mb-2 text-sm font-medium">
          Impact
        </h3>
        {incident.slos.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No SLOs are linked to this incident, so Nines cannot measure its
            impact. Alerts on SLOs join it when they share a journey.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-muted-foreground">
                <tr>
                  <th className="py-1 pr-4 font-medium">SLO</th>
                  <th className="py-1 pr-4 font-medium">Impact started</th>
                  <th className="py-1 pr-4 font-medium">Alert</th>
                  <th className="py-1 pr-4 font-medium">Bad events</th>
                  <th className="py-1 pr-4 font-medium">Budget consumed</th>
                  <th className="py-1 font-medium">Data coverage</th>
                </tr>
              </thead>
              <tbody>
                {incident.slos.map((slo) => {
                  const impact = incident.impact.find(
                    (i) => i.sloId === slo.id,
                  );
                  const lag = detectionLag(slo, impact);
                  return (
                    <tr key={slo.id} className="border-t align-top">
                      <td className="py-2 pr-4">
                        <button
                          className="text-left underline-offset-2 hover:underline"
                          onClick={() => onSLO(slo.id)}
                        >
                          {name(slo.id)}
                        </button>
                        <p className="text-xs text-muted-foreground">
                          Definition revision {slo.configRevision}
                        </p>
                      </td>
                      <td className="py-2 pr-4 tabular-nums">
                        {formatTime(impact?.impactStartedAt)}
                        {lag ? (
                          <p className="text-xs text-muted-foreground">
                            {lag} before the alert
                          </p>
                        ) : null}
                      </td>
                      <td className="py-2 pr-4">
                        {slo.firing
                          ? `Firing since ${formatTime(slo.firingSince)}`
                          : slo.clearedAt
                            ? `Cleared ${formatTime(slo.clearedAt)}`
                            : "Not firing"}
                      </td>
                      <td className="py-2 pr-4 tabular-nums">
                        {impact ? badEventsText(impact) : "Unknown"}
                      </td>
                      <td className="py-2 pr-4">
                        {impact ? budgetText(impact) : "Unknown"}
                        {impact?.reason ? (
                          <p className="text-xs text-muted-foreground">
                            {impact.reason}
                          </p>
                        ) : null}
                      </td>
                      <td className="py-2 tabular-nums">
                        {impact
                          ? `${(impact.coverage * 100).toFixed(0)}%`
                          : "—"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        <p className="mt-2 text-xs text-muted-foreground">
          Computed from stored measurements with the same arithmetic as SLO
          status. Missing data stays unknown, never zero.
        </p>
      </section>

      <section aria-labelledby="incident-journeys">
        <h3 id="incident-journeys" className="mb-2 text-sm font-medium">
          Affected critical user journeys
        </h3>
        {incident.journeys.length ? (
          <ul className="space-y-1 text-sm">
            {incident.journeys.map((j) => (
              <li key={j.id}>
                <button
                  className="underline-offset-2 hover:underline"
                  onClick={() => onJourney(j.id)}
                >
                  {j.id}
                </button>{" "}
                <span className="text-xs text-muted-foreground">
                  revision {j.revision}
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            None of the affected SLOs belongs to a critical user journey.
          </p>
        )}
      </section>

      <section aria-labelledby="incident-timeline">
        <h3 id="incident-timeline" className="mb-2 text-sm font-medium">
          Timeline
        </h3>
        <ol className="space-y-3 border-l pl-4">
          {incident.timeline.map((e, n) => (
            <li key={n} className="text-sm">
              <p className="text-xs text-muted-foreground">
                <time dateTime={e.time}>{formatTime(e.time)}</time> ·{" "}
                {e.actor ? e.actor : "Nines"}
              </p>
              <p className="break-words">{e.message}</p>
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}

export function Incidents({
  scope,
  objectives,
  reviewsOn,
  onDefineSLO,
  onSLO,
  onJourney,
}: {
  scope: RequestScope;
  objectives: Objective[];
  reviewsOn: boolean;
  onDefineSLO?: () => void;
  onSLO: (id: string) => void;
  onJourney: (id: string) => void;
}) {
  const [items, setItems] = useState<Incident[]>([]);
  const [selected, setSelected] = useResourceRoute(scope.tenant, "incidents");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [declaring, setDeclaring] = useState(false);
  const [title, setTitle] = useState("");
  const [chosen, setChosen] = useState<string[]>([]);
  const [transition, setTransition] = useState<IncidentState | null>(null);
  const [note, setNote] = useState("");
  const writable = canEdit(scope);
  const current = items.find((i) => i.id === selected) ?? items[0];

  async function reload() {
    setError("");
    try {
      setItems(await listIncidents(scope, scope.signal));
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    void reload();
    // Incidents change as alerts fire and clear; refresh while open.
    const timer = window.setInterval(() => void reload(), 30_000);
    return () => window.clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scope]);

  async function act(run: () => Promise<Incident>) {
    setBusy(true);
    setError("");
    try {
      const updated = await run();
      setItems((prev) =>
        [updated, ...prev.filter((i) => i.id !== updated.id)].sort((a, b) =>
          b.openedAt.localeCompare(a.openedAt),
        ),
      );
      setSelected(updated.id);
      return true;
    } catch (e) {
      setError(message(e));
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function startReview(incident: Incident) {
    setBusy(true);
    setError("");
    try {
      // One review per incident: open the existing one rather than start a
      // second.
      const existing = await findIncidentReview(scope, incident.id);
      if (existing) {
        navigateTo(workspacePath(scope.tenant, "investigations", existing));
        return;
      }
      const review = await createInvestigation(scope, {
        id: crypto.randomUUID(),
        title: `Review: ${incident.title}`.slice(0, 160),
        scope: {
          kind: "Incident",
          resourceId: incident.id,
          expectedRevision: incident.revision,
          at: new Date(Math.floor(Date.now() / 1000) * 1000 - 1000)
            .toISOString()
            .replace(".000Z", "Z"),
        },
      });
      navigateTo(workspacePath(scope.tenant, "investigations", review.id));
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          Critical burn alerts and paging alerts from your other tools open
          incidents. Nines does not page anyone; keep your response tools.
        </p>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => void reload()}
            aria-label="Reload incidents"
          >
            <RefreshCw className="size-4" />
          </Button>
          <Button
            disabled={!writable || busy}
            onClick={() => {
              setTitle("");
              setChosen([]);
              setDeclaring(true);
            }}
          >
            <Plus className="size-4" />
            Declare incident
          </Button>
        </div>
      </div>
      {error ? (
        <p
          role="alert"
          className="mb-4 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900"
        >
          {error}
        </p>
      ) : null}
      <div className="investigation-workspace">
        <Card className="investigation-list">
          <div className="panel-header">
            <h2 className="section-label">Incidents</h2>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-2">
            {items.map((item) => (
              <button
                key={item.id}
                className={cn(
                  "mb-1 w-full rounded-md border border-transparent p-3 text-left hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring",
                  current?.id === item.id && "border-border bg-muted",
                )}
                onClick={() => setSelected(item.id)}
                aria-current={current?.id === item.id ? "true" : undefined}
              >
                <p className="mb-2 break-words text-sm font-medium">
                  {item.title}
                </p>
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <StateBadge state={item.state} />
                  <span>{formatTime(item.openedAt)}</span>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {worstImpact(item)}
                </p>
              </button>
            ))}
            {loading && !items.length ? (
              <p role="status" className="p-3 text-sm">
                Loading incidents…
              </p>
            ) : !items.length ? (
              <p className="p-3 text-sm text-muted-foreground">
                No incidents. A critical burn alert opens one, or declare one
                yourself.
              </p>
            ) : null}
          </div>
        </Card>
        {current ? (
          <Card className="investigation-main overflow-y-auto">
            <IncidentDetail
              incident={current}
              objectives={objectives}
              writable={writable}
              busy={busy}
              onTransition={(s) => {
                setNote("");
                setTransition(s);
              }}
              onReview={() => void startReview(current)}
              reviewsOn={reviewsOn}
              onDefineSLO={onDefineSLO}
              onSLO={onSLO}
              onJourney={onJourney}
            />
          </Card>
        ) : null}
      </div>

      <Dialog open={declaring} onOpenChange={setDeclaring}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Declare an incident</DialogTitle>
            <DialogDescription>
              Declaring early carries no penalty. Alerts on the SLOs you choose
              join this incident instead of opening another.
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={async (e) => {
              e.preventDefault();
              if (
                await act(() => declareIncident(scope, { title, slos: chosen }))
              )
                setDeclaring(false);
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="incident-title">
                What users are experiencing
              </Label>
              <Input
                id="incident-title"
                value={title}
                maxLength={200}
                onChange={(e) => setTitle(e.target.value)}
                required
              />
            </div>
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">Affected SLOs</legend>
              <div className="max-h-48 space-y-1 overflow-y-auto">
                {objectives.map((o) => (
                  <label
                    key={o.slo.id}
                    className="flex items-center gap-2 text-sm"
                  >
                    <input
                      type="checkbox"
                      checked={chosen.includes(o.slo.id)}
                      onChange={(e) =>
                        setChosen((prev) =>
                          e.target.checked
                            ? [...prev, o.slo.id]
                            : prev.filter((id) => id !== o.slo.id),
                        )
                      }
                    />
                    {o.slo.name || o.slo.id}
                  </label>
                ))}
              </div>
            </fieldset>
            <div className="dialog-actions">
              <Button
                type="button"
                variant="outline"
                onClick={() => setDeclaring(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={busy || !title.trim()}>
                Declare
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={transition !== null}
        onOpenChange={(o) => !o && setTransition(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Mark {transition ? stateLabels[transition].toLowerCase() : ""}
            </DialogTitle>
            <DialogDescription>
              Say what changed. The note goes on the timeline with your name.
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!current || !transition) return;
              if (
                await act(() =>
                  transitionIncident(scope, current, transition, note),
                )
              )
                setTransition(null);
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="incident-note">Note</Label>
              <Textarea
                id="incident-note"
                value={note}
                maxLength={2000}
                onChange={(e) => setNote(e.target.value)}
              />
            </div>
            <div className="dialog-actions">
              <Button
                type="button"
                variant="outline"
                onClick={() => setTransition(null)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={busy}>
                Save
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
