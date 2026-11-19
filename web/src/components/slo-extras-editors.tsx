import { useId } from "react";
import { sloType } from "@/lib/slo-types";
import { FormField } from "@/components/ui/form-layout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import type { CustomerPolicy } from "@/lib/api";
import {
  MAX_CUSTOMERS,
  MAX_LABELS,
  labelRows,
  labelsFromRows,
  type IntervalDraft,
  type LabelRow,
} from "@/lib/slo-extras";

export function LabelsEditor({
  value,
  onChange,
  disabled,
}: {
  value?: Record<string, string>;
  onChange: (labels: Record<string, string> | undefined) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const rows = labelRows(value);
  const set = (next: LabelRow[]) => {
    try {
      onChange(labelsFromRows(next));
    } catch {
      // Keep typing: invalid drafts are reported when the definition is built.
      onChange(Object.fromEntries(next.map((r) => [r.key, r.value])));
    }
  };
  return (
    <fieldset className="space-y-2 rounded-md border p-4" disabled={disabled}>
      <legend className="px-1 text-sm font-medium">Labels (optional)</legend>
      <p className="text-xs text-muted-foreground">
        Ownership and classification, such as team or product. Labels drive
        filters and roll-ups; they never change what is measured.
      </p>
      {rows.map((row, i) => (
        <div key={i} className="form-grid">
          <FormField htmlFor={`${id}-k${i}`} label="Label name">
            <Input
              id={`${id}-k${i}`}
              value={row.key}
              onChange={(e) =>
                set(
                  rows.map((r, j) =>
                    j === i ? { ...r, key: e.target.value } : r,
                  ),
                )
              }
            />
          </FormField>
          <FormField htmlFor={`${id}-v${i}`} label="Value">
            <Input
              id={`${id}-v${i}`}
              value={row.value}
              onChange={(e) =>
                set(
                  rows.map((r, j) =>
                    j === i ? { ...r, value: e.target.value } : r,
                  ),
                )
              }
            />
          </FormField>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => set(rows.filter((_, j) => j !== i))}
          >
            Remove label {row.key || i + 1}
          </Button>
        </div>
      ))}
      {rows.length < MAX_LABELS ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => set([...rows, { key: "", value: "" }])}
        >
          Add label
        </Button>
      ) : null}
    </fieldset>
  );
}

export function CustomerReportEditor({
  value,
  onChange,
  disabled,
  unsupported,
}: {
  value?: CustomerPolicy;
  onChange: (value: CustomerPolicy | undefined) => void;
  disabled?: boolean;
  /** Why per-customer reporting cannot be used with this measurement. */
  unsupported?: string;
}) {
  const id = useId();
  if (unsupported)
    return (
      <p className="text-xs text-muted-foreground">
        Per-customer reporting: {unsupported}
      </p>
    );
  if (!value)
    return (
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        onClick={() =>
          onChange({ label: "", customers: [{ value: "", name: "" }] })
        }
      >
        Add per-customer report
      </Button>
    );
  const setRow = (
    i: number,
    patch: Partial<CustomerPolicy["customers"][number]>,
  ) =>
    onChange({
      ...value,
      customers: value.customers.map((c, j) =>
        j === i ? { ...c, ...patch } : c,
      ),
    });
  return (
    <fieldset className="space-y-3 rounded-md border p-4" disabled={disabled}>
      <legend className="px-1 text-sm font-medium">
        {sloType("customers").name}
      </legend>
      <p className="text-xs text-muted-foreground">
        Reports this SLO for a short list of customers next to the service-wide
        figure, so one failing customer is not hidden by the average. Use
        account or tenant identifiers, never personal data. It adds no alerts.
      </p>
      <FormField htmlFor={`${id}-label`} label="Customer label on the metric">
        <Input
          id={`${id}-label`}
          value={value.label}
          onChange={(e) => onChange({ ...value, label: e.target.value })}
        />
      </FormField>
      {value.customers.map((c, i) => (
        <div key={i} className="form-grid">
          <FormField htmlFor={`${id}-c${i}`} label="Label value">
            <Input
              id={`${id}-c${i}`}
              value={c.value}
              onChange={(e) => setRow(i, { value: e.target.value })}
            />
          </FormField>
          <FormField htmlFor={`${id}-n${i}`} label="Display name (optional)">
            <Input
              id={`${id}-n${i}`}
              value={c.name ?? ""}
              onChange={(e) => setRow(i, { name: e.target.value || undefined })}
            />
          </FormField>
          <FormField htmlFor={`${id}-t${i}`} label="Own target % (optional)">
            <Input
              id={`${id}-t${i}`}
              inputMode="decimal"
              value={
                c.target === undefined
                  ? ""
                  : String(Number((c.target * 100).toPrecision(12)))
              }
              onChange={(e) =>
                setRow(i, {
                  target: e.target.value.trim()
                    ? Number((Number(e.target.value) / 100).toPrecision(12))
                    : undefined,
                })
              }
            />
          </FormField>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              onChange({
                ...value,
                customers: value.customers.filter((_, j) => j !== i),
              })
            }
          >
            Remove customer {c.name || c.value || i + 1}
          </Button>
        </div>
      ))}
      <div className="flex flex-wrap gap-2">
        {value.customers.length < MAX_CUSTOMERS ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              onChange({
                ...value,
                customers: [...value.customers, { value: "" }],
              })
            }
          >
            Add customer
          </Button>
        ) : null}
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => onChange(undefined)}
        >
          Remove per-customer report
        </Button>
      </div>
    </fieldset>
  );
}

