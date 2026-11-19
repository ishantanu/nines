import { FormField } from "@/components/ui/form-layout";
import { NativeSelect } from "@/components/ui/select";
import { sloType } from "@/lib/slo-types";

export type MeasureChoice =
  "availability" | "latency" | "sessions" | "good-minute";

// One decision, one line: how is success counted? Availability is the default,
// so most people never touch it. Names come from the shared SLO type list so
// the dropdown, badges, filters and docs always say the same thing.
// Per-customer reports and labels are optional add-ons and journey run
// objectives belong to journeys, so neither is offered here as a peer.
const order: MeasureChoice[] = [
  "availability",
  "latency",
  "sessions",
  "good-minute",
];
const hints: Record<MeasureChoice, string> = {
  availability: "Requests or jobs that succeed, out of all eligible ones.",
  latency: "Requests faster than a threshold, out of all eligible ones.",
  sessions: "Sessions without a crash or error, out of all sessions.",
  "good-minute":
    "Minutes whose error rate stayed under a line, for signals with a steady noise floor. Opens Settings.",
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
}: {
  value: string;
  onChoose: (kind: MeasureChoice) => void;
  disabled?: boolean;
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
      <p className="text-xs text-muted-foreground">{current.hint}</p>
    </div>
  );
}
