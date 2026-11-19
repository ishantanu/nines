import type { ReactNode } from "react";
import { Badge } from "@/components/ui/badge";

// Status/tag chip on the Astryx Badge.
export function NotchBadge({
  tone = "tag",
  children,
}: {
  tone?: "tag" | "review" | "attention";
  children?: ReactNode;
}) {
  return (
    <Badge
      data-tone={tone}
      variant={
        tone === "attention"
          ? "destructive"
          : tone === "review"
            ? "default"
            : "secondary"
      }
    >
      {children}
    </Badge>
  );
}
