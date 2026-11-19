import * as React from "react";
import { cn } from "cn";

// A plain <label>: Astryx's FieldLabel only takes string text, but call sites
// pass rich children (e.g. replay-label spans), so this keeps the element and
// aligns its type with the theme.
function Label({ className, ...props }: React.ComponentProps<"label">) {
  return (
    <label
      data-slot="label"
      className={cn(
        "flex items-center gap-2 text-sm leading-none font-medium select-none group-data-[disabled=true]:pointer-events-none group-data-[disabled=true]:opacity-50 peer-disabled:cursor-not-allowed peer-disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

export { Label };
