import * as React from "react";
import { TextArea } from "@astryxdesign/core/TextArea";
import { cn } from "cn";
import { splitFieldClasses } from "./field-classes";

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
        rows={rows}
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
