import * as React from "react";
import { TextArea } from "@astryxdesign/core/TextArea";
import { cn } from "cn";
import { splitFieldClasses } from "./field-classes";

// Astryx sizes the textarea by rows (default 3), not by height, so a
// min-h-* class only grew the frame around a 3-line box. Derive rows from the
// minimum height (Tailwind spacing: 0.25rem = 4px) and the text size's line
// height, so the editable area fills the space it is given.
export function rowsForMinHeight(className?: string): number | undefined {
  const m = /(?:^|\s)min-h-(\d+)(?:\s|$)/.exec(className ?? "");
  if (!m) return undefined;
  const line = /(?:^|\s)text-xs(?:\s|$)/.test(className ?? "") ? 16 : 20;
  const padding = 16;
  return Math.max(3, Math.floor((Number(m[1]) * 4 - padding) / line));
}

// Adapter over Astryx TextArea for controlled values; uncontrolled usage keeps
// a native element styled from Astryx tokens.
function Textarea({
  className,
  value,
  defaultValue,
  onChange,
  disabled,
  readOnly,
  autoFocus,
  name,
  maxLength,
  spellCheck,
  rows,
  "aria-label": ariaLabel,
  "aria-invalid": ariaInvalid,
  ...rest
}: React.ComponentProps<"textarea">) {
  if (typeof value === "string" && defaultValue === undefined) {
    return (
      <TextArea
        {...(rest as object)}
        data-slot="textarea"
        label={ariaLabel ?? ""}
        isLabelHidden
        value={value}
        onChange={(_, event) => onChange?.(event)}
        isDisabled={disabled}
        isReadOnly={readOnly}
        hasAutoFocus={autoFocus}
        htmlName={name}
        htmlMaxLength={maxLength}
        hasSpellCheck={
          spellCheck === undefined
            ? undefined
            : spellCheck === true || spellCheck === "true"
        }
        rows={rows ?? rowsForMinHeight(className)}
        width="100%"
        {...splitFieldClasses(className)}
        status={
          ariaInvalid && ariaInvalid !== "false" ? { type: "error" } : undefined
        }
      />
    );
  }
  return (
    <textarea
      value={value}
      defaultValue={defaultValue}
      onChange={onChange}
      disabled={disabled}
      readOnly={readOnly}
      autoFocus={autoFocus}
      name={name}
      maxLength={maxLength}
      spellCheck={spellCheck}
      rows={rows}
      aria-label={ariaLabel}
      aria-invalid={ariaInvalid}
      data-slot="textarea"
      className={cn(
        "min-h-16 w-full rounded-[var(--radius-element)] border border-input bg-card px-3 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-outline-color)] disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive",
        className,
      )}
      {...rest}
    />
  );
}

export { Textarea };
