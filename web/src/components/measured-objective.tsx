import { FormField, FormSection } from "@/components/ui/form-layout";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { stringifyDefinition as stringify } from "@/lib/definition-yaml";
import { api, canEdit, message, type RequestScope } from "@/lib/api";
import { type BindingManifest } from "@/lib/topology";
import { prepareInlineObjective } from "@/lib/measurement";
import {
  inlineObjectiveDefinition,
  inlineObjectiveDraft,
  type InlineObjectiveDraft,
} from "@/lib/objective-editor";
import { SLOTemplatePicker } from "./slo-template-picker";
import { draftFromSLOTemplate } from "@/lib/slo-templates";
import { MeasurementFields } from "./measurement-editor";
import { BudgetPolicyEditor } from "./budget-policy-editor";
import { QualityPolicyEditor } from "./quality-policy-editor";
import { SLOLifecycleSelect } from "./slo-lifecycle";
import { PolicyEditor } from "./policy-editor";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
  value,
  onChange,
  fixedName,
  connections,
  footer,
}: {
  scope: RequestScope;
  onCreated: () => void;
  onBusyChange: (v: boolean) => void;
  value: InlineObjectiveDraft;
  onChange: (value: InlineObjectiveDraft) => void;
  fixedName?: boolean;
  connections: { id: string; kind: string }[];
  footer?: HTMLElement | null;
}) {
  const {
    measurement: draft,
    errorBudgetPolicy,
    quality,
    lifecycle,
    target,
    window,
    policies,
  } = value;
  function update(patch: Partial<InlineObjectiveDraft>) {
    onChange({ ...value, ...patch });
  }
  const [preview, setPreview] = useState<Awaited<
    ReturnType<typeof prepareInlineObjective>
  > | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [ack, setAck] = useState(false);
  const previewRef = useRef<HTMLElement>(null);
  const errorRef = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    (error ? errorRef.current : previewRef.current)?.scrollIntoView?.({
      block: "nearest",
    });
  }, [preview, error]);
  const controller = useRef(new AbortController());
  useEffect(() => {
    const c = new AbortController();
    controller.current = c;
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
    const c = controller.current;
    working(true);
    setError("");
    setPreview(null);
    try {
      const manifest = inlineObjectiveDefinition(value);
      const p = await prepareInlineObjective(scope, manifest, c.signal);
      if (!c.signal.aborted && !scope.signal.aborted) setPreview(p);
    } catch (e) {
      if (!c.signal.aborted) setError(message(e));
    } finally {
      if (!c.signal.aborted) working(false);
    }
  }
  async function apply() {
    if (!preview || !ack || !canEdit(scope)) return;
    const c = controller.current;
    working(true);
    setError("");
    try {
      await api("/api/resources/apply", scope, {
        method: "POST",
        headers: {
          "Content-Type": "application/yaml",
          "If-Match": `"${preview.plan.expectedRevision}"`,
        },
        signal: c.signal,
        body: stringify(preview.plan.manifest),
      });
      if (!c.signal.aborted && !scope.signal.aborted) onCreated();
    } catch (e) {
      if (!c.signal.aborted) setError(message(e));
      setPreview(null);
    } finally {
      if (!c.signal.aborted) working(false);
    }
  }
  const actions = (
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
  );
  return (
    <div className="space-y-5">
      <p className="text-sm text-muted-foreground">
        Define what counts as success, then set the target and window. The
        measurement is saved and versioned inside this SLO.
      </p>
      {!fixedName ? (
        <SLOTemplatePicker
          onBusyChange={working}
          scope={scope}
          disabled={busy || !canEdit(scope)}
          onSelect={(template) =>
            change(() => {
              const next = inlineObjectiveDraft(template.manifest);
              onChange({
                ...next,
                measurement: {
                  ...draftFromSLOTemplate(template),
                  source: draft.source,
                },
              });
            })
          }
        />
      ) : null}
      <FormSection
        title="Measurement"
        description="Define eligible events, success criteria and where they are observed."
      >
        <MeasurementFields
          draft={draft}
          connections={connections}
          disabled={busy || !canEdit(scope)}
          fixedName={fixedName}
          onChange={(value) => change(() => update({ measurement: value }))}
        />
      </FormSection>
      <FormSection
        title="Objective"
        description="Set the reliability target and rolling evaluation window."
      >
        <div className="form-grid">
          <div>
            <FormField htmlFor="inline-target" label="Target (%)">
              <Input
                id="inline-target"
                value={target}
                disabled={busy}
                onChange={(e) =>
                  change(() => update({ target: e.target.value }))
                }
              />
            </FormField>
          </div>
          <div>
            <FormField htmlFor="inline-window" label="Rolling window">
              <Input
                id="inline-window"
                value={window}
                disabled={busy}
                onChange={(e) =>
                  change(() => update({ window: e.target.value }))
                }
              />
            </FormField>
          </div>
        </div>
      </FormSection>
      <BudgetPolicyEditor
        value={errorBudgetPolicy}
        disabled={busy}
        onChange={(value) => change(() => update({ errorBudgetPolicy: value }))}
      />
      <QualityPolicyEditor
        value={quality}
        disabled={busy}
        onChange={(value) => change(() => update({ quality: value }))}
      />
      <SLOLifecycleSelect
        value={lifecycle}
        disabled={busy}
        onChange={(value) => change(() => update({ lifecycle: value }))}
      />
      <PolicyEditor
        policies={policies}
        disabled={busy}
        onChange={(value) => change(() => update({ policies: value }))}
      />
      {error ? (
        <p ref={errorRef} role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {preview ? (
        <section ref={previewRef} className="space-y-3 rounded-md border p-3">
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
      {footer ? createPortal(actions, footer) : actions}
    </div>
  );
}
