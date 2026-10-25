import { useId } from "react";
import type { ErrorBudgetPolicy } from "@/lib/api";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
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
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor={`${id}-owner`}>Responsible owner</Label>
          <Input
            id={`${id}-owner`}
            value={value.owner}
            onChange={(e) => onChange({ ...value, owner: e.target.value })}
          />
        </div>
        <div>
          <Label htmlFor={`${id}-url`}>Runbook URL</Label>
          <Input
            id={`${id}-url`}
            type="url"
            value={value.runbookURL}
            onChange={(e) => onChange({ ...value, runbookURL: e.target.value })}
          />
        </div>
        <div>
          <Label htmlFor={`${id}-review`}>
            Review at remaining budget (0 to 1)
          </Label>
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
        </div>
        <div>
          <Label htmlFor={`${id}-exhausted`}>
            When the budget is exhausted
          </Label>
          <select
            id={`${id}-exhausted`}
            className="h-9 w-full rounded-md border bg-background px-3 text-sm"
            value={value.onExhausted || "blocked"}
            onChange={(e) =>
              onChange({
                ...value,
                onExhausted: e.target.value as ErrorBudgetPolicy["onExhausted"],
              })
            }
          >
            <option value="blocked">Block proceeding</option>
            <option value="review_required">Require owner review</option>
          </select>
        </div>
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
