import { healthOf, percent, type Snapshot } from "@/lib/api";

export function SLOHealthNotice({ status }: { status: Snapshot }) {
  if (healthOf(status) !== "unknown") return null;
  const reason =
    status.dataStatus === "no_traffic"
      ? "Measurements contain no traffic, so attainment cannot be calculated. Check the denominator and selected population."
      : status.coverage < 1
        ? `Measurement coverage is ${percent(status.coverage, 1)} of the selected SLO window. Collect or backfill the missing intervals and check sample freshness. Displayed ratios use the available measurements.`
        : "No usable attainment measurement is available for the selected window. Check the query source and collection evidence.";
  return (
    <p role="status" className="rounded-md border p-4 text-sm leading-6">
      <span className="font-medium">SLO health is unknown. </span>
      {reason} Rule activation is verified separately above.
    </p>
  );
}
