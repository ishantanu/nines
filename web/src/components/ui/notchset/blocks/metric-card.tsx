"use client";

/**
 * Metric Card (Notchset) v1.0.0 · Beautiful UI
 * Docs: https://beautiful-ui.dev/components/metric-card
 * MIT licensed: free to use, change and share.
 */

// Adapted for measured SLI: static exact values, accessible heading, real time-aware
// history supplied as a slot, and no generic percentage-change or fabricated series.
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/card";

export function SLOMetricCard({
  label,
  compact = false,
  value,
  sub,
  trend,
  children,
  className,
}: {
  label: ReactNode;
  compact?: boolean;
  value: ReactNode;
  sub?: ReactNode;
  trend?: ReactNode;
  children?: ReactNode;
  className?: string;
}) {
  return (
    <Card data-slot="slo-metric-card" className={cn("gap-0 py-0", className)}>
      <div
        className={cn(
          "flex min-w-0 flex-col gap-2",
          compact ? "px-4 pt-3" : "px-5 pt-5",
        )}
      >
        {label}
      </div>
      <div
        className={cn(
          "flex flex-col gap-2 pb-3",
          compact ? "px-4 pt-3" : "px-5 pt-5",
        )}
      >
        <div
          className={cn(
            "font-semibold tracking-tight tabular-nums",
            compact ? "text-2xl" : "text-3xl",
          )}
        >
          {value}
        </div>
        {sub && <div className="text-sm text-muted-foreground">{sub}</div>}
      </div>
      {trend && (
        <div className={compact ? "px-4 pb-3" : "px-5 pb-4"}>{trend}</div>
      )}
      {children && (
        <div
          className={cn(
            "mt-auto border-t",
            compact ? "min-h-32 px-4 py-3" : "px-5 py-4",
          )}
        >
          {children}
        </div>
      )}
    </Card>
  );
}
