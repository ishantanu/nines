import { FormField } from "@/components/ui/form-layout";
import { NativeSelect } from "@/components/ui/select";
import type { MeasurementKind } from "@/lib/measurement";
import { sloType } from "@/lib/slo-types";

/**
 * One decision, one line: what does this SLO count? Availability is the
 * default, so most people never touch it. Names come from the shared SLO type
 * list so the select, badges, filters and docs always say the same thing.
 *
 * Good-minute and throughput judge intervals from queries, so choosing them
 * opens Settings. Per-customer is an add-on to any type except sessions:
 * choosing it turns the report on for the current type.
 */
export type MeasureChoice =
  MeasurementKind | "good-minute" | "throughput" | "customers";

const order: MeasureChoice[] = [
  "availability",
  "latency",
  "correctness",
  "durability",
  "freshness",
  "coverage",
  "sessions",
  "good-minute",
  "throughput",
  "customers",
];
const hints: Record<MeasureChoice, string> = {
  availability: "Requests or jobs that succeed, out of all eligible ones.",
  latency: "Requests faster than a threshold, out of all eligible ones.",
  correctness: "Results that pass validation, out of all results.",
  durability: "Stored objects verified intact, out of all objects checked.",
  freshness: "Data younger than an age threshold, out of all data served.",
  coverage: "Records processed, out of all records received.",
  sessions: "Sessions without a crash or error, out of all sessions.",
  "good-minute":
    "Minutes whose error rate stayed under a line, for signals with a steady noise floor. Opens Settings.",
  throughput:
    "Minutes that processed at least a minimum number of events, for pipelines and consumers. Opens Settings.",
  customers:
    "Adds a report for each listed customer beside the service-wide figure, to the type above. Opens Optional settings.",
};
export const measureChoices = order.map((id) => ({
  id,
  label: sloType(id).name,
  hint: hints[id],
}));

export function SLOKindSelect({
  value,
  onChoose,
  disabled,
  perCustomer = false,
}: {
  value: string;
  onChoose: (kind: MeasureChoice) => void;
  disabled?: boolean;
  /** Per-customer reporting is on for this SLO. */
  perCustomer?: boolean;
}) {
  const current =
    measureChoices.find((c) => c.id === value) ?? measureChoices[0];
  return (
    <div className="space-y-1">
      <FormField htmlFor="slo-measure" label="SLO type">
        <NativeSelect
          id="slo-measure"
          value={current.id}
          disabled={disabled}
          onChange={(e) => onChoose(e.target.value as MeasureChoice)}
        >
          {measureChoices.map((c) => (
            <option key={c.id} value={c.id}>
              {c.label}
            </option>
          ))}
        </NativeSelect>
      </FormField>
      <p className="text-xs text-muted-foreground">
        {current.hint}
        {perCustomer
          ? ` ${sloType("customers").name} reporting is on, under Optional settings.`
          : ""}
      </p>
    </div>
  );
}
