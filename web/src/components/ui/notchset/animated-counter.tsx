import { Text } from "@astryxdesign/core/Text";

// Shows the exact formatted measured value. Never interpolate or animate
// between SLI values; an unavailable value says so rather than showing zero.
export function AnimatedCounter({
  value,
  className,
  format = (n: number) => Math.round(n).toLocaleString("en-US"),
}: {
  value: number;
  format?: (value: number) => string;
  className?: string;
}) {
  return (
    <Text
      as="span"
      data-slot="animated-counter"
      className={`tabular-nums ${className ?? ""}`.trim()}
    >
      {Number.isFinite(value) ? format(value) : "Unavailable"}
    </Text>
  );
}
