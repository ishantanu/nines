// Adapted from MIT Beautiful UI Notchset v1.0.0. See /beautiful-ui-license.txt.
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/card";
import { AnimatedCounter } from "./animated-counter";

// A compact metric strip: semantic description-list entries, no fabricated
// percentage delta or trend where the API supplies only a current count.
export function MetricCard({
  label,
  value,
  className,
}: {
  label: ReactNode;
  value: number;
  className?: string;
}) {
  return (
    <Card
      data-slot="metric-card"
      className={cn("notch-metric gap-3 px-4 py-3", className)}
    >
      <dt className="flex items-center gap-2 text-sm text-muted-foreground">
        {label}
      </dt>
      <dd className="text-2xl font-semibold tabular-nums">
        <AnimatedCounter value={value} />
      </dd>
    </Card>
  );
}
