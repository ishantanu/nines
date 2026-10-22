import type { IntentPlan } from "@/lib/intent";
import type { FlowRun } from "@/lib/intent-conversations";

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
  const spec = compiled.definition.spec;
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
        <div>
          <dt className="text-muted-foreground">Selected binding</dt>
          <dd className="mt-1 break-all">
            {compiled.binding.name} · revision {compiled.binding.revision}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Observation point</dt>
          <dd className="mt-1">{spec.observationPoint}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Measurement population</dt>
          <dd className="mt-1 break-all font-mono">
            {JSON.stringify(spec.labels)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Success condition</dt>
          <dd className="mt-1 break-all">
            {spec.measurement === "latency"
              ? `Attempt duration at most ${spec.thresholdSeconds} seconds`
              : JSON.stringify(spec.successLabels ?? {})}
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
            <dt>Binding digest</dt>
            <dd>{compiled.binding.digest ?? "Unavailable"}</dd>
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
