import * as React from "react";
import { Selector } from "@astryxdesign/core/Selector";
import { splitFieldClasses } from "./field-classes";

// Adapter over Astryx Selector. Astryx takes an options array, so the compound
// parts below are markers that Select reads (they render nothing themselves).
type Option = { value: string; label?: string; disabled?: boolean };
type Section = { type: "section"; title?: string; options: Option[] };

function textOf(node: React.ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (React.isValidElement<{ children?: React.ReactNode }>(node))
    return textOf(node.props.children);
  return "";
}

export function SelectValue(_props: { placeholder?: string }) {
  return null;
}
export function SelectLabel(_props: { children?: React.ReactNode }) {
  return null;
}
export function SelectItem(_props: {
  value: string;
  disabled?: boolean;
  className?: string;
  children?: React.ReactNode;
}) {
  return null;
}
export function SelectGroup(_props: { children?: React.ReactNode }) {
  return null;
}
export function SelectContent(_props: {
  className?: string;
  position?: string;
  children?: React.ReactNode;
}) {
  return null;
}
type TriggerProps = {
  id?: string;
  className?: string;
  size?: "sm" | "default";
  "aria-label"?: string;
  children?: React.ReactNode;
};
export function SelectTrigger(_props: TriggerProps) {
  return null;
}

// Flatten fragments, arrays and conditionals into a list of elements.
function elements(children: React.ReactNode): React.ReactElement[] {
  const out: React.ReactElement[] = [];
  React.Children.forEach(children, (child) => {
    if (!React.isValidElement<{ children?: React.ReactNode }>(child)) return;
    if (child.type === React.Fragment)
      out.push(...elements(child.props.children));
    else out.push(child);
  });
  return out;
}

type ItemProps = {
  value: string;
  disabled?: boolean;
  children?: React.ReactNode;
};
const toOption = (el: React.ReactElement<ItemProps>): Option => ({
  value: el.props.value,
  label: textOf(el.props.children),
  disabled: el.props.disabled,
});

function buildOptions(content: React.ReactNode): Array<Option | Section> {
  const options: Array<Option | Section> = [];
  for (const el of elements(content)) {
    if (el.type === SelectItem)
      options.push(toOption(el as React.ReactElement<ItemProps>));
    else if (el.type === SelectGroup) {
      const parts = elements(
        (el.props as { children?: React.ReactNode }).children,
      );
      const title = parts.find((p) => p.type === SelectLabel);
      options.push({
        type: "section",
        title: title
          ? textOf((title.props as { children?: React.ReactNode }).children)
          : undefined,
        options: parts
          .filter((p) => p.type === SelectItem)
          .map((p) => toOption(p as React.ReactElement<ItemProps>)),
      });
    }
  }
  return options;
}

export function Select({
  value: controlled,
  defaultValue,
  onValueChange,
  disabled,
  name,
  children,
}: {
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  disabled?: boolean;
  name?: string;
  required?: boolean;
  children?: React.ReactNode;
}) {
  const [uncontrolled, setUncontrolled] = React.useState(defaultValue);
  const parts = elements(children);
  const trigger = parts.find((p) => p.type === SelectTrigger) as
    React.ReactElement<TriggerProps> | undefined;
  const content = parts.find((p) => p.type === SelectContent) as
    React.ReactElement<{ children?: React.ReactNode }> | undefined;
  const placeholder = (
    elements(trigger?.props.children).find((p) => p.type === SelectValue) as
      React.ReactElement<{ placeholder?: string }> | undefined
  )?.props.placeholder;
  const value = controlled ?? uncontrolled;
  return (
    <Selector
      {...splitFieldClasses(trigger?.props.className)}
      id={trigger?.props.id}
      data-slot="select"
      label={trigger?.props["aria-label"] ?? ""}
      isLabelHidden
      options={buildOptions(content?.props.children)}
      value={value || undefined}
      placeholder={placeholder}
      size={trigger?.props.size === "sm" ? "sm" : "md"}
      isDisabled={disabled}
      htmlName={name}
      width="100%"
      onChange={(next) => {
        if (next == null) return;
        if (controlled === undefined) setUncontrolled(next);
        onValueChange?.(next);
      }}
    />
  );
}

// Drop-in for a native <select>: takes <option>/<optgroup> children and emits a
// change event carrying `target.value`, rendered with the Astryx Selector. An
// option whose value is "" stays selectable via a sentinel.
const EMPTY = "\u0000empty";
type OptionProps = {
  value?: string | number;
  disabled?: boolean;
  children?: React.ReactNode;
};
type GroupProps = { label?: string; children?: React.ReactNode };
const optionOf = (el: React.ReactElement<OptionProps>): Option => {
  const text = textOf(el.props.children);
  const value = String(el.props.value ?? text);
  return {
    value: value === "" ? EMPTY : value,
    label: text,
    disabled: el.props.disabled,
  };
};
export function NativeSelect({
  id,
  value,
  defaultValue,
  onChange,
  disabled,
  required,
  name,
  className,
  children,
  "aria-label": ariaLabel,
  "aria-describedby": describedBy,
}: {
  id?: string;
  "aria-describedby"?: string;
  value?: string | number;
  defaultValue?: string | number;
  onChange?: React.ChangeEventHandler<HTMLSelectElement>;
  disabled?: boolean;
  required?: boolean;
  name?: string;
  className?: string;
  "aria-label"?: string;
  children?: React.ReactNode;
}) {
  const [uncontrolled, setUncontrolled] = React.useState(
    defaultValue === undefined ? undefined : String(defaultValue),
  );
  const options: Array<Option | Section> = [];
  for (const el of elements(children)) {
    if (el.type === "option")
      options.push(optionOf(el as React.ReactElement<OptionProps>));
    else if (el.type === "optgroup") {
      const group = el as React.ReactElement<GroupProps>;
      options.push({
        type: "section",
        title: group.props.label,
        options: elements(group.props.children).map((o) =>
          optionOf(o as React.ReactElement<OptionProps>),
        ),
      });
    }
  }
  const current = value === undefined ? uncontrolled : String(value);
  const flat = options.flatMap((o) => ("options" in o ? o.options : [o]));
  const selected =
    current === "" || current === undefined
      ? flat.find((o) => o.value === EMPTY)?.value
      : current;
  return (
    <Selector
      containerClassName={splitFieldClasses(className).containerClassName}
      id={id}
      aria-describedby={describedBy}
      data-slot="select"
      label={ariaLabel ?? ""}
      isLabelHidden
      options={options}
      value={selected}
      isDisabled={disabled}
      isRequired={required}
      htmlName={name}
      width="100%"
      onChange={(next) => {
        if (next == null) return;
        const out = next === EMPTY ? "" : next;
        if (value === undefined) setUncontrolled(out);
        const target = { value: out, name, id };
        onChange?.({ target, currentTarget: target } as never);
      }}
    />
  );
}
