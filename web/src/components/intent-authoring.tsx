import { useEffect, useState } from "react";
import { stringify } from "yaml";
import { api, message, type Journey, type RequestScope } from "@/lib/api";
import { RESOURCE_API_VERSION } from "@/lib/definitions";
import {
  intentGoal,
  intentPopulation,
  type IntentDocument,
  type IntentPlan,
} from "@/lib/intent";
import {
  IntentMeasurementFields,
  goalWithMeasurement,
} from "./intent-measurement";
import { measurementDraft, type MeasurementDraft } from "@/lib/measurement";
import { IntentEvidence } from "./intent-evidence";
import { IntentBacktestPanel } from "./intent-backtest";
import { duration, durationSeconds } from "@/lib/history";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

type Draft = {
  name: string;
  project: string;
  service: string;
  source: string;
  description: string;
  outcome: string;
  population: string;
  threshold: string;
  target: string;
  window: string;
  short: string;
  long: string;
  burn: string;
  pending: string;
  coverage: string;
};
const initial: Draft = {
  name: "",
  project: "",
  service: "",
  source: "",
  description: "",
  outcome: "availability",
  population: "",
  threshold: "",
  target: "99.9",
  window: "28d",
  short: "5m",
  long: "1h",
  burn: "",
  pending: "2m",
  coverage: "1",
};
export function IntentAuthoring({
  scope,
  journeys,
  onStartConversation,
}: {
  scope: RequestScope;
  journeys: Journey[];
  onStartConversation?: (goal: IntentDocument) => void;
}) {
  const [draft, setDraft] = useState(initial);
  const [measurement, setMeasurement] = useState<MeasurementDraft>(() => ({
    ...measurementDraft(),
    metric: "",
    observationPoint: "",
    success: [],
    step: "60",
    scrape: "15",
  }));
  const [modelEnabled, setModelEnabled] = useState(false);
  const [clarifications, setClarifications] = useState<string[]>([]);
  const [sources, setSources] = useState<{ id: string; kind: string }[]>([]);
  const [journey, setJourney] = useState("none"),
    [step, setStep] = useState("");
  const [alerts, setAlerts] = useState(false),
    [required, setRequired] = useState(true);
  const [plan, setPlan] = useState<IntentPlan | null>(null),
    [document, setDocument] = useState<IntentDocument | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  useEffect(() => {
    const c = new AbortController();
    api<{ connections: { id: string; kind: string }[] }>(
      "/api/connections",
      scope,
      { signal: c.signal },
    )
      .then((r) => setSources(r.connections))
      .catch((e) => {
        if (!c.signal.aborted && !scope.signal.aborted) setError(message(e));
      });
    api<{ naturalLanguageEnabled: boolean }>(
      "/api/intents/capabilities",
      scope,
      { signal: c.signal },
    )
      .then((r) => setModelEnabled(r.naturalLanguageEnabled))
      .catch(() => setModelEnabled(false));
    return () => c.abort();
  }, [scope]);
  function invalidate() {
    setPlan(null);
    setDocument(null);
    setError("");
    setClarifications([]);
  }
  function field(key: keyof Draft, value: string) {
    setDraft((d) => ({ ...d, [key]: value }));
    invalidate();
  }
  const selectedJourney = journeys.find(
    (j) => j.resource.manifest.metadata.name === journey,
  );
  function build(): IntentDocument {
    if (journey !== "none" && !selectedJourney)
      throw new Error("Select a current journey.");
    return goalWithMeasurement(
      {
        apiVersion: RESOURCE_API_VERSION,
        kind: "ServiceLevelObjectiveIntent",
        metadata: { name: draft.name, project: draft.project },
        spec: {
          description: draft.description,
          service: draft.service,
          source: draft.source,
          outcome: draft.outcome,
          population: intentPopulation(draft.population),
          ...intentGoal(draft.target, draft.window),
          noDataPolicy: "unknown",
          ...(draft.outcome === "latency"
            ? { thresholdSeconds: Number(draft.threshold) }
            : {}),
          ...(alerts
            ? {
                policy: {
                  shortWindowSeconds: durationSeconds(draft.short),
                  longWindowSeconds: durationSeconds(draft.long),
                  burnRate: Number(draft.burn),
                  forSeconds: durationSeconds(draft.pending),
                  minCoverage: Number(draft.coverage),
                },
              }
            : {}),
          ...(selectedJourney
            ? {
                journey: {
                  name: journey,
                  step,
                  expectedRevision: selectedJourney.resource.revision,
                  required,
                },
              }
            : {}),
        },
      },
      measurement,
    );
  }
  async function draftLanguage() {
    setBusy(true);
    invalidate();
    try {
      const r = await api<{
        status: string;
        model: string;
        intent?: IntentDocument;
        clarifications: string[];
      }>("/api/intents/draft", scope, {
        method: "POST",
        body: JSON.stringify({
          text: draft.description,
          metadata: { name: draft.name, project: draft.project },
          service: draft.service,
          source: draft.source,
          population: intentPopulation(draft.population),
          noDataPolicy: "unknown",
        }),
      });
      setClarifications(
        r.intent
          ? [
              `Draft prepared with ${r.model}. Review the typed fields before verifying the measurement.`,
              ...r.clarifications,
            ]
          : r.clarifications,
      );
      if (r.intent) {
        const spec = r.intent.spec;
        setDraft((d) => ({
          ...d,
          outcome: spec.outcome,
          target: String(Number((spec.target * 100).toPrecision(12))),
          window: duration(spec.windowSeconds),
          threshold:
            spec.thresholdSeconds == null ? "" : String(spec.thresholdSeconds),
        }));
      }
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function discover() {
    setBusy(true);
    invalidate();
    try {
      const intent = build();
      const p = await api<IntentPlan>("/api/intents/plan", scope, {
        method: "POST",
        body: JSON.stringify({ intent }),
      });
      setDocument(intent);
      setPlan(p);
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function select(name: string, expectedRevision: number) {
    if (!document) return;
    setBusy(true);
    setError("");
    try {
      setPlan(
        await api<IntentPlan>("/api/intents/plan", scope, {
          method: "POST",
          body: JSON.stringify({
            intent: document,
            binding: { name, expectedRevision },
          }),
        }),
      );
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const input = (key: keyof Draft, label: string, placeholder?: string) => (
    <div className="grid gap-2" key={key}>
      <Label htmlFor={`intent-${key}`}>{label}</Label>
      <Input
        id={`intent-${key}`}
        value={draft[key]}
        placeholder={placeholder}
        onChange={(e) => field(key, e.target.value)}
      />
    </div>
  );
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Reliability goal</CardTitle>
          <p className="text-sm text-muted-foreground">
            Preview the outcome and eligible traffic, then continue in the
            approval workflow to confirm a measurement and review changes. This
            form does not apply definitions.
          </p>
        </CardHeader>
        <CardContent>
          <fieldset disabled={busy} className="space-y-5">
            <div className="grid gap-4 md:grid-cols-3">
              {input("name", "SLO ID", "checkout-success")}
              {input("project", "Project", "commerce")}
              {input("service", "Service", "checkout")}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="intent-description">
                Desired user experience
              </Label>
              <Textarea
                id="intent-description"
                value={draft.description}
                onChange={(e) => field("description", e.target.value)}
                placeholder="Production checkout attempts should complete successfully."
              />
              <p className="text-xs text-muted-foreground">
                The fields below define the executable goal. This description is
                preserved for review.
              </p>
            </div>
            <div className="grid gap-4 md:grid-cols-3">
              <div className="grid gap-2">
                <Label htmlFor="intent-source">Data source</Label>
                <Select
                  value={draft.source}
                  onValueChange={(v) => field("source", v)}
                >
                  <SelectTrigger id="intent-source">
                    <SelectValue placeholder="Select a connection" />
                  </SelectTrigger>
                  <SelectContent>
                    {sources.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.id} ({s.kind})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="intent-outcome">Outcome</Label>
                <Select
                  value={draft.outcome}
                  onValueChange={(v) => field("outcome", v)}
                >
                  <SelectTrigger id="intent-outcome">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="availability">
                      Successful attempts
                    </SelectItem>
                    <SelectItem value="latency">
                      Attempts within a duration
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {draft.outcome === "latency" ? (
                input("threshold", "Maximum duration (seconds)", "0.3")
              ) : (
                <p className="self-end text-sm text-muted-foreground">
                  Define the metric and success labels below.
                </p>
              )}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="intent-population">
                Eligible traffic labels (YAML)
              </Label>
              <Textarea
                id="intent-population"
                className="min-h-24 font-mono text-xs"
                value={draft.population}
                onChange={(e) => field("population", e.target.value)}
                placeholder={'environment: "production"\nservice: "checkout"'}
              />
              <p className="text-xs text-muted-foreground">
                These labels select eligible events. Environment, retries and
                transaction identity are never inferred.
              </p>
            </div>
            <section
              className="space-y-4 rounded-md border p-4"
              aria-label="SLO measurement"
            >
              <h3 className="font-medium">How to measure this outcome</h3>
              <p className="text-sm text-muted-foreground">
                The measurement belongs to this SLO. Verify the metric, success
                criteria and observation point before preparing a proposal.
              </p>
              <IntentMeasurementFields
                draft={{
                  ...measurement,
                  measurement: draft.outcome as "availability" | "latency",
                }}
                disabled={busy}
                onChange={(d) => {
                  setMeasurement(d);
                  invalidate();
                }}
              />
            </section>
            <div className="grid gap-4 md:grid-cols-3">
              {input("target", "Target (%)")}
              {input("window", "Rolling window", "28d")}
              <div className="grid gap-2">
                <Label>Missing data</Label>
                <p className="flex h-9 items-center text-sm">
                  Unknown; never counted as success
                </p>
              </div>
            </div>
            <div className="grid gap-4 md:grid-cols-3">
              <div className="grid gap-2">
                <Label htmlFor="intent-journey">Critical user journey</Label>
                <Select
                  value={journey}
                  onValueChange={(v) => {
                    setJourney(v);
                    setStep("");
                    invalidate();
                  }}
                >
                  <SelectTrigger id="intent-journey">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">No journey link</SelectItem>
                    {journeys.map((j) => (
                      <SelectItem
                        key={j.resource.manifest.metadata.name}
                        value={j.resource.manifest.metadata.name}
                      >
                        {j.resource.manifest.spec.displayName}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {selectedJourney ? (
                <>
                  <div className="grid gap-2">
                    <Label htmlFor="intent-step">Journey step</Label>
                    <Select
                      value={step}
                      onValueChange={(v) => {
                        setStep(v);
                        invalidate();
                      }}
                    >
                      <SelectTrigger id="intent-step">
                        <SelectValue placeholder="Select a step" />
                      </SelectTrigger>
                      <SelectContent>
                        {selectedJourney.resource.manifest.spec.steps.map(
                          (s) => (
                            <SelectItem key={s.id} value={s.id}>
                              {s.name}
                            </SelectItem>
                          ),
                        )}
                      </SelectContent>
                    </Select>
                  </div>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={required}
                      onChange={(e) => {
                        setRequired(e.target.checked);
                        invalidate();
                      }}
                    />
                    Required for journey health
                  </label>
                </>
              ) : null}
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={alerts}
                onChange={(e) => {
                  setAlerts(e.target.checked);
                  invalidate();
                }}
              />
              Include a burn-rate alert policy
            </label>
            {alerts ? (
              <div className="grid gap-4 md:grid-cols-3">
                {input("short", "Short window")}
                {input("long", "Long window")}
                {input("burn", "Burn-rate threshold", "Enter a threshold")}
                {input("pending", "Pending duration")}
                {input("coverage", "Minimum coverage (0 to 1)")}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">
                No alert policy will be added. Recording and alert rules are
                deployed through the separate rule review.
              </p>
            )}
            <div className="space-y-2 border-t pt-4">
              {modelEnabled ? (
                <>
                  <Button
                    variant="outline"
                    onClick={draftLanguage}
                    disabled={busy || scope.role === "viewer"}
                  >
                    Draft goal from description
                  </Button>
                  <p className="text-xs text-muted-foreground">
                    Sends only this description to the configured model. Include
                    a percentage and rolling duration; review the resulting
                    fields before verifying telemetry.
                  </p>
                </>
              ) : (
                <p className="text-xs text-muted-foreground">
                  Natural-language drafting is not configured for this tenant.
                  Structured authoring remains available.
                </p>
              )}
              {clarifications.map((q) => (
                <p key={q} role="status" className="text-sm">
                  {q}
                </p>
              ))}
            </div>
            {onStartConversation ? (
              <Button
                variant="outline"
                disabled={busy || scope.role === "viewer"}
                onClick={() => {
                  try {
                    onStartConversation(build());
                  } catch (e) {
                    setError(message(e));
                  }
                }}
              >
                Start conversation from this goal
              </Button>
            ) : null}
            <Button onClick={discover} disabled={busy}>
              {busy ? "Checking…" : "Verify measurement"}
            </Button>
          </fieldset>
        </CardContent>
      </Card>
      {error ? (
        <p
          role="alert"
          className="rounded-md border border-destructive/30 p-4 text-sm text-destructive"
        >
          {error}
        </p>
      ) : null}
      {plan ? (
        <Card>
          <CardHeader>
            <CardTitle>
              {plan.status === "ready"
                ? "Review generated definitions"
                : "Measurement selection"}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {plan.clarifications.map((s) => (
              <p key={s} className="text-sm">
                {s}
              </p>
            ))}
            {plan.candidates.map((c) => (
              <div key={c.binding.name} className="rounded-md border p-4">
                <h3 className="font-medium">{c.definition.spec.displayName}</h3>
                <dl className="mt-3 grid gap-2 text-sm md:grid-cols-2">
                  <div>
                    <dt className="text-muted-foreground">Observation point</dt>
                    <dd>{c.definition.spec.observationPoint}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">Measurement</dt>
                    <dd className="break-all font-mono text-xs">
                      {c.definition.spec.metric}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">Success criteria</dt>
                    <dd className="break-all">
                      {c.definition.spec.measurement === "latency"
                        ? `Duration at most ${c.definition.spec.thresholdSeconds} seconds`
                        : JSON.stringify(c.definition.spec.successLabels)}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">Binding revision</dt>
                    <dd>
                      {c.binding.name} · {c.binding.revision}
                    </dd>
                  </div>
                </dl>
                <Button
                  className="mt-4"
                  variant="outline"
                  disabled={busy}
                  onClick={() => select(c.binding.name, c.binding.revision)}
                >
                  Select and validate telemetry
                </Button>
              </div>
            ))}
            {plan.compiled ? (
              <>
                <IntentEvidence
                  compiled={plan.compiled}
                  sourceDigest={plan.sourceDigest}
                />
                <p className="text-sm">
                  Metadata and series checked at{" "}
                  {new Date(plan.compiled.evidence.to).toLocaleString()}. This
                  does not establish historical SLI performance.
                </p>
                <details>
                  <summary className="cursor-pointer text-sm font-medium">
                    Intent YAML
                  </summary>
                  <pre className="code-block mt-3 max-h-96 overflow-auto">
                    {stringify(document)}
                  </pre>
                </details>
                <Label htmlFor="intent-slo-yaml">SLO YAML</Label>
                <Textarea
                  id="intent-slo-yaml"
                  readOnly
                  value={plan.compiled.yaml}
                  className="h-80 font-mono text-xs"
                />
                <p className="break-all font-mono text-xs text-muted-foreground">
                  SHA-256 {plan.compiled.digest}
                </p>
                <p className="text-sm text-muted-foreground">
                  This preview is read-only. Continue in the conversation to
                  confirm the measurement and record recommendation evidence,
                  approval and audit history.
                </p>
                <Button
                  disabled={
                    busy ||
                    scope.role === "viewer" ||
                    !document ||
                    !onStartConversation
                  }
                  onClick={() => {
                    if (document) onStartConversation?.(document);
                  }}
                >
                  Continue to review and approval
                </Button>
                {plan.journey ? (
                  <>
                    <Label htmlFor="intent-cuj-yaml">
                      CUJ update (revision {plan.journey.expectedRevision})
                    </Label>
                    <Textarea
                      id="intent-cuj-yaml"
                      readOnly
                      value={plan.journey.yaml}
                      className="h-64 font-mono text-xs"
                    />
                    <p className="text-sm text-muted-foreground">
                      The approved change set applies the SLO and this journey
                      link atomically.
                    </p>
                  </>
                ) : null}
                {[
                  ...plan.compiled.warnings,
                  ...plan.compiled.evidence.warnings,
                ].map((s) => (
                  <p key={s} className="text-xs text-muted-foreground">
                    {s}
                  </p>
                ))}
              </>
            ) : null}
            <details>
              <summary className="cursor-pointer text-sm">
                Scope and limitations
              </summary>
              <ul className="mt-3 list-disc space-y-2 pl-5 text-xs text-muted-foreground">
                {plan.warnings.map((s) => (
                  <li key={s}>{s}</li>
                ))}
              </ul>
            </details>
          </CardContent>
        </Card>
      ) : null}
      {plan?.compiled && document ? (
        <IntentBacktestPanel
          key={plan.compiled.digest}
          scope={scope}
          intent={document}
          binding={plan.compiled.binding}
          step={
            plan.compiled.manifest?.spec.stepSeconds ??
            plan.compiled.definition?.spec.stepSeconds ??
            60
          }
        />
      ) : null}
    </div>
  );
}
