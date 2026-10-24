import { replayLabels } from "@/lib/replay-labels";
// Only wrap product-owned copy. Unknown copy still renders normally but stays masked.
// Do not wrap a resource name, measurement, query, error or other provider response.
export function ReplayLabel({ children }: { children: string }) {
  return (
    <span data-replay-label={replayLabels.has(children) ? children : undefined}>
      {children}
    </span>
  );
}
