import { useEffect, useRef, useState } from "react";
import { stringify } from "yaml";
import {
  api,
  canEdit,
  message,
  type RequestScope,
  type BurnRatePolicy,
  type SLO,
} from "@/lib/api";
import { bindingStarter, type BindingManifest } from "@/lib/topology";
import {
  measurementDraft,
  sloFromMeasurementDraft,
  prepareInlineObjective,
  type MeasurementDraft,
} from "@/lib/measurement";
import { duration } from "@/lib/history";
import { MeasurementFields } from "./measurement-editor";
import { BudgetPolicyEditor } from "./budget-policy-editor";
import { QualityPolicyEditor } from "./quality-policy-editor";
import { SLOLifecycleSelect } from "./slo-lifecycle";
import { PolicyEditor } from "./policy-editor";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
export function MeasurementSummary({
  definition,
}: {
  definition: BindingManifest;
}) {
  const s = definition.spec;
  return (
    <div className="space-y-2 rounded-md border bg-muted/30 p-3 text-sm">
      <p className="font-medium">{s.displayName}</p>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-xs">
        <dt className="text-muted-foreground">Service / project</dt>
        <dd className="break-words">
          {s.service} / {definition.metadata.project}
        </dd>
        <dt className="text-muted-foreground">Telemetry connection</dt>
        <dd className="break-words">{s.source}</dd>
        <dt className="text-muted-foreground">Metric</dt>
        <dd className="break-all font-mono">{s.metric}</dd>
        <dt className="text-muted-foreground">Eligible events</dt>
        <dd className="break-words">
          {Object.entries(s.labels)
            .map(([k, v]) => `${k} = ${JSON.stringify(v)}`)
            .join(", ") || "All series in this metric family"}
        </dd>
        <dt className="text-muted-foreground">Good events</dt>
        <dd className="break-words">
          {s.measurement === "latency"
            ? `Duration at most ${s.thresholdSeconds} seconds`
            : Object.entries(s.successLabels ?? {})
                .map(([k, v]) => `${k} = ${JSON.stringify(v)}`)
                .join(", ")}
        </dd>
        <dt className="text-muted-foreground">Measurement / scrape</dt>
        <dd>
          {s.stepSeconds}s / {s.scrapeIntervalSeconds ?? "unspecified"}s
        </dd>
      </dl>
      <p className="text-xs text-muted-foreground">{s.observationPoint}</p>
    </div>
  );
}

