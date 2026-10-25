import { percent, type RequestScope } from "@/lib/api";
import type { IntentOutcome } from "@/lib/intent-conversations";
export function IntentOutcomePanel({
  report,
  scope,
  conversation,
}: {
  report?: IntentOutcome;
  scope: RequestScope;
  conversation: string;
}) {
  if (
    !report ||
    report.tenant !== scope.tenant ||
    report.conversation !== conversation ||
    scope.signal.aborted
  )
    return null;
  return (
    <section
      className="space-y-3 rounded-md border p-4"
      aria-label="Post-change verification"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium">Post-change verification</h3>
        <span className="text-xs font-medium">
          {report.state.replaceAll("_", " ")}
        </span>
      </div>
      <p className="text-sm">{report.message}</p>
      <dl className="grid gap-3 text-xs sm:grid-cols-3">
        <div>
          <dt className="text-muted-foreground">Measured SLI</dt>
          <dd>
            {report.reliability == null
              ? "No verified measurement"
              : percent(report.reliability, 3)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Pre-change SLI</dt>
          <dd>
            {report.baselineReliability == null
              ? "Not comparable"
              : percent(report.baselineReliability, 3)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Evidence quality</dt>
          <dd>{report.quality?.state ?? "Awaiting evidence"}</dd>
        </div>
      </dl>
      {(
        [
          ["Rule storage", report.deployment],
          ["Rule evaluation", report.evaluation],
          ["Query output", report.ruleOutput],
        ] as const
      ).map(([label, stage]) =>
        stage ? (
          <p className="text-xs" key={label}>
            <span className="font-medium">{label}: </span>
            {stage.message}
          </p>
        ) : null,
      )}
      <p className="text-xs text-muted-foreground">
        {report.operation} · {report.sloId} · revision {report.revision}.
        Checked {report.checkedAt}. Warmup ends {report.warmupUntil}.
        Observation deadline {report.deadlineAt}.
      </p>
      {report.healthySince ? (
        <p className="text-xs">
          Qualifying observations since {report.healthySince}.
        </p>
      ) : null}
      <details className="text-xs">
        <summary className="cursor-pointer font-medium">
          Verification limits
        </summary>
        <ul className="mt-2 list-disc space-y-1 pl-4 text-muted-foreground">
          {report.limitations.map((x) => (
            <li key={x}>{x}</li>
          ))}
        </ul>
      </details>
      <p className="text-xs text-muted-foreground">
        A separate human review is required for rollback. This observer cannot
        apply changes or restore consumed error budget.
      </p>
    </section>
  );
}
