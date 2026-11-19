import { useMemo, useState } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { healthOf, type Objective } from "@/lib/api";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export type LabelRow = {
  value: string;
  objectives: number;
  degraded: number;
  atRisk: number;
  unknown: number;
  healthy: number;
  lowestBudget: number | null;
};

export function labelKeys(objectives: Objective[]): string[] {
  const keys = new Set<string>();
  for (const o of objectives)
    if (!o.slo.retirement)
      for (const key of Object.keys(o.slo.labels ?? {})) keys.add(key);
  return [...keys].sort();
}

// Groups active objectives by one label's values. Objectives without the label
// are counted separately, never folded into a value, and an unmeasured budget
// stays unknown rather than being treated as full.
export function rollUpByLabel(
  objectives: Objective[],
  key: string,
): { rows: LabelRow[]; unlabelled: number } {
  const groups = new Map<string, LabelRow>();
  let unlabelled = 0;
  for (const o of objectives) {
    if (o.slo.retirement) continue;
    const value = o.slo.labels?.[key];
    if (value === undefined) {
      unlabelled++;
      continue;
    }
    const row = groups.get(value) ?? {
      value,
      objectives: 0,
      degraded: 0,
      atRisk: 0,
      unknown: 0,
      healthy: 0,
      lowestBudget: null,
    };
    row.objectives++;
    const health = healthOf(o.status);
    if (health === "degraded") row.degraded++;
    else if (health === "at_risk") row.atRisk++;
    else if (health === "healthy") row.healthy++;
    else row.unknown++;
    const budget = o.status.budgetRemainingFraction;
    if (budget !== null && budget !== undefined && Number.isFinite(budget))
      row.lowestBudget =
        row.lowestBudget === null ? budget : Math.min(row.lowestBudget, budget);
    groups.set(value, row);
  }
  const rows = [...groups.values()].sort(
    (a, b) =>
      b.degraded - a.degraded ||
      b.atRisk - a.atRisk ||
      a.value.localeCompare(b.value),
  );
  return { rows, unlabelled };
}

export function LabelRollup({ objectives }: { objectives: Objective[] }) {
  const keys = useMemo(() => labelKeys(objectives), [objectives]);
  const [chosen, setChosen] = useState("");
  const key = keys.includes(chosen) ? chosen : (keys[0] ?? "");
  const { rows, unlabelled } = useMemo(
    () => rollUpByLabel(objectives, key),
    [objectives, key],
  );
  if (!keys.length) return null;
  return (
    <Card className="gap-0 py-0" aria-label="Reliability by label">
      <CardHeader className="border-b py-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <Heading level={2}>Reliability by label</Heading>
            <Text as="p" size="sm" color="secondary">
              Active objectives grouped by one label, worst first. Counts show
              health states, not a combined score.
            </Text>
          </div>
          <div className="w-48">
            <Label htmlFor="rollup-key">Group by</Label>
            <NativeSelect
              id="rollup-key"
              value={key}
              onChange={(e) => setChosen(e.target.value)}
            >
              {keys.map((k) => (
                <option key={k} value={k}>
                  {k}
                </option>
              ))}
            </NativeSelect>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3 py-5">
        <Table aria-label={`Objectives grouped by ${key}`}>
          <TableHeader>
            <TableRow>
              <TableHead>{key}</TableHead>
              <TableHead>Objectives</TableHead>
              <TableHead>Degraded</TableHead>
              <TableHead>At risk</TableHead>
              <TableHead>Incomplete</TableHead>
              <TableHead>Healthy</TableHead>
              <TableHead>Lowest budget</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.value}>
                <TableCell className="font-medium">{r.value}</TableCell>
                <TableCell className="tabular-nums">{r.objectives}</TableCell>
                <TableCell className="tabular-nums">{r.degraded}</TableCell>
                <TableCell className="tabular-nums">{r.atRisk}</TableCell>
                <TableCell className="tabular-nums">{r.unknown}</TableCell>
                <TableCell className="tabular-nums">{r.healthy}</TableCell>
                <TableCell className="tabular-nums">
                  {r.lowestBudget === null
                    ? "Unknown"
                    : `${(r.lowestBudget * 100).toFixed(1)}%`}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {unlabelled ? (
          <Text as="p" size="sm" color="secondary" role="status">
            {unlabelled} active{" "}
            {unlabelled === 1 ? "objective has" : "objectives have"} no{" "}
            <code>{key}</code> label and {unlabelled === 1 ? "is" : "are"} not
            counted above.
          </Text>
        ) : null}
      </CardContent>
    </Card>
  );
}
