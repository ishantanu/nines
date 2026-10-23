import { SLORevisionHistory } from "./slo-revision-history";
import { MeasuredObjective } from "./measured-objective";
import { PolicyEditor } from "./policy-editor";
import { useEffect, useState } from "react";
import { parse, stringify } from "yaml";
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
  customMeasurement: boolean;
  policies: NonNullable<SLO["policies"]>;
  providerWindow: boolean;
  scrape: string;
  name: string;
  project: string;
  displayName: string;
  service: string;
  source: string;
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
    customMeasurement: false,
    providerWindow: !!window,
    scrape: String(window?.scrapeIntervalSeconds ?? 15),
    name: d.metadata.name,
    project: d.metadata.project,
    displayName: d.spec.displayName,
    service: d.spec.service,
    source: d.spec.source ?? "",
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
  const spec = {
    ...base.spec,
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
    policy: undefined,
    policies: d.enabled ? d.policies : undefined,
  };
  if (d.customMeasurement) {
    spec.measurement = undefined;
    spec.measurementBinding = undefined;
    spec.intent = undefined;
  }
  if (base.spec.intent && !d.customMeasurement)
    spec.intent = {
      ...base.spec.intent,
      service: spec.service,
      source: spec.source ?? "",
      target: spec.target,
      windowSeconds: spec.windowSeconds,
      policy: spec.policy,
      policies: spec.policies,
    };
  return { ...base, metadata: { name: d.name, project: d.project }, spec };
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
  useEffect(() => {
    setOpen(false);
    setPlan(null);
    setBusy(false);
  }, [scope]);
  function changeOpen(value: boolean) {
    if (busy) return;
    setOpen(value);
    if (value) {
      const d = definitionFromSLO(
        resource ?? {
          id: "my-objective",
          project: "production",
          service: "api",
          name: "API availability",
          target: 0.999,
          windowSeconds: 86400,
          stepSeconds: 60,
          providerWindow: {
            goodSelector: 'http_requests_total{code="200"}',
            totalSelector: "http_requests_total",
            scrapeIntervalSeconds: 15,
          },
          policies: [
            {
              name: "fast-burn",
              severity: "critical",
              shortWindowSeconds: 300,
              longWindowSeconds: 3600,
              burnRate: 14.4,
              forSeconds: 120,
              minCoverage: 1,
            },
            {
              name: "sustained-burn",
              severity: "warning",
              shortWindowSeconds: 1800,
              longWindowSeconds: 21600,
              burnRate: 6,
              forSeconds: 300,
              minCoverage: 1,
            },
          ],
        },
      );
      setText(stringify(d));
      setDraft(objectiveDraft(d, resource?.providerWindow));
      setMode(resource && !resource.measurement ? "settings" : "measurement");
      setPlan(null);
      setError("");
      setMeasurementAck(false);
      api<{ connections: { id: string; kind: string }[] }>(
        "/api/connections",
        scope,
      )
        .then((r) => setConnections(r.connections))
        .catch(() => setConnections([]));
    }
  }
  function document() {
    const d = parse(text) as SLODefinition;
    if (d.kind !== "ServiceLevelObjective")
      throw new Error("Expected a ServiceLevelObjective YAML definition.");
    return mode === "settings" && draft ? objectiveDefinition(d, draft) : d;
  }
  function setField<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft((d) => (d ? { ...d, [key]: value } : d));
    setPlan(null);
    setError("");
    setMeasurementAck(false);
  }
  async function review() {
    setBusy(true);
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
      });
      if (result.manifest.kind !== "ServiceLevelObjective")
        throw new Error("Expected a ServiceLevelObjective.");
      setText(body);
      setPlan(result);
      setMeasurementAck(false);
    } catch (e) {
      setError(message(e));
      setPlan(null);
    } finally {
      setBusy(false);
    }
  }
  async function apply() {
    if (!plan || (plan.measurementChanged && !measurementAck)) return;
    setBusy(true);
    setError("");
    try {
      await api("/api/resources/apply", scope, {
        method: "POST",
        headers: {
          "Content-Type": "application/yaml",
          "If-Match": `"${plan.expectedRevision}"`,
        },
        body: text,
      });
      setOpen(false);
      onCreated();
    } catch (e) {
      setError(message(e));
      setPlan(null);
    } finally {
      setBusy(false);
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
    !!(resource?.measurementBinding || resource?.measurement) &&
    !draft?.customMeasurement;
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant={resource ? "outline" : "default"}>
          {resource ? (
            <Settings2 className="size-4" />
          ) : (
            <Plus className="size-4" />
          )}
          {resource ? "Edit definition" : "Define objective"}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {resource
              ? "Edit service level objective"
              : "Define service level objective"}
          </DialogTitle>
          <DialogDescription>
            {resource
              ? "Edit the objective, measurement and burn-rate policies, then review the new revision. Measurement changes archive existing evidence and require fresh coverage. Rule delivery follows the data source policy."
              : "Define what counts as success, choose a target and window, then review before applying. Use advanced queries or YAML for custom measurements."}
          </DialogDescription>
        </DialogHeader>
        <Tabs
          value={mode}
          onValueChange={(value) => {
            try {
              if (value === "measurement" || mode === "measurement") {
                if (mode === "settings") setText(stringify(document()));
                if (value === "settings") {
                  const d = parse(text) as SLODefinition;
                  if (d.kind !== "ServiceLevelObjective")
                    throw new Error("Expected a ServiceLevelObjective.");
                  setDraft(objectiveDraft(d, resource?.providerWindow));
                }
                setMode(value);
                setPlan(null);
                setError("");
                return;
              }
              if (value === "yaml") {
                setText(stringify(document()));
              } else {
                const d = parse(text) as SLODefinition;
                if (d.kind !== "ServiceLevelObjective")
                  throw new Error("Expected a ServiceLevelObjective.");
                setDraft(objectiveDraft(d, resource?.providerWindow));
              }
              setMode(value);
              setPlan(null);
              setError("");
            } catch (e) {
              setError(message(e));
            }
          }}
        >
          <TabsList>
            {!resource || resource.measurement ? (
              <TabsTrigger value="measurement" disabled={busy}>
                Measurement
              </TabsTrigger>
            ) : null}
            <TabsTrigger value="settings" disabled={busy}>
              {resource ? "Settings" : "Advanced queries"}
            </TabsTrigger>
            <TabsTrigger value="yaml" disabled={busy}>
              YAML
            </TabsTrigger>
          </TabsList>
          {!resource || resource.measurement ? (
            <TabsContent value="measurement">
              <MeasuredObjective
                resource={resource}
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
                {resource?.measurementBinding || resource?.measurement ? (
                  <div className="space-y-2 rounded-md border p-3 text-sm">
                    <p>
                      {resource.measurement
                        ? "This measurement is owned by the SLO. Edit it in the Measurement tab, or switch to custom queries below."
                        : `Shared measurement: ${resource.measurementBinding?.name} · revision ${resource.measurementBinding?.revision}. Update the reference in YAML or use custom queries.`}
                    </p>
                    <Button
                      variant="outline"
                      disabled={busy}
                      onClick={() =>
                        setField("customMeasurement", !draft.customMeasurement)
                      }
                    >
                      {draft.customMeasurement
                        ? "Keep saved measurement"
                        : "Use custom queries"}
                    </Button>
                    {draft.customMeasurement ? (
                      <p className="text-xs text-muted-foreground">
                        The next reviewed revision removes the saved measurement
                        reference and intent provenance. Previous revisions
                        retain both.
                      </p>
                    ) : null}
                  </div>
                ) : null}
                <div className="grid gap-3 sm:grid-cols-2">
                  {fields.map((f) => (
                    <div key={f.key}>
                      <Label htmlFor={`slo-${f.key}`}>{f.label}</Label>
                      <Input
                        id={`slo-${f.key}`}
                        className="mt-1"
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
                    </div>
                  ))}
                  <div>
                    <Label htmlFor="slo-source">Metrics connection</Label>
                    <select
                      id="slo-source"
                      className="mt-1 h-9 w-full rounded-md border bg-white px-3 text-sm"
                      value={draft.source === "primary" ? "" : draft.source}
                      disabled={busy || measurementLocked}
                      onChange={(e) => setField("source", e.target.value)}
                    >
                      <option value="">Primary (default)</option>
                      {connections
                        .filter((c) => c.id !== "primary")
                        .map((c) => (
                          <option key={c.id} value={c.id}>
                            {c.id} · {c.kind}
                          </option>
                        ))}
                    </select>
                  </div>
                </div>
                <div className="grid gap-3">
                  <div>
                    <Label htmlFor="slo-good-query">
                      {draft.providerWindow
                        ? "Good-event counter selector"
                        : "Good interval-count query"}
                    </Label>
                    <Textarea
                      id="slo-good-query"
                      className="mt-1 font-mono text-xs"
                      value={draft.goodQuery}
                      disabled={busy || measurementLocked}
                      onChange={(e) => setField("goodQuery", e.target.value)}
                    />
                  </div>
                  <div>
                    <Label htmlFor="slo-total-query">
                      {draft.providerWindow
                        ? "Total-event counter selector"
                        : "Total interval-count query"}
                    </Label>
                    <Textarea
                      id="slo-total-query"
                      className="mt-1 font-mono text-xs"
                      value={draft.totalQuery}
                      disabled={busy || measurementLocked}
                      onChange={(e) => setField("totalQuery", e.target.value)}
                    />
                  </div>
                </div>
                {draft.providerWindow ? (
                  <div>
                    <Label htmlFor="slo-scrape">
                      Source scrape interval (seconds)
                    </Label>
                    <Input
                      id="slo-scrape"
                      type="number"
                      min="1"
                      className="mt-1"
                      value={draft.scrape}
                      disabled={busy || measurementLocked}
                      onChange={(e) => setField("scrape", e.target.value)}
                    />
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
            <Label htmlFor="slo-config">
              ServiceLevelObjective manifest (YAML)
            </Label>
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
              className="mt-2 min-h-80 font-mono text-xs"
              spellCheck={false}
            />
          </TabsContent>
        </Tabs>
        {mode !== "measurement" && error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {mode !== "measurement" && plan ? (
          <div className="rounded-md border bg-slate-50 p-3 text-sm">
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
              <pre className="code-block mt-2">{stringify(plan.manifest)}</pre>
            </details>
            <p className="mt-2 break-all font-mono text-xs">
              SHA-256 {plan.digest}
            </p>
          </div>
        ) : null}
        {resource ? (
          <SLORevisionHistory scope={scope} id={resource.id} />
        ) : null}
        {mode !== "measurement" ? (
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
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