export function IntervalJudgingEditor({
  value,
  onChange,
  disabled,
}: {
  value: IntervalDraft;
  onChange: (value: IntervalDraft) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const throughput = value.mode === "throughput";
  return (
    <fieldset className="space-y-3 rounded-md border p-4" disabled={disabled}>
      <legend className="px-1 text-sm font-medium">
        {throughput ? sloType("throughput").name : sloType("good-minute").name}
        {value.enabled ? "" : " (optional)"}
      </legend>
      <Label className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={value.enabled}
          onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
        />
        Judge intervals instead of events
      </Label>
      <p className="text-xs text-muted-foreground">
        With the interval-count queries above, each measurement interval is
        judged good or bad, and the target is the share of judged intervals that
        were good. Every judged interval weighs the same however busy it was.
      </p>
      {value.enabled ? (
        <>
          <FormField htmlFor={`${id}-mode`} label="Judge each interval by">
            <NativeSelect
              id={`${id}-mode`}
              value={value.mode}
              disabled={disabled}
              onChange={(e) =>
                onChange({
                  ...value,
                  mode: e.target.value as IntervalDraft["mode"],
                })
              }
            >
              <option value="errorRate">
                Error rate ({sloType("good-minute").name})
              </option>
              <option value="throughput">
                Minimum volume ({sloType("throughput").name})
              </option>
            </NativeSelect>
          </FormField>
          {throughput ? (
            <>
              <FormField
                htmlFor={`${id}-least`}
                label="Interval is good with at least this many good events"
              >
                <Input
                  id={`${id}-least`}
                  inputMode="decimal"
                  value={value.minGoodEvents}
                  onChange={(e) =>
                    onChange({ ...value, minGoodEvents: e.target.value })
                  }
                />
              </FormField>
              <p className="text-xs text-muted-foreground">
                The good-events query counts the work done in each interval,
                such as records processed. An observed interval below the
                minimum is bad, including one with nothing processed; an
                interval with no data stays unknown.
              </p>
            </>
          ) : (
            <div className="form-grid">
              <FormField
                htmlFor={`${id}-rate`}
                label="Interval is bad above error rate (0 to 1)"
              >
                <Input
                  id={`${id}-rate`}
                  inputMode="decimal"
                  value={value.errorRate}
                  onChange={(e) =>
                    onChange({ ...value, errorRate: e.target.value })
                  }
                />
              </FormField>
              <FormField
                htmlFor={`${id}-min`}
                label="Minimum events to judge an interval (optional)"
              >
                <Input
                  id={`${id}-min`}
                  inputMode="numeric"
                  value={value.minEvents}
                  onChange={(e) =>
                    onChange({ ...value, minEvents: e.target.value })
                  }
                />
              </FormField>
            </div>
          )}
        </>
      ) : null}
    </fieldset>
  );
}
