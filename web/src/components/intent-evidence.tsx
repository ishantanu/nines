import { MeasurementLabels } from "./measurement-labels";
import type { IntentPlan } from "@/lib/intent";
import type {
  FlowRun,
  IntentTemplateSelection,
} from "@/lib/intent-conversations";

export function IntentProgress({
  progress,
}: {
  progress: NonNullable<FlowRun["progress"]>;
}) {
  const phase =
    progress.state === "receiving"
      ? "Receiving model response"
      : progress.state === "received"
        ? "Model response received"
        : "Waiting for model response";
  return (
    <span className="mt-1 block text-xs">
      {phase}
      {progress.bytes > 0 ? ` · ${progress.bytes.toLocaleString()} bytes` : ""}
    </span>
  );
}

export function IntentEvidence({
  compiled,
  sourceDigest,
}: {
  compiled: NonNullable<IntentPlan["compiled"]>;
  sourceDigest?: string;
}) {
  const inline = compiled.manifest?.spec.measurement;
  const spec = inline
    ? {
        ...inline,
        source: compiled.manifest.spec.source,
        measurement: inline.type,
      }
    : compiled.definition?.spec;
  if (!spec)
    return (
      <p role="alert">
        Measurement evidence is unavailable. Verify telemetry again.
      </p>
    );
  const warnings = [
    ...new Set([...compiled.warnings, ...compiled.evidence.warnings]),
  ];
  return (
    <section
      aria-label="Measurement evidence"
      className="space-y-3 rounded-md border p-3 text-xs"
    >
      <h3 className="font-medium">Measurement evidence</h3>
      <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <dt className="text-muted-foreground">Source connection</dt>
          <dd className="mt-1 break-all">{spec.source}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Metric</dt>
          <dd className="mt-1 break-all font-mono">{spec.metric}</dd>
        </div>
        {compiled.binding ? (
          <div>
            <dt className="text-muted-foreground">Shared measurement</dt>
            <dd className="mt-1 break-all">
              {compiled.binding.name} · revision {compiled.binding.revision}
            </dd>
          </div>
        ) : (
          <div>
            <dt className="text-muted-foreground">Versioning</dt>
            <dd className="mt-1">Owned by this SLO</dd>
          </div>
        )}
        <div>
          <dt className="text-muted-foreground">Observation point</dt>
          <dd className="mt-1">{spec.observationPoint}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Measurement population</dt>
          <dd className="mt-1 break-all font-mono">
            <MeasurementLabels labels={spec.labels} />
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Success condition</dt>
          <dd className="mt-1 break-all">
            {spec.measurement === "latency" ? (
              `Attempt duration at most ${spec.thresholdSeconds} seconds`
            ) : (
              <MeasurementLabels
                labels={spec.successLabels}
                empty="No success labels specified"
              />
            )}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Evidence window</dt>
          <dd className="mt-1 break-all">
            {compiled.evidence.from ?? "Start unavailable"} to{" "}
            {compiled.evidence.to}
          </dd>
        </div>
      </dl>
      <details>
        <summary className="cursor-pointer font-medium">
          Evidence fingerprints
        </summary>
        <dl className="mt-2 space-y-2 break-all font-mono">
          <div>
            <dt>Discovery fingerprint</dt>
            <dd>{compiled.evidence.fingerprint}</dd>
          </div>
          <div>
            <dt>Source fingerprint</dt>
            <dd>{sourceDigest ?? "Unavailable"}</dd>
          </div>
          <div>
            <dt>Measurement digest</dt>
            <dd>
              {compiled.measurementDigest ??
                compiled.binding?.digest ??
                "Unavailable"}
            </dd>
          </div>
        </dl>
      </details>
      <div className="space-y-2">
        <p className="font-medium">Review assumptions</p>
        <p>
          Confirm whether retries and failed attempts belong in this population.
          Discovery does not establish end-to-end user success.
        </p>
        {warnings.length ? (
          <ul className="list-disc space-y-1 pl-4 text-muted-foreground">
            {warnings.map((warning) => (
              <li key={warning}>{warning}</li>
            ))}
          </ul>
        ) : null}
      </div>
    </section>
  );
}

export function IntentTemplateEvidence({
  selection,
}: {
  selection: IntentTemplateSelection;
}) {
  return (
    <details className="rounded-md border p-3 text-xs">
      <summary className="cursor-pointer font-medium">
        Measurement template · {selection.templateId}
      </summary>
      <dl className="mt-3 grid gap-2 sm:grid-cols-2">
        <div>
          <dt className="text-muted-foreground">Origin</dt>
          <dd className="mt-1">
            {selection.extension
              ? `${selection.extension.name} · ${selection.extension.version}`
              : "Built-in template"}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Template revision</dt>
          <dd className="mt-1">{selection.templateRevision}</dd>
        </div>
        <div className="min-w-0 sm:col-span-2">
          <dt className="text-muted-foreground">Template digest</dt>
          <dd className="mt-1 break-all font-mono">
            {selection.templateDigest}
          </dd>
        </div>
        {selection.extension ? (
          <div className="min-w-0 sm:col-span-2">
            <dt className="text-muted-foreground">
              Extension artifact SHA-256
            </dt>
            <dd className="mt-1 break-all font-mono">
              {selection.extension.sha256}
            </dd>
          </div>
        ) : null}
      </dl>
      <p className="mt-3 text-muted-foreground">
        This records the recipe used to draft the measurement. Telemetry
        evidence and human confirmation are reviewed separately. The typed goal
        supplies the target, rolling window and alert policies.
      </p>
    </details>
  );
}
