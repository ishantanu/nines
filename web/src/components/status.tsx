import { CircleCheck, CircleHelp, CircleX, TriangleAlert } from "lucide-react";
import { cn } from "@/lib/utils";

const appearance: Record<string, { icon: typeof CircleCheck; color: string }> =
  {
    healthy: { icon: CircleCheck, color: "status-healthy" },
    passed: { icon: CircleCheck, color: "status-healthy" },
    degraded: { icon: CircleX, color: "status-degraded" },
    violated: { icon: CircleX, color: "status-degraded" },
    failed: { icon: CircleX, color: "status-degraded" },
    at_risk: { icon: TriangleAlert, color: "status-at-risk" },
    incomplete: { icon: TriangleAlert, color: "status-at-risk" },
  };

// Status is information, so it has no button border, fill, hover or focus state.
export function Status({ value }: { value: string }) {
  const { icon: Icon, color } = appearance[value] ?? {
    icon: CircleHelp,
    color: "status-unknown",
  };
  return (
    <span
      data-slot="status"
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 text-xs leading-5 font-medium capitalize",
        color,
      )}
    >
      <Icon className="size-3.5 shrink-0" aria-hidden="true" />
      {value.replaceAll("_", " ")}
    </span>
  );
}
