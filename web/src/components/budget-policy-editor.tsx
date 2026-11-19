import { FormField } from "@/components/ui/form-layout";
import { NativeSelect } from "@/components/ui/select";
import { useId } from "react";
import type { BudgetBand, ErrorBudgetPolicy } from "@/lib/api";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
export function BudgetPolicyEditor({
  value,
  onChange,
  disabled = false,
}: {
  value?: ErrorBudgetPolicy;
  onChange: (value: ErrorBudgetPolicy | undefined) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const setBand = (i: number, patch: Partial<BudgetBand>) =>
    value &&
    onChange({
      ...value,
      bands: (value.bands ?? []).map((b, j) =>
        j === i ? { ...b, ...patch } : b,
      ),
    });
  const removeBand = (i: number) => {
    if (!value) return;
    const bands = (value.bands ?? []).filter((_, j) => j !== i);
    const { bands: _drop, ...rest } = value;
    void _drop;
    onChange(bands.length ? { ...rest, bands } : rest);
  };
  if (!value)
    return (
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        onClick={() =>
          onChange({
            owner: "",
            runbookURL: "",
            reviewBelow: 0.25,
            onExhausted: "blocked",
          })
        }
      >
        Add error-budget policy
      </Button>
    );
  return (
    <fieldset className="space-y-3 rounded-md border p-4" disabled={disabled}>
      <legend className="px-1 text-sm font-medium">
        Error-budget response policy
      </legend>
      <div className="form-grid">
        <div>
          <FormField htmlFor={`${id}-owner`} label="Responsible owner">
            <Input
              id={`${id}-owner`}
              value={value.owner}
              onChange={(e) => onChange({ ...value, owner: e.target.value })}
            />
          </FormField>
        </div>
        <div>
          <FormField htmlFor={`${id}-url`} label="Runbook URL">
            <Input
              id={`${id}-url`}
              type="url"
              value={value.runbookURL}
              onChange={(e) =>
                onChange({ ...value, runbookURL: e.target.value })
              }
            />
          </FormField>
        </div>
        <div>
          <FormField
            htmlFor={`${id}-review`}
            label="Review at remaining budget (0 to 1)"
          >
            <Input
              id={`${id}-review`}
              type="number"
              min="0"
              max="1"
              step="any"
              value={value.reviewBelow}
              onChange={(e) =>
                onChange({ ...value, reviewBelow: Number(e.target.value) })
              }
            />
          </FormField>
        </div>
        <div>
          <FormField
            htmlFor={`${id}-exhausted`}
            label="When the budget is exhausted"
          >
            <NativeSelect
              id={`${id}-exhausted`}
              className="h-9 w-full rounded-md border bg-background px-3 text-sm"
              value={value.onExhausted || "blocked"}
              onChange={(e) =>
                onChange({
                  ...value,
                  onExhausted: e.target
                    .value as ErrorBudgetPolicy["onExhausted"],
                })
              }
            >
              <option value="blocked">Block proceeding</option>
              <option value="review_required">Require owner review</option>
            </NativeSelect>
          </FormField>
        </div>
      </div>
      <div className="space-y-2">
        <div className="text-sm font-medium">Response levels (optional)</div>
        <p className="text-xs text-muted-foreground">
          State the response agreed for each level of remaining budget, for
          example slowing risky launches. Levels are shown with the decision;
          they never change it or block anything.
        </p>
        {(value.bands ?? []).map((band, i) => (
          <div key={i} className="form-grid">
            <FormField htmlFor={`${id}-band-${i}-name`} label="Level name">
              <Input
                id={`${id}-band-${i}-name`}
                value={band.name}
                onChange={(e) => setBand(i, { name: e.target.value })}
              />
            </FormField>
            <FormField
              htmlFor={`${id}-band-${i}-below`}
              label="At or below remaining budget (0 to 1)"
            >
              <Input
                id={`${id}-band-${i}-below`}
                type="number"
                min="0"
                max="1"
                step="any"
                value={band.below}
                onChange={(e) => setBand(i, { below: Number(e.target.value) })}
              />
            </FormField>
            <FormField
              htmlFor={`${id}-band-${i}-response`}
              label="Agreed response"
            >
              <Input
                id={`${id}-band-${i}-response`}
                value={band.response}
                onChange={(e) => setBand(i, { response: e.target.value })}
              />
            </FormField>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => removeBand(i)}
            >
              Remove level {band.name || i + 1}
            </Button>
          </div>
        ))}
        {(value.bands ?? []).length < 5 ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              onChange({
                ...value,
                bands: [
                  ...(value.bands ?? []),
                  { name: "", below: 0.5, response: "" },
                ],
              })
            }
          >
            Add response level
          </Button>
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">
        Insufficient measurement evidence never permits proceeding. Decisions
        guide engineers and CI; they grant no deployment permission and do not
        automatically pause releases.
      </p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => onChange(undefined)}
      >
        Remove error-budget policy
      </Button>
    </fieldset>
  );
}
