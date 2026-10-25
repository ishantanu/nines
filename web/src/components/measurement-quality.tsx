import { useEffect, useRef, useState } from "react";
import {
  api,
  message,
  percent,
  type MeasurementQuality,
  type RequestScope,
} from "@/lib/api";
import { Button } from "./ui/button";
import { Card, CardHeader, CardTitle, CardContent } from "./ui/card";
export function MeasurementQualityPanel({
  report,
  scope,
  cohortLabels,
}: {
  report: MeasurementQuality;
  scope: RequestScope;
  cohortLabels?: string[];
}) {
  const [inspected, setInspected] = useState<MeasurementQuality>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const request = useRef(new AbortController());
  useEffect(() => {
    request.current.abort();
    request.current = new AbortController();
    setInspected(undefined);
    setBusy(false);
    setError("");
    return () => request.current.abort();
  }, [report.sloId, report.revision, scope]);
  const data = report;
  const cohorts =
    inspected?.sloId === report.sloId &&
    inspected.tenant === report.tenant &&
    inspected.revision === report.revision &&
    inspected.sourceDigest === report.sourceDigest &&
    inspected.definitionDigest === report.definitionDigest
      ? inspected.cohorts
      : undefined;
  async function inspect() {
    const controller = request.current;
    setBusy(true);
    setError("");
    try {
      const next = await api<MeasurementQuality>(
        `/api/slos/${report.sloId}/quality?cohorts=true&expectedRevision=${report.revision}`,
        scope,
        { signal: controller.signal },
      );
      if (!controller.signal.aborted && !scope.signal.aborted) {
        if (
          next.tenant !== scope.tenant ||
          next.sloId !== report.sloId ||
          next.revision !== report.revision
        )
          throw new Error(
            "Quality evidence scope changed; refresh this objective.",
          );
        setInspected(next);
      }
    } catch (e) {
      if (!controller.signal.aborted && !scope.signal.aborted)
        setError(message(e));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  if (report.tenant !== scope.tenant || scope.signal.aborted) return null;
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b p-4">
        <CardTitle className="text-sm">Measurement quality</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 p-4">
        <dl className="grid gap-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground">Eligible events</dt>
            <dd className="tabular-nums">
              {data.total.toLocaleString()} /{" "}
              {data.minimumEvents.toLocaleString()} minimum
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Window coverage</dt>
            <dd>{percent(data.coverage, 2)}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Sample age</dt>
            <dd>
              {data.freshness.ageAtObservationSeconds ?? "Unavailable"} /{" "}
              {data.freshness.staleAfterSeconds}s allowed
            </dd>
          </div>
        </dl>
        {data.sourceFrom && data.sourceTo ? (
          <p className="text-xs text-muted-foreground">
            Verified source span: {new Date(data.sourceFrom).toISOString()} to{" "}
            {new Date(data.sourceTo).toISOString()}.
          </p>
        ) : null}
        <ul className="space-y-2 text-sm">
          {data.checks
            .filter((c) => c.state !== "satisfied")
            .map((c) => (
              <li key={c.code}>
                <span className="font-medium">
                  {c.code.replaceAll("_", " ")}:{" "}
                </span>
                {c.message}
              </li>
            ))}
        </ul>
        {data.observationPoint ? (
          <p className="text-sm">
            <span className="font-medium">Observation point: </span>
            {data.observationPoint}
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">
          Counts and coverage do not establish statistical confidence or
          end-to-end journey success. Evaluated{" "}
          {new Date(data.evaluatedAt).toISOString()}, revision {data.revision}.
        </p>
        {cohortLabels?.length ? (
          <Button
            size="sm"
            variant="outline"
            onClick={inspect}
            disabled={busy || scope.signal.aborted}
          >
            {busy ? "Inspecting cohorts…" : "Inspect cohorts"}
          </Button>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {cohorts ? (
          <div className="space-y-2 border-t pt-3">
            <p className="text-sm">{cohorts.message}</p>
            <p className="text-xs text-muted-foreground">
              Cohort cutoff: {new Date(cohorts.at).toISOString()}
            </p>
            {cohorts.state === "available" ? (
              <>
                <p className="text-xs text-muted-foreground">
                  {cohorts.matchesAggregate
                    ? "Cohort counts match the saved aggregate at this cutoff."
                    : "Cohort counts do not reconcile with the saved aggregate. Review missing numerators, collection timing and query semantics."}
                </p>
                <div className="overflow-x-auto">
                  <table className="w-full text-left text-sm">
                    <thead>
                      <tr className="border-b">
                        <th className="p-2">Cohort</th>
                        <th className="p-2">SLI</th>
                        <th className="p-2">Eligible events</th>
                        <th className="p-2">Evidence</th>
                      </tr>
                    </thead>
                    <tbody>
                      {cohorts.items.map((item) => (
                        <tr
                          key={JSON.stringify(item.labels)}
                          className="border-b"
                        >
                          <td className="p-2">
                            {Object.entries(item.labels)
                              .map(([k, v]) => `${k}=${v || "(missing)"}`)
                              .join(", ")}
                          </td>
                          <td className="p-2 tabular-nums">
                            {item.reliability === null
                              ? "Unavailable"
                              : percent(item.reliability, 3)}
                          </td>
                          <td className="p-2 tabular-nums">
                            {item.total.toLocaleString()}
                          </td>
                          <td className="p-2">
                            {item.state.replaceAll("_", " ")}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
