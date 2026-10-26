import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Label } from "./label";

export function FormField({
  htmlFor,
  label,
  description,
  children,
  className,
}: {
  htmlFor: string;
  label: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div data-slot="form-field" className={cn("min-w-0 space-y-2", className)}>
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {description ? (
        <p
          id={`${htmlFor}-description`}
          className="text-xs leading-5 text-muted-foreground"
        >
          {description}
        </p>
      ) : null}
    </div>
  );
}
export function FormSection({
  title,
  description,
  children,
  className,
  ...props
}: ComponentProps<"fieldset"> & { title: string; description?: ReactNode }) {
  return (
    <fieldset
      className={cn("min-w-0 rounded-md border p-4 sm:p-5", className)}
      {...props}
    >
      <legend className="px-1 text-sm font-semibold">{title}</legend>
      <div className="space-y-4">
        {description ? (
          <p className="text-sm leading-6 text-muted-foreground">
            {description}
          </p>
        ) : null}
        {children}
      </div>
    </fieldset>
  );
}