export function MeasuredObjective({
  scope,
  onCreated,
  onBusyChange,
  resource,
}: {
  scope: RequestScope;
  onCreated: () => void;
  onBusyChange: (v: boolean) => void;
  resource?: SLO;
}) {
  const [draft, setDraft] = useState<MeasurementDraft>(() => {
    const initial = measurementDraft();
    if (!resource?.measurement)
      return {
        ...initial,
        name: "",
        displayName: "",
        service: "",
        metric: "",
        observationPoint: "",
      };
    const m = resource.measurement;
    return measurementDraft({
      ...bindingStarter(resource.source || "primary", m.metric, m.labels),
      metadata: { name: resource.id, project: resource.project },
      spec: {
        ...m,
        measurement: m.type,
        displayName: resource.name,
        source: resource.source || "primary",
        service: resource.service,
        stepSeconds: resource.stepSeconds,
      },
    });
  });
  const [errorBudgetPolicy, setErrorBudgetPolicy] = useState<
    SLO["errorBudgetPolicy"]
  >(resource?.errorBudgetPolicy);
  const [quality, setQuality] = useState<SLO["quality"]>(resource?.quality);
  const [lifecycle, setLifecycle] = useState<NonNullable<SLO["lifecycle"]>>(
    resource?.lifecycle ?? "committed",
  );
  const [target, setTarget] = useState(
    String((resource?.target ?? 0.999) * 100),
  );
  const [window, setWindow] = useState(
    resource ? duration(resource.windowSeconds) : "28d",
  );
  const [policies, setPolicies] = useState<BurnRatePolicy[]>(
    resource?.policies ??
      (resource?.policy
        ? [
            {
              ...resource.policy,
              name: resource.policy.name || "default",
              severity: resource.policy.severity || "critical",
            },
          ]
        : []),
  );
  const [connections, setConnections] = useState<
    { id: string; kind: string }[]
  >([]);
  const [preview, setPreview] = useState<Awaited<
    ReturnType<typeof prepareInlineObjective>
  > | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [ack, setAck] = useState(false);
  const controller = useRef(new AbortController());
  useEffect(() => {
    const c = new AbortController();
    controller.current = c;
    api<{ connections: { id: string; kind: string }[] }>(
      "/api/connections",
      scope,
      { signal: c.signal },
    )
      .then((r) => setConnections(r.connections))
      .catch((e) => {
        if (!c.signal.aborted) setError(message(e));
      });
    return () => c.abort();
  }, [scope]);
  function change(action: () => void) {
    action();
    setPreview(null);
    setAck(false);
    setError("");
  }
  function working(v: boolean) {
    setBusy(v);
    onBusyChange(v);
  }
  async function prepare() {
    working(true);
    setError("");
    setPreview(null);
    try {
      const manifest = sloFromMeasurementDraft(draft, target, window, policies);
      manifest.spec.quality = quality;
      manifest.spec.errorBudgetPolicy = errorBudgetPolicy;
      if (lifecycle !== "committed" || resource?.lifecycle)
        manifest.spec.lifecycle = lifecycle;
      const p = await prepareInlineObjective(
        scope,
        manifest,
        controller.current.signal,
      );
      if (!controller.current.signal.aborted && !scope.signal.aborted)
        setPreview(p);
    } catch (e) {
      if (!controller.current.signal.aborted) setError(message(e));
    } finally {
      working(false);
    }
  }
  async function apply() {
    if (!preview || !ack) return;
    working(true);
    setError("");
    try {
      await api("/api/resources/apply", scope, {
        method: "POST",
        headers: {
          "Content-Type": "application/yaml",
          "If-Match": `"${preview.plan.expectedRevision}"`,
        },
        signal: controller.current.signal,
        body: stringify(preview.plan.manifest),
      });
      if (!controller.current.signal.aborted && !scope.signal.aborted)
        onCreated();
    } catch (e) {
      if (!controller.current.signal.aborted) setError(message(e));
      setPreview(null);
    } finally {
      working(false);
    }
  }
  return (
    <div className="space-y-5">
      <p className="text-sm text-muted-foreground">
        Define what counts as success, then set the target and window. The
        measurement is saved and versioned inside this SLO.
      </p>
      <MeasurementFields
        draft={draft}
        connections={connections}
        disabled={busy || !canEdit(scope)}
        fixedName={!!resource}
        onChange={(value) => change(() => setDraft(value))}
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor="inline-target">Target (%)</Label>
          <Input
            id="inline-target"
            value={target}
            disabled={busy}
            onChange={(e) => change(() => setTarget(e.target.value))}
          />
        </div>
        <div>
          <Label htmlFor="inline-window">Rolling window</Label>
          <Input
            id="inline-window"
            value={window}
            disabled={busy}
            onChange={(e) => change(() => setWindow(e.target.value))}
          />
        </div>
      </div>
      <BudgetPolicyEditor
        value={errorBudgetPolicy}
        disabled={busy}
        onChange={(value) => change(() => setErrorBudgetPolicy(value))}
      />
      <QualityPolicyEditor
        value={quality}
        disabled={busy}
        onChange={(value) => change(() => setQuality(value))}
      />
      <SLOLifecycleSelect
        value={lifecycle}
        disabled={busy}
        onChange={(value) => change(() => setLifecycle(value))}
      />
      <PolicyEditor
        policies={policies}
        disabled={busy}
        onChange={(value) => change(() => setPolicies(value))}
      />
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {preview ? (
        <section className="space-y-3 rounded-md border p-3">
          <p className="font-medium">
            {preview.plan.action} · {preview.plan.manifest.metadata.name} ·
            expected revision {preview.plan.expectedRevision}
          </p>
          <p className="text-xs">
            Telemetry checked {preview.compiled.evidence.from} to{" "}
            {preview.compiled.evidence.to}. No SLO or rules have been saved by
            this preview.
          </p>
          {[...preview.compiled.warnings, ...(preview.plan.warnings ?? [])].map(
            (warning, i) => (
              <p key={i} className="text-xs text-muted-foreground">
                {warning}
              </p>
            ),
          )}
          <details>
            <summary className="cursor-pointer text-sm">
              Review SLO YAML
            </summary>
            <pre className="code-block mt-2">
              {stringify(preview.plan.manifest)}
            </pre>
          </details>
          <label className="flex items-start gap-2 text-sm">
            <input
              type="checkbox"
              checked={ack}
              disabled={busy}
              onChange={(e) => setAck(e.target.checked)}
            />
            I reviewed the measurement, target, window and evidence limitations.
          </label>
        </section>
      ) : null}
      <div className="dialog-actions">
        <Button
          variant="outline"
          disabled={busy || !canEdit(scope)}
          onClick={prepare}
        >
          {busy ? "Checking…" : "Verify telemetry and review SLO"}
        </Button>
        <Button
          disabled={busy || !preview || !ack || !canEdit(scope)}
          onClick={apply}
        >
          Save reviewed SLO
        </Button>
      </div>
    </div>
  );
}
