import * as React from "react";
import { ProgressBar } from "@astryxdesign/core/ProgressBar";

// Adapter over Astryx ProgressBar. The label is required for accessibility,
// so it comes from aria-label and stays visually hidden.
function Progress({
  value,
  "aria-label": label,
  className,
}: {
  value?: number | null;
  "aria-label"?: string;
  className?: string;
}) {
  return (
    <ProgressBar
      data-slot="progress"
      className={className}
      label={label ?? "Progress"}
      isLabelHidden
      value={value ?? 0}
    />
  );
}

export { Progress };
