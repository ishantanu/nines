import * as React from "react";
import { Button as AstryxButton } from "@astryxdesign/core/Button";

// Adapter over Astryx Button. Call sites keep the established variant/size
// vocabulary; the mapping to Astryx lives here so it changes in one place.
type Variant =
  "default" | "destructive" | "outline" | "secondary" | "ghost" | "link";
type Size =
  "default" | "xs" | "sm" | "lg" | "icon" | "icon-xs" | "icon-sm" | "icon-lg";

const variants = {
  default: "primary",
  destructive: "destructive",
  outline: "secondary",
  secondary: "secondary",
  ghost: "ghost",
  link: "ghost",
} as const;
const sizes = {
  default: "md",
  xs: "sm",
  sm: "sm",
  lg: "lg",
  icon: "md",
  "icon-xs": "sm",
  "icon-sm": "sm",
  "icon-lg": "lg",
} as const;

function textOf(node: React.ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (React.isValidElement<{ children?: React.ReactNode }>(node))
    return textOf(node.props.children);
  return "";
}

// Astryx lays out `icon` before the label and `endContent` after it. Children
// that mix icons and text would otherwise all land in the text span, where a
// block-level SVG wraps onto its own line, so route icons to their slots.
function isIcon(node: React.ReactNode): node is React.ReactElement {
  if (!React.isValidElement<{ children?: React.ReactNode }>(node)) return false;
  return (
    node.type === "svg" ||
    (typeof node.type !== "string" && node.props.children === undefined)
  );
}
function splitIcons(children: React.ReactNode) {
  const nodes = React.Children.toArray(children);
  const firstText = nodes.findIndex((n) => !isIcon(n));
  if (firstText === -1) return { icon: undefined, end: undefined, rest: null };
  let lastText = nodes.length - 1;
  while (lastText > firstText && isIcon(nodes[lastText])) lastText--;
  return {
    icon: firstText > 0 ? nodes[0] : undefined,
    end: lastText < nodes.length - 1 ? nodes[lastText + 1] : undefined,
    rest: nodes.slice(firstText > 0 ? 1 : 0, lastText + 1),
  };
}

type AstryxProps = React.ComponentProps<typeof AstryxButton>;
type ButtonProps = Omit<
  AstryxProps,
  "label" | "variant" | "size" | "isIconOnly" | "isDisabled" | "tooltip"
> & {
  variant?: Variant;
  size?: Size;
  disabled?: boolean;
  title?: string;
  label?: string;
};

function Button({
  variant = "default",
  size = "default",
  disabled,
  title,
  label,
  children,
  // Established behavior: a button inside a form submits unless told otherwise.
  type = "submit",
  ...props
}: ButtonProps) {
  const iconOnly = size.startsWith("icon");
  const name =
    label ?? props["aria-label"] ?? title ?? (textOf(children) || undefined);
  if (iconOnly)
    return (
      <AstryxButton
        {...props}
        type={type}
        label={name ?? ""}
        variant={variants[variant]}
        size={sizes[size]}
        isDisabled={disabled}
        tooltip={title}
        isIconOnly
        icon={children}
      />
    );
  const { icon, end, rest } = splitIcons(children);
  const hasSlots = icon !== undefined || end !== undefined;
  return (
    <AstryxButton
      {...props}
      type={type}
      label={name ?? ""}
      variant={variants[variant]}
      size={sizes[size]}
      isDisabled={disabled}
      tooltip={title}
      {...(hasSlots
        ? {
            icon: icon ?? props.icon,
            endContent: end as AstryxProps["endContent"],
          }
        : {})}
    >
      {hasSlots ? rest : children}
    </AstryxButton>
  );
}

export { Button };
