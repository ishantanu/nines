import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  message,
  type Journey,
  type Objective,
  type RequestScope,
} from "@/lib/api";
import {
  badEventsText,
  budgetText,
  formatTime,
  type Incident,
} from "@/lib/incidents";
import type { Investigation } from "@/lib/investigations";
import {
  DEFINE_SLO_ACTION,
  actionLabels,
  causeLabels,
  hindsight,
  postmortemRevisions,
  readPostmortem,
  savePostmortem,
  type ActionKind,
  type CauseKind,
  type Postmortem,
} from "@/lib/postmortems";

const field = "h-9 w-full rounded-md border bg-background px-3 text-sm";

// Advice under a field: hindsight and blame wording, with what to say
// instead. Never a block.
function Hindsight({ text }: { text: string }) {
  const found = hindsight(text);
  if (!found.length) return null;
  return (
    <ul className="mt-1 space-y-1 text-xs text-amber-900" aria-live="polite">
      {found.map((f) => (
        <li key={f.phrase}>
          “{f.phrase}”: {f.suggestion}
        </li>
      ))}
    </ul>
  );
}

function TextList({
  id,
  label,
  items,
  disabled,
  onChange,
}: {
  id: string;
  label: string;
  items: string[];
  disabled: boolean;
  onChange: (items: string[]) => void;
}) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{label}</p>
      {items.map((text, n) => (
        <div key={n}>
          <div className="flex gap-2">
            <Input
              aria-label={`${label} ${n + 1}`}
              id={`${id}-${n}`}
              value={text}
              maxLength={1000}
              disabled={disabled}
              onChange={(e) =>
                onChange(items.map((v, i) => (i === n ? e.target.value : v)))
              }
            />
            <Button
              variant="outline"
              size="sm"
              disabled={disabled}
              aria-label={`Remove ${label.toLowerCase()} ${n + 1}`}
              onClick={() => onChange(items.filter((_, i) => i !== n))}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
          <Hindsight text={text} />
        </div>
      ))}
      <Button
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => onChange([...items, ""])}
      >
        <Plus className="size-4" />
        Add
      </Button>
    </div>
  );
}

