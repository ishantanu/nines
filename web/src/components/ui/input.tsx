import * as React from "react";
import { TextInput } from "@astryxdesign/core/TextInput";
import { NumberInput } from "@astryxdesign/core/NumberInput";
import {
  DateTimeInput,
  type ISODateTimeString,
} from "@astryxdesign/core/DateTimeInput";
import { FileInput } from "@astryxdesign/core/FileInput";
import { cn } from "cn";
import { splitFieldClasses } from "./field-classes";

// Adapter over the Astryx field components. Call sites keep the native
// string-valued `value` and `onChange(event)` contract, so number, date-time and
// file fields translate Astryx's typed callbacks into a minimal change event
// (`target.value` / `target.files`). Astryx's NumberInput commits on blur or
// Enter rather than per keystroke; that is its behavior and is kept.
const TEXT_TYPES = new Set([
  "text",
  "password",
  "email",
  "search",
  "tel",
  "url",
]);

// The call sites only read `target.value`, `target.files` and `target.name`.
function changeEvent(
  target: Record<string, unknown>,
): React.ChangeEvent<HTMLInputElement> {
  return { target, currentTarget: target } as never;
}
const num = (v: string | number | undefined) =>
  v === undefined || v === "" || Number.isNaN(Number(v)) ? null : Number(v);

function FileField({
  inputRef,
  onChange,
  accept,
  disabled,
  id,
  ariaLabel,
  className,
  rest,
}: {
  inputRef: React.Ref<HTMLInputElement> | undefined;
  onChange: React.ChangeEventHandler<HTMLInputElement> | undefined;
  accept?: string;
  disabled?: boolean;
  id?: string;
  ariaLabel?: string;
  className?: string;
  rest: object;
}) {
  const [file, setFile] = React.useState<File | null>(null);
  // Call sites reset the field by assigning `ref.current.value = ""`.
  React.useImperativeHandle(
    inputRef,
    () =>
      ({
        get value() {
          return file ? `C:\\fakepath\\${file.name}` : "";
        },
        set value(_: string) {
          setFile(null);
        },
      }) as HTMLInputElement,
    [file],
  );
  return (
    <FileInput
      {...rest}
      id={id}
      data-slot="input"
      label={ariaLabel ?? ""}
      isLabelHidden
      mode="input"
      accept={accept}
      isDisabled={disabled}
      value={file}
      width="100%"
      {...splitFieldClasses(className)}
      onChange={(next) => {
        const chosen = Array.isArray(next) ? next[0] : next;
        setFile(chosen ?? null);
        onChange?.(
          changeEvent({
            files: chosen ? [chosen] : [],
            value: chosen?.name ?? "",
          }),
        );
      }}
    />
  );
}

function Input({
  className,
  type = "text",
  value,
  defaultValue,
  onChange,
  disabled,
  readOnly,
  autoFocus,
  name,
  "aria-label": ariaLabel,
  "aria-invalid": ariaInvalid,
  ref,
  ...rest
}: React.ComponentProps<"input">) {
  const status =
    ariaInvalid && ariaInvalid !== "false"
      ? ({ type: "error" } as const)
      : undefined;
  if (type === "file")
    return (
      <FileField
        inputRef={ref}
        onChange={onChange}
        accept={rest.accept}
        disabled={disabled}
        id={rest.id}
        ariaLabel={ariaLabel}
        className={className}
        rest={{}}
      />
    );
  if (
    (type === "number" || type === "datetime-local") &&
    typeof value === "string" &&
    defaultValue === undefined
  ) {
    const common = {
      id: rest.id,
      "data-slot": "input",
      label: ariaLabel ?? "",
      isLabelHidden: true,
      isDisabled: disabled,
      isReadOnly: readOnly,
      isRequired: rest.required,
      hasAutoFocus: autoFocus,
      htmlName: name,
      width: "100%",
      status,
      ...splitFieldClasses(className),
    } as const;
    if (type === "number")
      return (
        <NumberInput
          {...common}
          value={num(value)}
          min={num(rest.min)}
          max={num(rest.max)}
          step={rest.step === "any" ? null : num(rest.step)}
          hasClear
          onChange={(next) =>
            onChange?.(
              changeEvent({ value: next === null ? "" : String(next), name }),
            )
          }
        />
      );
    return (
      <DateTimeInput
        {...common}
        value={(value || undefined) as ISODateTimeString | undefined}
        min={(rest.min as ISODateTimeString | undefined) || undefined}
        max={(rest.max as ISODateTimeString | undefined) || undefined}
        onChange={(next) =>
          onChange?.(changeEvent({ value: next ?? "", name }))
        }
      />
    );
  }
  if (
    TEXT_TYPES.has(type) &&
    typeof value === "string" &&
    defaultValue === undefined
  ) {
    // The visible name usually comes from an external <Label htmlFor>, so the
    // Astryx label stays visually hidden and empty unless aria-label is given.
    return (
      <TextInput
        {...(rest as object)}
        data-slot="input"
        type={type as "text"}
        label={ariaLabel ?? ""}
        isLabelHidden
        value={value}
        onChange={(_, event) => onChange?.(event)}
        isDisabled={disabled}
        isReadOnly={readOnly}
        hasAutoFocus={autoFocus}
        htmlName={name}
        width="100%"
        {...splitFieldClasses(className)}
        status={status}
      />
    );
  }
  return (
    <input
      type={type}
      value={value}
      defaultValue={defaultValue}
      onChange={onChange}
      disabled={disabled}
      readOnly={readOnly}
      autoFocus={autoFocus}
      name={name}
      aria-label={ariaLabel}
      aria-invalid={ariaInvalid}
      data-slot="input"
      className={cn(
        "h-9 w-full min-w-0 rounded-[var(--radius-element)] border border-input bg-card px-3 py-1 text-sm transition-colors outline-none file:inline-flex file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-outline-color)] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive",
        className,
      )}
      {...rest}
    />
  );
}

export { Input };
