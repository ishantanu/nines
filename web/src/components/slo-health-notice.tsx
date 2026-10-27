import { percent, type Snapshot } from "@/lib/api";

export function SLOHealthNotice({ status }: { status: Snapshot }) {
  if (status.reliability !== null && status.coverage >= 1) return null;
  const noTraffic = status.dataStatus === "no_traffic";
  const measured = status.reliability !== null;
  const title = noTraffic
    ? "No traffic. "
    : measured
      ? "Partial measurement window. "
      : "No SLI data. ";
  const reason = noTraffic
    ? "Measurements contain no traffic, so attainment cannot be calculated. Check the denominator and selected population."
    : measured
      ? `SLI attainment is calculated from the available measurements, covering ${percent(status.coverage, 3)} of the selected SLO window. This is not yet a full-window compliance result. Collect or backfill missing intervals and check sample freshness.`
      : "No usable attainment measurement is available for the selected window. Check the telemetry connection, queries and collection evidence.";
  return (
    <p role="status" className="rounded-md border p-4 text-sm leading-6">
      <span className="font-medium">{title}</span>
      {reason} Review collection evidence in Measurement and rule activation in
      Rules.
    </p>
  );
}
