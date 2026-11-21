import { Badge } from "@/components/ui/badge";
import { maturityPromise, type MaturityFeature } from "@/lib/maturity";

// Shows a feature's maturity where users meet it. Beta is the default for the
// stable core and stays unmarked, so only experimental features carry a badge.
export function MaturityBadge({
  feature,
}: {
  feature: MaturityFeature | undefined;
}) {
  if (feature?.level !== "experimental") return null;
  const why = feature.remaining ? ` Before beta: ${feature.remaining}` : "";
  return (
    <Badge
      variant="secondary"
      title={`${maturityPromise(feature.level)}${why}`}
      aria-label={`${feature.name} is experimental`}
    >
      Experimental
    </Badge>
  );
}
