import { Badge } from "@/components/ui/badge";
import { classifySLO, sloType, type ClassifiableSLO } from "@/lib/slo-types";

// Shows what the objective counts, derived from its saved definition.
export function SLOTypeBadges({ slo }: { slo: ClassifiableSLO }) {
  const { primary, addOns } = classifySLO(slo);
  return (
    <span className="flex flex-wrap items-center gap-1" data-slo-type={primary}>
      <Badge variant="outline">{sloType(primary).name}</Badge>
      {addOns.map((id) => (
        <Badge key={id} variant="secondary">
          {sloType(id).name}
        </Badge>
      ))}
    </span>
  );
}
