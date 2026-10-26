import { FormField } from "@/components/ui/form-layout";
import { useId } from "react";
import type { SLOLifecycle } from "@/lib/api";

export function SLOLifecycleSelect({
  value,
  onChange,
  disabled = false,
}: {
  value: SLOLifecycle;
  onChange: (value: SLOLifecycle) => void;
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <div className="space-y-2">
      <FormField htmlFor={id} label="Lifecycle">
        <select
          id={id}
          className="h-9 w-full rounded-md border bg-background px-3 text-sm"
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value as SLOLifecycle)}
        >
          <option value="experimental">Experimental</option>
          <option value="aspirational">Aspirational</option>
          <option value="committed">Committed</option>
          <option value="deprecated">Deprecated</option>
        </select>
      </FormField>
      <p className="text-xs text-muted-foreground">
        {value === "committed"
          ? "Configured burn alerts are enabled."
          : "Measurements continue. Burn alerts are disabled in Nines and generated rules."}
      </p>
    </div>
  );
}
