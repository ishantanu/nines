import { useEffect, useState } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { api, message, type RequestScope } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

// Mirrors api/v1alpha1/dependency_composition.go.
export type CompositionCheck = {
  service: string;
  sloId: string;
  target: number;
  verdict:
    | "exceeds_ceiling"
    | "within_ceiling"
    | "within_known_ceiling"
    | "no_ceiling";
  ceiling?: number;
  headroom?: number;
  dependencies: { service: string; sloId: string; target: number }[];
  unmeasuredDependencies: string[];
};
export type DependencyComposition = {
  tenant: string;
  id: string;
  checks: CompositionCheck[];
  limitations: string[];
};

const pct = (value: number) => `${(value * 100).toFixed(3)}%`;
const verdicts = {
  exceeds_ceiling: { label: "Above ceiling", variant: "destructive" },
  within_ceiling: { label: "Within ceiling", variant: "default" },
  within_known_ceiling: { label: "Within known ceiling", variant: "secondary" },
  no_ceiling: { label: "No ceiling", variant: "outline" },
} as const;

export function DependencyCompositionPanel({
  scope,
  id,
  revision,
}: {
  scope: RequestScope;
  id: string;
  revision: number;
}) {
  const [data, setData] = useState<DependencyComposition | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    setError("");
    api<DependencyComposition>(
      `/api/dependency-maps/${encodeURIComponent(id)}/composition`,
      scope,
      { signal: controller.signal },
    )
      .then((next) => {
        if (!controller.signal.aborted) setData(next);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(message(e));
      });
    return () => controller.abort();
  }, [scope, id, revision]);
  const checks = data?.checks.filter((c) => c.verdict !== "no_ceiling") ?? [];
  return (
    <Card className="gap-0 py-0" aria-label="Composed availability ceiling">
      <CardHeader className="border-b py-4">
        <Heading level={2}>
          Availability ceiling from critical dependencies
        </Heading>
        <Text as="p" size="sm" color="secondary">
          A service in series with critical dependencies cannot honestly promise
          more than the product of their committed availability targets.
        </Text>
      </CardHeader>
      <CardContent className="space-y-4 py-5">
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : !data ? (
          <Text color="secondary" role="status">
            Checking composed availability…
          </Text>
        ) : !checks.length ? (
          <Text color="secondary" role="status">
            No committed availability SLO has a critical dependency with a
            committed availability SLO, so no ceiling can be computed.
          </Text>
        ) : (
          <Table aria-label="Composed availability by objective">
            <TableHeader>
              <TableRow>
                <TableHead>Objective</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Ceiling</TableHead>
                <TableHead>Headroom</TableHead>
                <TableHead>Result</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {checks.map((c) => (
                <TableRow key={`${c.service}/${c.sloId}`}>
                  <TableCell>
                    <span className="font-medium">{c.sloId}</span>
                    <span className="block text-xs text-muted-foreground">
                      {c.service} depends on{" "}
                      {c.dependencies.map((d) => d.service).join(", ")}
                      {c.unmeasuredDependencies.length
                        ? `; no committed availability SLO for ${c.unmeasuredDependencies.join(", ")}`
                        : ""}
                    </span>
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {pct(c.target)}
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {c.ceiling === undefined ? "—" : pct(c.ceiling)}
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {c.headroom === undefined
                      ? "—"
                      : `${c.headroom >= 0 ? "+" : "−"}${Math.abs(c.headroom * 100).toFixed(3)} pts`}
                  </TableCell>
                  <TableCell>
                    <Badge variant={verdicts[c.verdict].variant}>
                      {verdicts[c.verdict].label}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        {data ? (
          <ul className="list-disc space-y-1 pl-5">
            {data.limitations.map((text) => (
              <li key={text}>
                <Text as="span" size="sm" color="secondary">
                  {text}
                </Text>
              </li>
            ))}
          </ul>
        ) : null}
      </CardContent>
    </Card>
  );
}
