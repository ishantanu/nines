import { providerLabel } from "@/lib/telemetry-providers";
import { FormField } from "@/components/ui/form-layout";
import { SLORevisionHistory } from "./slo-revision-history";
import { MeasuredObjective } from "./measured-objective";
import { BudgetPolicyEditor } from "./budget-policy-editor";
import { QualityPolicyEditor } from "./quality-policy-editor";
import { SLOLifecycleSelect } from "./slo-lifecycle";
import { PolicyEditor } from "./policy-editor";
import { useEffect, useRef, useState } from "react";
import { stringifyDefinition as stringify } from "@/lib/definition-yaml";
import {
  newObjectiveDefinition,
  inlineObjectiveDraft,
  inlineObjectiveDefinition,
  parseObjectiveDefinition,
} from "@/lib/objective-editor";
import { Plus, Settings2 } from "lucide-react";
import { definitionFromSLO, type SLODefinition } from "@/lib/definitions";
import { duration, durationSeconds } from "@/lib/history";
import { api, canEdit, message, type RequestScope, type SLO } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
type Draft = {
  errorBudgetPolicy?: SLO["errorBudgetPolicy"];
  quality?: SLO["quality"];
  lifecycle: NonNullable<SLO["lifecycle"]>;
  customMeasurement: boolean;
  policies: NonNullable<SLO["policies"]>;
  providerWindow: boolean;
  scrape: string;
  name: string;
  project: string;
  displayName: string;
  service: string;
  source: string;
  queryLanguage: string;
  target: string;
  window: string;
  step: string;
  goodQuery: string;
  totalQuery: string;
  enabled: boolean;
  short: string;
  long: string;
  burn: string;
  pending: string;
  coverage: string;
};
export function objectiveDraft(
  d: SLODefinition,
  derived?: SLO["providerWindow"],
): Draft {
  const window = d.spec.providerWindow ?? derived;
  const p = d.spec.policy;
  return {
    lifecycle: d.spec.lifecycle ?? "committed",
    quality: d.spec.quality,
    errorBudgetPolicy: d.spec.errorBudgetPolicy,
    customMeasurement: false,
    providerWindow: !!window,
    scrape: String(window?.scrapeIntervalSeconds ?? 15),
    name: d.metadata.name,
    project: d.metadata.project,
    displayName: d.spec.displayName,
    service: d.spec.service,
    source: d.spec.source ?? "",
    queryLanguage: d.spec.queryLanguage ?? "",
    target: String(d.spec.target),
    window: duration(d.spec.windowSeconds),
    step: duration(d.spec.stepSeconds),
    goodQuery: window?.goodSelector ?? d.spec.goodQuery ?? "",
    totalQuery: window?.totalSelector ?? d.spec.totalQuery ?? "",
    enabled: !!p || !!d.spec.policies?.length,
    policies:
      d.spec.policies ??
      (p
        ? [
            {
              ...p,
              name: p.name || "primary",
              severity: p.severity || "critical",
            },
          ]
        : []),
    short: duration(p?.shortWindowSeconds ?? 300),
    long: duration(p?.longWindowSeconds ?? 3600),
    burn: String(p?.burnRate ?? 14.4),
    pending: duration(p?.forSeconds ?? 120),
    coverage: String(p?.minCoverage ?? 1),
  };
}
export function objectiveDefinition(
  base: SLODefinition,
  d: Draft,
): SLODefinition {
  if (d.enabled && !d.policies.length)
    throw new Error("Add an alert policy or disable burn-rate alerts.");
  const spec: SLODefinition["spec"] = {
    ...base.spec,
    quality: d.quality,
    errorBudgetPolicy: d.errorBudgetPolicy,
    lifecycle:
      d.lifecycle === "committed" && !base.spec.lifecycle
        ? undefined
        : d.lifecycle,
    displayName: d.displayName,
    service: d.service,
    target: Number(d.target),
    windowSeconds: durationSeconds(d.window),
    stepSeconds: durationSeconds(d.step),
    providerWindow:
      base.spec.measurement && !d.customMeasurement
        ? undefined
        : d.providerWindow
          ? {
              goodSelector: d.goodQuery,
              totalSelector: d.totalQuery,
              scrapeIntervalSeconds: Number(d.scrape),
            }
          : undefined,
    goodQuery:
      d.providerWindow || (base.spec.measurement && !d.customMeasurement)
        ? undefined
        : d.goodQuery || undefined,
    totalQuery:
      d.providerWindow || (base.spec.measurement && !d.customMeasurement)
        ? undefined
        : d.totalQuery || undefined,
    source: d.source || undefined,
    queryLanguage: d.queryLanguage || undefined,
    policy: undefined,
    policies: d.enabled ? d.policies : undefined,
  };
  if (
    d.enabled &&
    base.spec.policy &&
    !base.spec.policies &&
    JSON.stringify(d.policies) ===
      JSON.stringify([
        {
          ...base.spec.policy,
          name: base.spec.policy.name || "primary",
          severity: base.spec.policy.severity || "critical",
        },
      ])
  ) {
    spec.policy = base.spec.policy;
    spec.policies = undefined;
  }
  if (d.customMeasurement) {
    spec.measurement = undefined;
    spec.measurementBinding = undefined;
    spec.intent = undefined;
  }
  if (base.spec.intent && !d.customMeasurement)
    spec.intent = {
      ...base.spec.intent,
      lifecycle: spec.lifecycle,
      quality: spec.quality,
      errorBudgetPolicy: spec.errorBudgetPolicy,
      service: spec.service,
      source: spec.source ?? "",
      target: spec.target,
      windowSeconds: spec.windowSeconds,
      policy: spec.policy,
      policies: spec.policies,
    };
  return {
    ...base,
    metadata: { ...base.metadata, name: d.name, project: d.project },
    spec,
  };
}
export function ObjectiveCreate({
  scope,
  onCreated,
  resource,
}: {
  scope: RequestScope;
  onCreated: () => void;
  resource?: SLO;
}) {
  const [measurementFooter, setMeasurementFooter] =
    useState<HTMLDivElement | null>(null);
  const [definition, setDefinition] = useState<SLODefinition>(
    newObjectiveDefinition,
  );
  const [measured, setMeasured] = useState(() =>
    inlineObjectiveDraft(newObjectiveDefinition()),
  );
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState("settings");
  const [text, setText] = useState("");
  const [draft, setDraft] = useState<Draft | null>(null);
  const [connections, setConnections] = useState<
    { id: string; kind: string }[]
  >([]);
  const [error, setError] = useState("");
  const [measurementAck, setMeasurementAck] = useState(false);
  const [busy, setBusy] = useState(false);
  const [plan, setPlan] = useState<{
    measurementChanged?: boolean;
    warnings?: string[];
    action: string;
    expectedRevision: number;
    digest: string;
    manifest: SLODefinition;
  } | null>(null);
  const reviewRef = useRef<HTMLDivElement>(null);
  const errorRef = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    (error ? errorRef.current : reviewRef.current)?.scrollIntoView?.({
      block: "nearest",
    });
  }, [plan, error]);
  useEffect(() => {
    setOpen(false);
    setPlan(null);
    setBusy(false);
  }, [scope, resource?.id]);
  function changeOpen(value: boolean) {
    if (busy) return;
    setOpen(value);
    if (value) {
      const d = resource
        ? definitionFromSLO(resource)
        : newObjectiveDefinition();
      setDefinition(d);
      setMeasured(
        inlineObjectiveDraft(d.spec.measurement ? d : newObjectiveDefinition()),
      );
      setText(stringify(d));
      setDraft(
        objectiveDraft(
          d,
          d.spec.measurement ? undefined : resource?.providerWindow,
        ),
      );
      setMode(resource && !resource.measurement ? "settings" : "measurement");
      setPlan(null);
      setError("");
      setMeasurementAck(false);
    }
  }
  const request = useRef(new AbortController());
  useEffect(() => {
    const c = new AbortController();
    request.current = c;
    if (open) {
      setConnections([]);
      api<{ connections: { id: string; kind: string }[] }>(
        "/api/connections",
        scope,
        { signal: c.signal },
      )
        .then((r) => {
          if (!c.signal.aborted) setConnections(r.connections);
        })
        .catch((e) => {
          if (!c.signal.aborted) setError(message(e));
        });
    }
    return () => c.abort();
  }, [open, scope]);
  function document() {
    const base = mode === "yaml" ? parseObjectiveDefinition(text) : definition;
    const current =
      mode === "measurement"
        ? inlineObjectiveDefinition(measured, false)
        : mode === "settings" && draft
          ? objectiveDefinition(base, draft)
          : base;
    if (resource && current.metadata.name !== resource.id)
      throw Error(
        "This editor keeps the existing SLO identifier. Use Define objective to create a separate SLO.",
      );
    return current;
  }
  function invalidate() {
    setPlan(null);
    setError("");
    setMeasurementAck(false);
  }
  function changeMode(value: string) {
    try {
      const current = document();
      if (value === "measurement") setMeasured(inlineObjectiveDraft(current));
      if (value === "settings")
        setDraft(
          objectiveDraft(
            current,
            current.spec.measurement ? undefined : resource?.providerWindow,
          ),
        );
      setDefinition(current);
      setText(stringify(current));
      setMode(value);
      invalidate();
    } catch (e) {
      setError(message(e));
      setPlan(null);
      setMeasurementAck(false);
    }
  }
  function setField<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft((d) => (d ? { ...d, [key]: value } : d));
    setPlan(null);
    setError("");
    setMeasurementAck(false);
  }
  async function review() {
    const c = request.current;
    setBusy(true);
    setPlan(null);
    setError("");
    try {
      const body = stringify(document());
      const result = await api<{
        measurementChanged?: boolean;
        warnings?: string[];
        action: string;
        expectedRevision: number;
        digest: string;
        manifest: SLODefinition;
      }>("/api/resources/plan", scope, {
        method: "POST",
        headers: { "Content-Type": "application/yaml" },
        body,
        signal: c.signal,
      });
      if (c.signal.aborted) return;
      if (result.manifest.kind !== "ServiceLevelObjective")
        throw new Error("Expected a ServiceLevelObjective.");
      setText(body);
      setPlan(result);
      setMeasurementAck(false);
    } catch (e) {
      if (!c.signal.aborted) {
        setError(message(e));
        setPlan(null);
      }
    } finally {
      if (!c.signal.aborted) setBusy(false);
    }
  }
  async function apply() {
    if (
      !canEdit(scope) ||
      !plan ||
      (plan.measurementChanged && !measurementAck)
    )
      return;
    const c = request.current;
    setBusy(true);
    setError("");
    try {
      await api("/api/resources/apply", scope, {
        method: "POST",
        headers: {
          "Content-Type": "application/yaml",
          "If-Match": `"${plan.expectedRevision}"`,
        },
        body: stringify(plan.manifest),
        signal: c.signal,
      });
      if (c.signal.aborted) return;
      setOpen(false);
      onCreated();
    } catch (e) {
      if (!c.signal.aborted) {
        setError(message(e));
        setPlan(null);
      }
    } finally {
      if (!c.signal.aborted) setBusy(false);
    }
  }
  const fields: {
    key: keyof Draft;
    label: string;
    type?: string;
    fixed?: boolean;
  }[] = [
    { key: "name", label: "Identifier", fixed: true },
    { key: "project", label: "Project" },
    { key: "displayName", label: "Display name" },
    { key: "service", label: "Service" },
    { key: "target", label: "SLO target (fraction)", type: "number" },
    { key: "window", label: "Rolling SLO window (e.g. 28d)" },
    { key: "step", label: "Measurement interval (e.g. 1m)" },
  ];
  const measurementLocked =
    !!(definition.spec.measurementBinding || definition.spec.measurement) &&
    !draft?.customMeasurement;
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button
          size="sm"
          variant={resource ? "outline" : "default"}
          disabled={!canEdit(scope)}
        >
          {resource ? (
            <Settings2 className="size-4" />
          ) : (
            <Plus className="size-4" />
          )}
          {resource ? "Edit definition" : "Define objective"}
        </Button>
      </DialogTrigger>
      <DialogContent className="definition-dialog sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {resource
              ? "Edit service level objective"
              : "Define service level objective"}
          </DialogTitle>
          <DialogDescription>
            {resource
              ? "Edit the objective, measurement and burn-rate policies, then review the new revision. Measurement changes archive existing evidence and require fresh coverage. Rule delivery follows the telemetry connection policy."
              : "Define what counts as success, choose a target and window, then review before applying. Use advanced queries or YAML for custom measurements."}
          </DialogDescription>
        </DialogHeader>
        <div className="dialog-body space-y-5">
          <Tabs value={mode} onValueChange={changeMode}>
            <TabsList>
              <TabsTrigger
                value="measurement"
                disabled={busy || !definition.spec.measurement}
                title={
                  !definition.spec.measurement
                    ? "Custom queries and shared measurements are edited in Settings or YAML."
                    : undefined
                }
              >
                Measurement
              </TabsTrigger>
              <TabsTrigger value="settings" disabled={busy}>
                {resource || definition.spec.measurement
                  ? "Settings"
                  : "Advanced queries"}
              </TabsTrigger>
              <TabsTrigger value="yaml" disabled={busy}>
                YAML
              </TabsTrigger>
            </TabsList>
            {definition.spec.measurement ? (
              <TabsContent value="measurement">
                <MeasuredObjective
                  footer={measurementFooter}
                  value={measured}
                  onChange={(value) => {
                    setMeasured(value);
                    invalidate();
                  }}
                  fixedName={!!resource}
                  connections={connections}
                  scope={scope}
                  onBusyChange={setBusy}
                  onCreated={() => {
                    setOpen(false);
                    onCreated();
                  }}
                />
              </TabsContent>
            ) : null}
            <TabsContent value="settings" className="space-y-5">
              {draft ? (
                <>
                  {definition.spec.measurementBinding ||
                  definition.spec.measurement ? (
                    <div className="space-y-2 rounded-md border p-3 text-sm">
                      <p>
                        {definition.spec.measurement
                          ? "This measurement is owned by the SLO. Edit it in the Measurement tab, or switch to custom queries below."
                          : `Shared measurement: ${definition.spec.measurementBinding?.name} · revision ${definition.spec.measurementBinding?.revision}. Update the reference in YAML or use custom queries.`}
                      </p>
                      <Button
                        variant="outline"
                        disabled={busy}
                        onClick={() =>
                          setField(
                            "customMeasurement",
                            !draft.customMeasurement,
                          )
                        }
                      >
                        {draft.customMeasurement
                          ? "Keep saved measurement"
                          : "Use custom queries"}
                      </Button>
                      {draft.customMeasurement ? (
                        <p className="text-xs text-muted-foreground">
                          The next reviewed revision removes the saved
                          measurement reference and intent provenance. Previous
                          revisions retain both.
                        </p>
                      ) : null}
                    </div>
                  ) : null}
                  <BudgetPolicyEditor
                    value={draft.errorBudgetPolicy}
                    disabled={busy}
                    onChange={(value) => setField("errorBudgetPolicy", value)}
                  />
                  <QualityPolicyEditor
                    value={draft.quality}
                    disabled={busy}
                    onChange={(value) => setField("quality", value)}
                  />
                  <SLOLifecycleSelect
                    value={draft.lifecycle}
                    disabled={busy}
                    onChange={(value) => setField("lifecycle", value)}
                  />
                  <div className="form-grid">
                    {fields.map((f) => (
                      <div key={f.key}>
                        <FormField htmlFor={`slo-${f.key}`} label={f.label}>
                          <Input
                            id={`slo-${f.key}`}
                            type={f.type ?? "text"}
                            step="any"
                            value={String(draft[f.key])}
                            disabled={
                              busy ||
                              (!!resource && f.fixed) ||
                              (measurementLocked &&
                                (f.key === "service" || f.key === "step"))
                            }
                            onChange={(e) => setField(f.key, e.target.value)}
                          />
                        </FormField>
                      </div>
                    ))}
                    <div>
                      <FormField
                        htmlFor="slo-source"
                        label="Metrics connection"
                      >
                        <select
                          id="slo-source"
                          className="h-9 w-full rounded-md border bg-white px-3 text-sm"
                          value={draft.source === "primary" ? "" : draft.source}
                          disabled={busy || measurementLocked}
                          onChange={(e) => setField("source", e.target.value)}
                        >
                          <option value="">
                            {connections.some((c) => c.id === "primary")
                              ? `Primary · ${providerLabel(connections.find((c) => c.id === "primary")!.kind)}`
                              : "Primary (not configured)"}
                          </option>
                          {connections
                            .filter((c) => c.id !== "primary")
                            .map((c) => (
                              <option key={c.id} value={c.id}>
                                {c.id} · {providerLabel(c.kind)}
                              </option>
                            ))}
                        </select>
                      </FormField>
                    </div>
                  </div>
                  {!measurementLocked ? (
                    <div className="space-y-2">
                      <FormField
                        htmlFor="slo-query-language"
                        label="Query language"
                      >
                        <select
                          id="slo-query-language"
                          className="h-9 w-full rounded-md border bg-background px-3 text-sm"
                          value={draft.queryLanguage}
                          disabled={busy}
                          onChange={(e) =>
                            setField("queryLanguage", e.target.value)
                          }
                        >
                          <option value="">PromQL (default)</option>
                          <option value="promql">PromQL</option>
                          <option value="datadog">Datadog count query</option>
                          <option value="nrql">NRQL event count</option>
                          <option value="metricsql">MetricsQL</option>
                          <option value="googlecloud">
                            Google Cloud Monitoring filter
                          </option>
                        </select>
                      </FormField>
                      <p className="text-xs text-muted-foreground">
                        Use the query language supported by the selected
                        connection. Changing it does not translate your queries.
                        Provider compatibility is checked during review.
                      </p>
                    </div>
                  ) : null}
                  <div className="grid gap-3">
                    <div>
                      <FormField
                        htmlFor="slo-good-query"
                        label={
                          <>
                            {" "}
                            {draft.providerWindow
                              ? "Good-event counter selector"
                              : "Good interval-count query"}{" "}
                          </>
                        }
                      >
                        <Textarea
                          id="slo-good-query"
                          className="font-mono text-xs"
                          value={draft.goodQuery}
                          disabled={busy || measurementLocked}
                          onChange={(e) =>
                            setField("goodQuery", e.target.value)
                          }
                        />
                      </FormField>
                    </div>
                    <div>
                      <FormField
                        htmlFor="slo-total-query"
                        label={
                          <>
                            {" "}
                            {draft.providerWindow
                              ? "Total-event counter selector"
                              : "Total interval-count query"}{" "}
                          </>
                        }
                      >
                        <Textarea
                          id="slo-total-query"
                          className="font-mono text-xs"
                          value={draft.totalQuery}
                          disabled={busy || measurementLocked}
                          onChange={(e) =>
                            setField("totalQuery", e.target.value)
                          }
                        />
                      </FormField>
                    </div>
                  </div>
                  {draft.providerWindow ? (
                    <div>
                      <FormField
                        htmlFor="slo-scrape"
                        label="Source scrape interval (seconds)"
                      >
                        <Input
                          id="slo-scrape"
                          type="number"
                          min="1"
                          value={draft.scrape}
                          disabled={busy || measurementLocked}
                          onChange={(e) => setField("scrape", e.target.value)}
                        />
                      </FormField>
                      <p className="mt-1 text-xs text-muted-foreground">
                        Match the source scrape configuration. Each full SLO and
                        burn window is evaluated independently.
                      </p>
                    </div>
                  ) : null}
                  <div className="border-t pt-4">
                    <Label className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={draft.enabled}
                        disabled={busy}
                        onChange={(e) => setField("enabled", e.target.checked)}
                      />
                      Burn-rate alert policies
                    </Label>
                    {draft.enabled ? (
                      <PolicyEditor
                        policies={draft.policies}
                        onChange={(value) => setField("policies", value)}
                        disabled={busy}
                      />
                    ) : null}
                  </div>
                </>
              ) : null}
            </TabsContent>
            <TabsContent value="yaml">
              <FormField
                htmlFor="slo-config"
                label="ServiceLevelObjective manifest (YAML)"
              >
                <Textarea
                  id="slo-config"
                  disabled={busy}
                  value={text}
                  onChange={(e) => {
                    setText(e.target.value);
                    setPlan(null);
                    setError("");
                    setMeasurementAck(false);
                  }}
                  className="min-h-80 font-mono text-xs"
                  spellCheck={false}
                />
              </FormField>
            </TabsContent>
          </Tabs>
          {error ? (
            <p ref={errorRef} role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          {mode !== "measurement" && plan ? (
            <div
              ref={reviewRef}
              className="rounded-md border bg-slate-50 p-3 text-sm"
            >
              {plan.measurementChanged ? (
                <div className="mb-3 space-y-2 rounded-md border p-3">
                  {(plan.warnings ?? []).map((warning) => (
                    <p key={warning} className="text-sm">
                      {warning}
                    </p>
                  ))}
                  <label className="flex items-start gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={measurementAck}
                      onChange={(e) => setMeasurementAck(e.target.checked)}
                      disabled={busy}
                    />
                    I reviewed the measurement change and understand that active
                    coverage starts fresh.
                  </label>
                </div>
              ) : null}
              <p>
                Validated · {plan.action} · revision {plan.expectedRevision} →{" "}
                {plan.action === "unchanged"
                  ? plan.expectedRevision
                  : plan.expectedRevision + 1}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                Source: {plan.manifest.spec.source || "primary"} · SLO window{" "}
                {duration(plan.manifest.spec.windowSeconds)}
              </p>
              <details className="mt-2">
                <summary className="cursor-pointer text-xs">
                  Review resulting YAML
                </summary>
                <pre className="code-block mt-2">
                  {stringify(plan.manifest)}
                </pre>
              </details>
              <p className="mt-2 break-all font-mono text-xs">
                SHA-256 {plan.digest}
              </p>
            </div>
          ) : null}
          {resource ? (
            <SLORevisionHistory scope={scope} id={resource.id} />
          ) : null}
        </div>
        {mode === "measurement" ? (
          <div ref={setMeasurementFooter} />
        ) : (
          <div className="dialog-actions">
            <Button
              variant="outline"
              disabled={busy || !text.trim()}
              onClick={review}
            >
              {busy ? "Working…" : "Validate and plan"}
            </Button>
            <Button
              disabled={
                busy ||
                !plan ||
                !canEdit(scope) ||
                (!!plan.measurementChanged && !measurementAck)
              }
              onClick={apply}
            >
              {plan?.action === "unchanged"
                ? "Keep current definition"
                : "Apply definition"}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
