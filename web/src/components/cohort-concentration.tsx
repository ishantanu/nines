import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import type { CohortConcentration } from "@/lib/api";
import { Badge } from "@/components/ui/badge";

const share = (value: number) => `${(value * 100).toFixed(1)}%`;
const name = (labels: Record<string, string>) =>
  Object.entries(labels)
    .map(([k, v]) => `${k}=${v || "(missing)"}`)
    .join(", ");

// States and verdicts are shown in words, never as a score: the pattern is a
// reading of where failures landed, with the counts behind it.
function headline(c: CohortConcentration) {
  if (c.state === "no_failures")
    return { label: "No failures", variant: "secondary" as const };
  if (c.state === "insufficient_failures")
    return { label: "Too few failures to read", variant: "outline" as const };
  if (c.state === "incomplete")
    return { label: "Incomplete evidence", variant: "outline" as const };
  return c.verdict === "concentrated"
    ? { label: "Failures are concentrated", variant: "destructive" as const }
    : { label: "Failures are spread out", variant: "default" as const };
}

export function CohortConcentrationSummary({
  concentration: c,
}: {
  concentration: CohortConcentration;
}) {
  const head = headline(c);
  return (
    <section
      aria-label="Failure concentration"
      className="space-y-2 rounded-md border p-3"
    >
      <div className="flex flex-wrap items-center gap-2">
        <Heading level={3}>Failure concentration</Heading>
        <Badge variant={head.variant}>{head.label}</Badge>
      </div>
      {c.state === "available" ? (
        <>
          <Text as="p" size="sm">
            {c.failures.toLocaleString()} failures out of{" "}
            {c.events.toLocaleString()} events. The largest cohort holds{" "}
            {share(c.topShare)} of the failures; they behave like{" "}
            {c.effectiveCohorts.toFixed(1)} equally affected cohorts.
          </Text>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b">
                  <th className="p-2">Cohort</th>
                  <th className="p-2">Failures</th>
                  <th className="p-2">Share of failures</th>
                  <th className="p-2">Share of traffic</th>
                  <th className="p-2">Failure share vs traffic share</th>
                </tr>
              </thead>
              <tbody>
                {c.contributors.map((x) => (
                  <tr key={JSON.stringify(x.labels)} className="border-b">
                    <td className="p-2">{name(x.labels)}</td>
                    <td className="p-2 tabular-nums">
                      {x.failures.toLocaleString()}
                    </td>
                    <td className="p-2 tabular-nums">
                      {share(x.failureShare)}
                    </td>
                    <td className="p-2 tabular-nums">{share(x.eventShare)}</td>
                    <td className="p-2 tabular-nums">
                      {x.representation === undefined
                        ? "—"
                        : `${x.representation.toFixed(1)}×`}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      ) : (
        <Text as="p" size="sm" color="secondary">
          {c.state === "insufficient_failures"
            ? `Only ${c.failures.toLocaleString()} failures were observed; at least 30 are needed before a pattern means anything.`
            : c.state === "no_failures"
              ? "No failed events in any cohort at this cutoff."
              : "A cohort has an unknown numerator, so concentration is not computed rather than guessed."}
        </Text>
      )}
      <ul className="list-disc space-y-1 pl-5">
        {c.limitations.map((text) => (
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
