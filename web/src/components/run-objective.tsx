import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { percent, type JourneyRunObjective } from "@/lib/api";
import { Badge } from "@/components/ui/badge";

const hours = (seconds: number) =>
  seconds % 86400 === 0 ? `${seconds / 86400} d` : `${seconds / 3600} h`;

// Unjudged runs and runs for another contract are listed, never counted as
// successes, and a window with too few judged runs is explained, not scored.
export function RunObjectiveCard({
  objective: o,
}: {
  objective: JourneyRunObjective;
}) {
  const met = o.reliability !== null && o.reliability >= o.target;
  return (
    <section
      aria-label="Run objective"
      className="space-y-3 rounded-md border p-4"
    >
      <div className="flex flex-wrap items-center gap-2">
        <Heading level={3}>Run objective</Heading>
        {o.state === "available" ? (
          <Badge variant={met ? "default" : "destructive"}>
            {met ? "Met" : "Not met"}
          </Badge>
        ) : (
          <Badge variant="outline">
            {o.state === "no_runs" ? "No judged runs" : "Too few runs"}
          </Badge>
        )}
      </div>
      <Text as="p" size="sm" color="secondary">
        {percent(o.target, 1)} of judged runs must pass over the last{" "}
        {hours(o.windowSeconds)}, with at least {o.minRuns} judged runs.
      </Text>
      {o.state === "available" ? (
        <Text as="p" size="sm">
          {percent(o.reliability, 1)} of {o.judged} judged runs passed (burn{" "}
          {o.burnRate?.toFixed(2)}×).
        </Text>
      ) : (
        <Text as="p" size="sm">
          {o.judged} of {o.minRuns} judged runs so far, so the objective is not
          scored.
        </Text>
      )}
      <dl className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-5">
        {[
          ["Passed", o.passed],
          ["Failed", o.failed],
          ["Violated", o.violated],
          ["Unjudged", o.unjudged],
          ["Other contract", o.otherRevision],
        ].map(([label, value]) => (
          <div key={label}>
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="font-medium tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
      {o.truncated ? (
        <Text as="p" size="sm" color="secondary" role="status">
          The latest 200 runs were evaluated; this window may hold more.
        </Text>
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
