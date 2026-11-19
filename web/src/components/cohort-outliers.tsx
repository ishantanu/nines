import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import type { CohortOutliers } from "@/lib/api";
import { Badge } from "@/components/ui/badge";

const name = (labels: Record<string, string>) =>
  Object.entries(labels)
    .map(([k, v]) => `${k}=${v || "(missing)"}`)
    .join(", ");
const pct = (v: number) => `${(v * 100).toFixed(2)}%`;

// A relative comparison, separate from the SLO target: it names cohorts that
// are significantly worse than their peers, with the counts behind the claim.
export function CohortOutlierSummary({
  outliers: o,
}: {
  outliers: CohortOutliers;
}) {
  const label =
    o.state === "incomplete"
      ? "Incomplete evidence"
      : o.state === "insufficient"
        ? "Too few cohorts to compare"
        : o.items.length
          ? `${o.items.length} below peers`
          : "None below peers";
  return (
    <section
      aria-label="Peer comparison"
      className="space-y-2 rounded-md border p-3"
    >
      <div className="flex flex-wrap items-center gap-2">
        <Heading level={3}>Peer comparison</Heading>
        <Badge variant={o.items.length ? "destructive" : "outline"}>
          {label}
        </Badge>
      </div>
      {o.items.length ? (
        <ul className="space-y-1 text-sm">
          {o.items.map((i) => (
            <li key={name(i.labels)}>
              {name(i.labels)}: {pct(i.reliability)} against{" "}
              {pct(i.peerReliability)} for all other cohorts (
              {i.total.toLocaleString()} events, z = {i.zScore.toFixed(1)})
            </li>
          ))}
        </ul>
      ) : null}
      <ul className="list-disc space-y-1 pl-5">
        {o.limitations.map((text) => (
          <li key={text}>
            <Text as="span" size="sm" color="secondary">
              {text}
            </Text>
          </li>
        ))}
      </ul>
    </section>
  );
}
