import { Spinner } from "@astryxdesign/core/Spinner";

// Working indicator: an Astryx Spinner whose label is the readable text.
export function TextShimmer({
  children,
  active = true,
  className,
}: {
  children: string;
  active?: boolean;
  className?: string;
}) {
  if (!active) return <span className={className}>{children}</span>;
  return (
    <span data-slot="text-shimmer" className={className}>
      <Spinner size="sm" label={children} />
    </span>
  );
}
