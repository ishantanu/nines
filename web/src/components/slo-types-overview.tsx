import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import type { Objective } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { SLO_TYPES, countSLOTypes, sloType } from "@/lib/slo-types";
import { SLOTypeGuide } from "./slo-type-guide";

// What the workspace measures, by kind, with every kind explained on demand.
export function SLOTypesOverview({ objectives }: { objectives: Objective[] }) {
  const active = objectives.filter((o) => !o.slo.retirement).map((o) => o.slo);
  const counts = countSLOTypes(active);
  return (
    <Card className="gap-0 py-0" aria-label="SLO types">
      <CardHeader className="border-b py-4">
        <Heading level={2}>SLO types</Heading>
        <Text as="p" size="sm" color="secondary">
          Availability, latency, sessions, good-minute and per-customer SLOs
          answer different questions. Choose the one that matches what users
          experience.
        </Text>
      </CardHeader>
      <CardContent className="space-y-4 py-5">
        {counts.size ? (
          <ul className="flex flex-wrap gap-2" aria-label="SLO types in use">
            {SLO_TYPES.filter((t) => counts.has(t.id)).map((t) => (
              <li key={t.id}>
                <Badge variant="outline">
                  {sloType(t.id).name}: {counts.get(t.id)}
                </Badge>
              </li>
            ))}
          </ul>
        ) : (
          <Text as="p" size="sm" color="secondary">
            No SLOs are defined yet.
          </Text>
        )}
        <details className="disclosure-panel">
          <summary>Explore every kind of SLO</summary>
          <div className="pt-3">
            <SLOTypeGuide title="Kinds of SLO" />
          </div>
        </details>
      </CardContent>
    </Card>
  );
}
