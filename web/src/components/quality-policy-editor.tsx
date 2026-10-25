import { useEffect, useId, useState } from "react";
import type { MeasurementQualityPolicy } from "@/lib/api";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
export function QualityPolicyEditor({
  value,
  onChange,
  disabled = false,
}: {
  value?: MeasurementQualityPolicy;
  onChange: (value: MeasurementQualityPolicy | undefined) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const normalized = value?.cohortLabels?.join(",") ?? "";
  const [cohortText, setCohortText] = useState(normalized);
  useEffect(() => {
    if (
      cohortText
        .split(",")
        .map((x) => x.trim())
        .filter(Boolean)
        .join(",") !== normalized
    )
      setCohortText(normalized);
  }, [normalized, cohortText]);
  function update(next: MeasurementQualityPolicy) {
    onChange(
      next.minEvents || next.maxSampleAgeSeconds || next.cohortLabels?.length
        ? next
        : undefined,
    );
  }
  return (
    <fieldset className="space-y-3 rounded-md border p-4" disabled={disabled}>
      <legend className="px-1 text-sm font-medium">
        Measurement quality requirements
      </legend>
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor={`${id}-events`}>Minimum eligible events</Label>
          <Input
            id={`${id}-events`}
            type="number"
            min="0"
            step="any"
            value={value?.minEvents ?? 0}
            onChange={(e) =>
              update({ ...value, minEvents: Number(e.target.value) })
            }
          />
        </div>
        <div>
          <Label htmlFor={`${id}-age`}>Maximum sample age (seconds)</Label>
          <Input
            id={`${id}-age`}
            type="number"
            min="0"
            value={value?.maxSampleAgeSeconds ?? 0}
            onChange={(e) =>
              update({ ...value, maxSampleAgeSeconds: Number(e.target.value) })
            }
          />
        </div>
        <div className="sm:col-span-2">
          <Label htmlFor={`${id}-cohorts`}>
            Cohort labels (comma separated)
          </Label>
          <Input
            id={`${id}-cohorts`}
            value={cohortText}
            onChange={(e) => {
              setCohortText(e.target.value);
              update({
                ...value,
                cohortLabels: e.target.value
                  .split(",")
                  .map((x) => x.trim())
                  .filter(Boolean),
              });
            }}
            placeholder="region,customer_tier"
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        Zero sample age uses the measurement interval and collection delay.
        Counts are a traffic requirement, not statistical confidence. Cohort
        inspection is an explicit provider read, limited to 100 groups and three
        labels.
      </p>
    </fieldset>
  );
}
