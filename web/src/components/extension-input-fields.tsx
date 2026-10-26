import { useId } from "react";
import type { SLOTemplate } from "@/lib/slo-templates";
import { extensionLabelRows } from "@/lib/slo-templates";
import { MeasurementFilters } from "./measurement-editor";
import { Label } from "./ui/label";
import { Input } from "./ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";

export function ExtensionInputFields({
  template,
  values,
  disabled,
  onChange,
}: {
  template: SLOTemplate;
  values: Record<string, unknown>;
  disabled: boolean;
  onChange: (name: string, value: unknown) => void;
}) {
  const id = useId();
  return (
    <fieldset className="space-y-4" disabled={disabled}>
      <legend className="mb-3 text-sm font-medium">Extension inputs</legend>
      {Object.entries(template.inputs ?? {}).map(([name, schema]) => {
        const value = values[name] ?? schema.default;
        const label = `${name}${schema.required ? " (required)" : ""}`;
        return (
          <div key={name} className="space-y-1">
            {schema.type === "labels" ? (
              <MeasurementFilters
                title={label}
                rows={extensionLabelRows(value)}
                disabled={disabled}
                onChange={(rows) => onChange(name, rows)}
              />
            ) : (
              <>
                <Label htmlFor={`${id}-${name}`}>{label}</Label>
                {schema.enum?.length ? (
                  <Select
                    disabled={disabled}
                    value={String(value ?? "")}
                    onValueChange={(v) => onChange(name, v)}
                  >
                    <SelectTrigger id={`${id}-${name}`} className="w-full">
                      <SelectValue placeholder="Choose a value" />
                    </SelectTrigger>
                    <SelectContent>
                      {schema.enum.map((v) => (
                        <SelectItem key={v} value={v}>
                          {v}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <Input
                    id={`${id}-${name}`}
                    disabled={disabled}
                    value={String(value ?? "")}
                    type={
                      schema.type === "number" || schema.type === "integer"
                        ? "number"
                        : "text"
                    }
                    min={schema.minimum}
                    max={schema.maximum}
                    step={schema.type === "integer" ? 1 : "any"}
                    onChange={(e) => onChange(name, e.target.value)}
                  />
                )}
              </>
            )}
            <p className="text-xs text-muted-foreground">
              {schema.description}
            </p>
          </div>
        );
      })}
    </fieldset>
  );
}
