import * as React from "react";
import { Badge as AstryxBadge } from "@astryxdesign/core/Badge";

// Adapter over Astryx Badge; call sites keep the established variant names.
type Variant =
  "default" | "secondary" | "destructive" | "outline" | "ghost" | "link";
const variants = {
  default: "purple",
  secondary: "neutral",
  destructive: "error",
  outline: "neutral",
  ghost: "neutral",
  link: "neutral",
} as const;

function Badge({
  variant = "default",
  children,
  ...props
}: Omit<React.ComponentProps<"span">, "children"> & {
  variant?: Variant;
  children?: React.ReactNode;
}) {
  return (
    <AstryxBadge
      data-slot="badge"
      data-variant={variant}
      variant={variants[variant]}
      label={children}
      {...(props as object)}
    />
  );
}

export { Badge };
