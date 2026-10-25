import type { BurnRatePolicy } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function PolicyEditor({
  policies,
  onChange,
  disabled = false,
}: {
  policies: BurnRatePolicy[];
  onChange: (value: BurnRatePolicy[]) => void;
  disabled?: boolean;
}) {
  function update(
    index: number,
    key: keyof BurnRatePolicy,
    value: string | number,
  ) {
    onChange(
      policies.map((p, i) => (i === index ? { ...p, [key]: value } : p)),
    );
  }
  return (
    <div className="mt-3 space-y-4">
      {policies.map((policy, index) => (
        <fieldset
          key={index}
          disabled={disabled}
          className="space-y-3 rounded-md border p-4"
        >
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">Policy {index + 1}</p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onChange(policies.filter((_, i) => i !== index))}
            >
              Remove policy
            </Button>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label htmlFor={`policy-name-${index}`}>Name</Label>
              <Input
                id={`policy-name-${index}`}
                value={policy.name ?? ""}
                onChange={(e) => update(index, "name", e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor={`policy-severity-${index}`}>Severity</Label>
              <select
                id={`policy-severity-${index}`}
                className="h-9 w-full rounded-md border bg-background px-3 text-sm"
                value={policy.severity ?? "critical"}
                onChange={(e) => update(index, "severity", e.target.value)}
              >
                {["critical", "warning", "info"].map((s) => (
                  <option key={s}>{s}</option>
                ))}
              </select>
            </div>
            {(
              [
                ["shortWindowSeconds", "Short window (seconds)"],
                ["longWindowSeconds", "Long window (seconds)"],
                ["burnRate", "Burn threshold"],
                ["forSeconds", "Pending duration (seconds)"],
                ["minCoverage", "Minimum coverage (0 to 1)"],
                ["minEvents", "Minimum events per alert window"],
              ] as const
            ).map(([key, label]) => (
              <div key={key}>
                <Label htmlFor={`policy-${key}-${index}`}>{label}</Label>
                <Input
                  id={`policy-${key}-${index}`}
                  type="number"
                  step="any"
                  min="0"
                  value={policy[key] ?? 0}
                  onChange={(e) => update(index, key, Number(e.target.value))}
                />
              </div>
            ))}
          </div>
        </fieldset>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled || policies.length >= 8}
        onClick={() => {
          const names = new Set(policies.map((p) => p.name));
          let suffix = policies.length + 1;
          while (names.has(`policy-${suffix}`)) suffix++;
          onChange([
            ...policies,
            {
              name: `policy-${suffix}`,
              severity: "warning",
              shortWindowSeconds: 300,
              longWindowSeconds: 3600,
              burnRate: 6,
              forSeconds: 120,
              minCoverage: 1,
            },
          ]);
        }}
      >
        Add alert policy
      </Button>
      <p className="text-xs text-muted-foreground">
        Each policy has independent pending, firing and recovery state. Windows
        must fit within the SLO window and align with its measurement step.
        Minimum events gates both windows by observed event count; it is not
        statistical confidence. Zero keeps the nonzero-traffic requirement.
      </p>
    </div>
  );
}
