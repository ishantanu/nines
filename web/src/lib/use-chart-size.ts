import { useLayoutEffect, useState } from "react";

// Measure both axes in CSS pixels so labels and strokes keep their size when
// a panel changes width, height, sidebar layout or browser zoom.
export function useChartSize() {
  const [element, ref] = useState<HTMLDivElement | null>(null);
  const [size, setSize] = useState({ width: 600, height: 230 });
  useLayoutEffect(() => {
    if (!element) return;
    const measure = (width: number, height: number) => {
      if (width <= 0 || height <= 0) return;
      const next = { width: Math.round(width), height: Math.round(height) };
      setSize((old) =>
        old.width === next.width && old.height === next.height ? old : next,
      );
    };
    const rect = element.getBoundingClientRect();
    measure(rect.width, rect.height);
    const observer = new ResizeObserver(([entry]) => {
      if (entry) measure(entry.contentRect.width, entry.contentRect.height);
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [element]);
  return { ref, ...size };
}