// The incident as captured on the review: timeline and impact are never
// retyped in the postmortem.
function CapturedIncident({ incident }: { incident: Incident | null }) {
  if (!incident)
    return (
      <p className="text-sm text-muted-foreground">
        Capture context on this review to fill in the timeline and impact from
        the incident record.
      </p>
    );
  return (
    <div className="space-y-4">
      <div>
        <h4 className="mb-1 text-sm font-medium">Impact</h4>
        {incident.impact.length ? (
          <ul className="space-y-1 text-sm">
            {incident.impact.map((i) => (
              <li key={i.sloId}>
                {i.sloId}: {budgetText(i)} · {badEventsText(i)} bad events
                {i.impactStartedAt
                  ? ` · from ${formatTime(i.impactStartedAt)}`
                  : ""}
                {i.reason ? (
                  <span className="block text-xs text-muted-foreground">
                    {i.reason}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            No SLO measured this incident, so its impact is unknown.
          </p>
        )}
      </div>
      <div>
        <h4 className="mb-1 text-sm font-medium">Timeline from Nines</h4>
        <ol className="space-y-1 text-sm">
          {incident.timeline.map((e, n) => (
            <li key={n}>
              <span className="text-xs text-muted-foreground">
                {formatTime(e.time)} · {e.actor || "Nines"}
              </span>{" "}
              {e.message}
            </li>
          ))}
        </ol>
      </div>
    </div>
  );
}

export function PostmortemEditor({
  scope,
  review,
  incident,
  objectives,
  journeys,
  onSaved,
}: {
  scope: RequestScope;
  review: Investigation;
  incident: Incident | null;
  objectives: Objective[];
  journeys: Journey[];
  onSaved: (review: Investigation) => void;
}) {
  const [draft, setDraft] = useState<Postmortem | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState("");
  const writable = scope.role === "owner" || scope.role === "editor";
  const revisions = postmortemRevisions(review);

  useEffect(() => {
    let live = true;
    readPostmortem(scope, review.id)
      .then((t) => live && setDraft(t.postmortem))
      .catch((e) => live && setError(message(e)));
    return () => {
      live = false;
    };
    // Load once per review; later saves update the draft in place.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [review.id]);

  if (!draft)
    return (
      <div className="p-4 text-sm">
        {error ? (
          <p role="alert" className="text-amber-900">
            {error}
          </p>
        ) : (
          <p role="status">Loading postmortem…</p>
        )}
      </div>
    );
  const p = draft;
  const set = (next: Partial<Postmortem>) => {
    setSaved("");
    setDraft({ ...p, ...next });
  };

  async function save() {
    setBusy(true);
    setError("");
    try {
      const result = await savePostmortem(scope, review, p);
      onSaved(result.investigation);
      setSaved(
        result.findings.length
          ? `Saved. ${result.findings.length} wording note${result.findings.length > 1 ? "s" : ""} to consider.`
          : "Saved.",
      );
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }

  const disabled = !writable || busy;
  return (
    <div className="space-y-8 p-4">
      <p className="text-xs text-muted-foreground">
        A blameless postmortem: describe conditions and decisions as they were
        known at the time. {writable ? "" : "Only owners and editors can edit."}
      </p>

      <section className="space-y-2">
        <Label htmlFor="pm-summary">Summary</Label>
        <Textarea
          id="pm-summary"
          value={p.summary}
          maxLength={8000}
          disabled={disabled}
          onChange={(e) => set({ summary: e.target.value })}
        />
        <Hindsight text={p.summary} />
      </section>

      <section className="space-y-3">
        <h3 className="text-sm font-medium">Timeline and impact</h3>
        <CapturedIncident incident={incident} />
        <div className="space-y-2">
          <p className="text-sm font-medium">Events Nines could not know</p>
          {p.addedEvents.map((e, n) => (
            <div key={n}>
              <div className="flex flex-wrap gap-2">
                <Input
                  type="datetime-local"
                  aria-label={`Event ${n + 1} time`}
                  className="w-56"
                  value={e.time ? e.time.slice(0, 16) : ""}
                  disabled={disabled}
                  onChange={(ev) =>
                    set({
                      addedEvents: p.addedEvents.map((x, i) =>
                        i === n
                          ? {
                              ...x,
                              time: new Date(ev.target.value).toISOString(),
                            }
                          : x,
                      ),
                    })
                  }
                />
                <Input
                  aria-label={`Event ${n + 1}`}
                  className="min-w-0 flex-1"
                  value={e.text}
                  maxLength={1000}
                  disabled={disabled}
                  onChange={(ev) =>
                    set({
                      addedEvents: p.addedEvents.map((x, i) =>
                        i === n ? { ...x, text: ev.target.value } : x,
                      ),
                    })
                  }
                />
                <Button
                  variant="outline"
                  size="sm"
                  disabled={disabled}
                  aria-label={`Remove event ${n + 1}`}
                  onClick={() =>
                    set({
                      addedEvents: p.addedEvents.filter((_, i) => i !== n),
                    })
                  }
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
              <Hindsight text={e.text} />
            </div>
          ))}
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={() =>
              set({
                addedEvents: [
                  ...p.addedEvents,
                  { time: new Date().toISOString(), text: "" },
                ],
              })
            }
          >
            <Plus className="size-4" />
            Add event
          </Button>
        </div>
      </section>

      <section className="space-y-2">
        <h3 className="text-sm font-medium">Contributing causes</h3>
        <p className="text-xs text-muted-foreground">
          Incidents rarely have one root cause. List what set it off, the
          conditions already present, and the safeguards that were missing or
          weak.
        </p>
        {p.contributingCauses.map((c, n) => (
          <div key={n}>
            <div className="flex flex-wrap gap-2">
              <NativeSelect
                aria-label={`Cause ${n + 1} kind`}
                className={`${field} w-56`}
                value={c.kind}
                disabled={disabled}
                onChange={(e) =>
                  set({
                    contributingCauses: p.contributingCauses.map((x, i) =>
                      i === n ? { ...x, kind: e.target.value as CauseKind } : x,
                    ),
                  })
                }
              >
                {(Object.keys(causeLabels) as CauseKind[]).map((k) => (
                  <option key={k} value={k}>
                    {causeLabels[k]}
                  </option>
                ))}
              </NativeSelect>
              <Input
                aria-label={`Cause ${n + 1}`}
                className="min-w-0 flex-1"
                value={c.text}
                maxLength={2000}
                disabled={disabled}
                onChange={(e) =>
                  set({
                    contributingCauses: p.contributingCauses.map((x, i) =>
                      i === n ? { ...x, text: e.target.value } : x,
                    ),
                  })
                }
              />
              <Button
                variant="outline"
                size="sm"
                disabled={disabled}
                aria-label={`Remove cause ${n + 1}`}
                onClick={() =>
                  set({
                    contributingCauses: p.contributingCauses.filter(
                      (_, i) => i !== n,
                    ),
                  })
                }
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
            <Hindsight text={c.text} />
          </div>
        ))}
        <Button
          variant="outline"
          size="sm"
          disabled={disabled}
          onClick={() =>
            set({
              contributingCauses: [
                ...p.contributingCauses,
                { kind: "trigger", text: "" },
              ],
            })
          }
        >
          <Plus className="size-4" />
          Add cause
        </Button>
      </section>

      <section className="grid gap-6 md:grid-cols-3">
        <TextList
          id="pm-well"
          label="What went well"
          items={p.lessons.wentWell}
          disabled={disabled}
          onChange={(wentWell) => set({ lessons: { ...p.lessons, wentWell } })}
        />
        <TextList
          id="pm-poorly"
          label="What went poorly"
          items={p.lessons.wentPoorly}
          disabled={disabled}
          onChange={(wentPoorly) =>
            set({ lessons: { ...p.lessons, wentPoorly } })
          }
        />
        <TextList
          id="pm-lucky"
          label="Where we got lucky"
          items={p.lessons.gotLucky}
          disabled={disabled}
          onChange={(gotLucky) => set({ lessons: { ...p.lessons, gotLucky } })}
        />
      </section>

      <section className="space-y-3">
        <h3 className="text-sm font-medium">Action items</h3>
        {p.actionItems.map((a, n) => {
          const update = (next: Partial<typeof a>) =>
            set({
              actionItems: p.actionItems.map((x, i) =>
                i === n ? { ...x, ...next } : x,
              ),
            });
          const standing = a.id === DEFINE_SLO_ACTION;
          return (
            <div key={a.id} className="space-y-2 rounded-md border p-3">
              <div className="flex flex-wrap items-center gap-2">
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={a.done}
                    disabled={disabled}
                    onChange={(e) => update({ done: e.target.checked })}
                  />
                  Done
                </label>
                <Input
                  aria-label={`Action ${n + 1}`}
                  className="min-w-0 flex-1"
                  value={a.title}
                  maxLength={300}
                  disabled={disabled || standing}
                  onChange={(e) => update({ title: e.target.value })}
                />
                {standing ? null : (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={disabled}
                    aria-label={`Remove action ${n + 1}`}
                    onClick={() =>
                      set({
                        actionItems: p.actionItems.filter((_, i) => i !== n),
                      })
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                )}
              </div>
              {standing ? (
                <p className="text-xs text-muted-foreground">
                  No SLO detected this incident, so this item stays until the
                  SLO exists. Mark it done then.
                </p>
              ) : null}
              <Hindsight text={a.title} />
              <div className="grid gap-2 sm:grid-cols-5">
                <Input
                  aria-label={`Action ${n + 1} owner`}
                  placeholder="Owner"
                  value={a.owner}
                  maxLength={200}
                  disabled={disabled}
                  onChange={(e) => update({ owner: e.target.value })}
                />
                <Input
                  type="date"
                  aria-label={`Action ${n + 1} due`}
                  value={a.due ?? ""}
                  disabled={disabled}
                  onChange={(e) => update({ due: e.target.value || undefined })}
                />
                <NativeSelect
                  aria-label={`Action ${n + 1} kind`}
                  className={field}
                  value={a.kind}
                  disabled={disabled}
                  onChange={(e) =>
                    update({ kind: e.target.value as ActionKind })
                  }
                >
                  {(Object.keys(actionLabels) as ActionKind[]).map((k) => (
                    <option key={k} value={k}>
                      {actionLabels[k]}
                    </option>
                  ))}
                </NativeSelect>
                <NativeSelect
                  aria-label={`Action ${n + 1} SLO`}
                  className={field}
                  value={a.slos?.[0] ?? ""}
                  disabled={disabled}
                  onChange={(e) =>
                    update({ slos: e.target.value ? [e.target.value] : [] })
                  }
                >
                  <option value="">No SLO</option>
                  {objectives.map((o) => (
                    <option key={o.slo.id} value={o.slo.id}>
                      {o.slo.name || o.slo.id}
                    </option>
                  ))}
                </NativeSelect>
                <NativeSelect
                  aria-label={`Action ${n + 1} journey`}
                  className={field}
                  value={a.journeys?.[0] ?? ""}
                  disabled={disabled}
                  onChange={(e) =>
                    update({ journeys: e.target.value ? [e.target.value] : [] })
                  }
                >
                  <option value="">No journey</option>
                  {journeys.map((j) => (
                    <option
                      key={j.resource.manifest.metadata.name}
                      value={j.resource.manifest.metadata.name}
                    >
                      {j.resource.manifest.spec.displayName}
                    </option>
                  ))}
                </NativeSelect>
              </div>
            </div>
          );
        })}
        <Button
          variant="outline"
          size="sm"
          disabled={disabled}
          onClick={() =>
            set({
              actionItems: [
                ...p.actionItems,
                {
                  id: crypto.randomUUID(),
                  title: "",
                  owner: "",
                  kind: "prevent",
                  done: false,
                },
              ],
            })
          }
        >
          <Plus className="size-4" />
          Add action item
        </Button>
      </section>

      <section className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={p.published}
            disabled={disabled}
            onChange={(e) => set({ published: e.target.checked })}
          />
          Published to the tenant
        </label>
        <div className="flex items-center gap-3">
          {saved ? (
            <span role="status" className="text-sm text-muted-foreground">
              {saved}
            </span>
          ) : null}
          <Button disabled={disabled} onClick={() => void save()}>
            Save revision
          </Button>
        </div>
      </section>
      {error ? (
        <p role="alert" className="text-sm text-amber-900">
          {error}
        </p>
      ) : null}

      <section>
        <h3 className="mb-2 text-sm font-medium">Revisions</h3>
        {revisions.length ? (
          <ol className="space-y-1 text-xs text-muted-foreground">
            {revisions.map((r) => (
              <li key={r.id}>
                {formatTime(r.createdAt)} · {r.actor.name || r.actor.subject}
              </li>
            ))}
          </ol>
        ) : (
          <p className="text-xs text-muted-foreground">Not saved yet.</p>
        )}
      </section>
    </div>
  );
}
