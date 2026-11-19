import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import type { Journey, Objective } from "@/lib/api";
import { Card, CardContent, CardHeader } from "@/components/ui/card";

// Few, user-focused objectives give clearer signals than many. Past this many
// committed objectives on one service, attention spreads thin.
export const CROWDED_SERVICE_OBJECTIVES = 5;

export type AdoptionFinding = {
  id: "crowded_service" | "no_journeys" | "unlinked" | "no_required_link";
  title: string;
  detail: string;
  items: string[];
};

const committed = (o: Objective) =>
  !o.slo.retirement && (o.slo.lifecycle ?? "committed") === "committed";

// Advice only. It reads what is already loaded, changes nothing and does not
// block any action, and every finding names the objects it is about.
export function adoptionFindings(
  objectives: Objective[],
  journeys: Journey[],
): AdoptionFinding[] {
  const active = objectives.filter(committed);
  const out: AdoptionFinding[] = [];
  const perService = new Map<string, number>();
  for (const o of active)
    perService.set(o.slo.service, (perService.get(o.slo.service) ?? 0) + 1);
  const crowded = [...perService]
    .filter(([, n]) => n > CROWDED_SERVICE_OBJECTIVES)
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  if (crowded.length)
    out.push({
      id: "crowded_service",
      title: "Services with many committed objectives",
      detail: `More than ${CROWDED_SERVICE_OBJECTIVES} committed objectives on one service usually dilute attention. Fewer, user-focused objectives give clearer signals; keep the rest experimental or aspirational.`,
      items: crowded.map(([service, n]) => `${service}: ${n} objectives`),
    });
  if (active.length && !journeys.length)
    out.push({
      id: "no_journeys",
      title: "No critical user journeys",
      detail:
        "Objectives defined only from backend metrics can miss what users experience. Define the few journeys users depend on and link the objectives that protect them.",
      items: [],
    });
  else if (journeys.length) {
    const linked = new Set(
      journeys.flatMap((j) => j.links.map((l) => l.link.id)),
    );
    const unlinked = active.filter((o) => !linked.has(o.slo.id));
    if (unlinked.length)
      out.push({
        id: "unlinked",
        title: "Objectives not linked to a journey",
        detail:
          "An objective no journey depends on is hard to prioritise. Link it to the journey it protects, or reconsider whether it needs to be committed.",
        items: unlinked.map((o) => o.slo.name || o.slo.id).sort(),
      });
    const noRequired = journeys.filter(
      (j) => !j.links.some((l) => l.link.required),
    );
    if (noRequired.length)
      out.push({
        id: "no_required_link",
        title: "Journeys with no required objective",
        detail:
          "A journey's health reflects only its required objectives, so with none it cannot be judged. Mark the objectives it cannot do without as required.",
        items: noRequired
          .map((j) => j.resource.manifest.spec.displayName)
          .sort(),
      });
  }
  return out;
}

export function AdoptionReview({
  objectives,
  journeys,
}: {
  objectives: Objective[];
  journeys: Journey[];
}) {
  const findings = adoptionFindings(objectives, journeys);
  if (!findings.length) return null;
  return (
    <Card className="gap-0 py-0" aria-label="Adoption review">
      <CardHeader className="border-b py-4">
        <Heading level={2}>Adoption review</Heading>
        <Text as="p" size="sm" color="secondary">
          Suggestions for keeping objectives few, user-focused and connected.
          They are advice only and change nothing.
        </Text>
      </CardHeader>
      <CardContent className="space-y-4 py-5">
        {findings.map((f) => (
          <section key={f.id} aria-label={f.title} className="space-y-1">
            <Heading level={3}>{f.title}</Heading>
            <Text as="p" size="sm" color="secondary">
              {f.detail}
            </Text>
            {f.items.length ? (
              <ul className="list-disc pl-5 text-sm">
                {f.items.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            ) : null}
          </section>
        ))}
      </CardContent>
    </Card>
  );
}
