import { useEffect, useState } from "react";
import type { RequestScope } from "@/lib/api";
import { useFeature } from "@/lib/features";
import { navigateTo, workspacePath } from "@/lib/navigation";
import {
  actionLabels,
  listOpenActionItems,
  type OpenActionItem,
} from "@/lib/postmortems";

// Open postmortem action items that change this SLO or journey. Shows
// nothing when there are none, or when reviews are turned off.
export function OpenActionItems({
  scope,
  slo,
  journey,
}: {
  scope: RequestScope;
  slo?: string;
  journey?: string;
}) {
  const [items, setItems] = useState<OpenActionItem[]>([]);
  const reviewsOn = useFeature("reliability-reviews");
  useEffect(() => {
    if (!reviewsOn) return;
    const controller = new AbortController();
    listOpenActionItems(scope, { slo, journey }, controller.signal)
      .then(setItems)
      .catch(() => setItems([]));
    return () => controller.abort();
  }, [scope, slo, journey, reviewsOn]);
  if (!reviewsOn || !items.length) return null;
  return (
    <section
      aria-labelledby="open-action-items"
      className="rounded-md border p-3"
    >
      <h3 id="open-action-items" className="mb-2 text-sm font-medium">
        Open action items from postmortems
      </h3>
      <ul className="space-y-1 text-sm">
        {items.map((a) => (
          <li key={a.reviewId + a.item.id}>
            <button
              className="text-left underline-offset-2 hover:underline"
              onClick={() =>
                navigateTo(
                  workspacePath(scope.tenant, "investigations", a.reviewId),
                )
              }
            >
              {a.item.title}
            </button>{" "}
            <span className="text-xs text-muted-foreground">
              {actionLabels[a.item.kind]}
              {a.item.owner ? ` · ${a.item.owner}` : ""}
              {a.item.due ? ` · due ${a.item.due}` : ""}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
