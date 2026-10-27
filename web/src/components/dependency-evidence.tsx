import type { DependencyEdge } from "@/lib/topology";
import { date } from "@/lib/api";
const states: Record<string, string> = {
  fresh: "Fresh sample",
  stale: "Stale sample",
  unknown: "Sample age unverified",
  unavailable: "Source unavailable",
  not_seen: "Not seen in window",
};
export function DependencyEvidence({ edge }: { edge: DependencyEdge }) {
  const evidence = edge.evidence;
  if (!evidence)
    return (
      <span className="text-sm text-muted-foreground">
        No observation configured
      </span>
    );
  const samples = evidence.samples;
  return (
    <div className="min-w-44 space-y-1 text-sm">
      <p
        className={
          evidence.freshness === "stale" || evidence.freshness === "unavailable"
            ? "font-medium text-[var(--status-at-risk)]"
            : "font-medium"
        }
      >
        {states[evidence.freshness] ?? "Unverified evidence"}
      </p>
      <p className="text-muted-foreground">
        {evidence.source}
        {evidence.sampleAgeSeconds != null
          ? ` · ${evidence.sampleAgeSeconds}s old`
          : ""}
      </p>
      {samples?.sampleContinuity != null ? (
        <p>
          {(samples.sampleContinuity * 100).toFixed(1)}% sample continuity ·{" "}
          {samples.series} observed series
        </p>
      ) : null}
      <details className="text-muted-foreground">
        <summary className="cursor-pointer">Evidence details</summary>
        <dl className="mt-2 space-y-1">
          <div>
            <dt className="font-medium">Window</dt>
            <dd>
              {date(evidence.from)} to {date(evidence.to)}
            </dd>
          </div>
          <div>
            <dt className="font-medium">Read at</dt>
            <dd>{date(evidence.readAt)}</dd>
          </div>
          {samples?.lastSampleAt ? (
            <div>
              <dt className="font-medium">Latest observed sample</dt>
              <dd>
                {date(samples.lastSampleAt)} · inspection resolution{" "}
                {samples.resolutionSeconds}s
              </dd>
            </div>
          ) : null}
          <div>
            <dt className="font-medium">Freshness limit</dt>
            <dd>{evidence.maxSampleAgeSeconds}s</dd>
          </div>
          <div>
            <dt className="font-medium">Counter</dt>
            <dd className="break-all font-mono text-xs">{evidence.metric}</dd>
          </div>
          <div>
            <dt className="font-medium">Connection fingerprint</dt>
            <dd className="break-all font-mono text-xs">
              {evidence.sourceDigest}
            </dd>
          </div>
          <div>
            <dt className="font-medium">Coverage limits</dt>
            <dd>
              Continuity covers the counter series present in this window.
              Instrumentation and trace-sampling coverage are unknown. Missing
              evidence does not prove independence or health.
            </dd>
          </div>
        </dl>
      </details>
    </div>
  );
}
