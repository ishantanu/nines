import { useEffect, useState } from "react";
import type { RequestScope } from "@/lib/api";
import { formatTime } from "@/lib/incidents";
import { navigateTo, workspacePath } from "@/lib/navigation";
import {
  listIncidentPostmortems,
  type IncidentPostmortem,
} from "@/lib/postmortems";

// Postmortems written about an incident: published ones link to the review;
// drafts say they are in progress.
export function IncidentPostmortems({
  scope,
  incident,
}: {
  scope: RequestScope;
  incident: string;
}) {
  const [items, setItems] = useState<IncidentPostmortem[] | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    listIncidentPostmortems(scope, incident, controller.signal)
      .then(setItems)
      .catch(() => setItems([]));
    return () => controller.abort();
  }, [scope, incident]);
  if (items === null) return null;
  const open = (id: string) =>
    navigateTo(workspacePath(scope.tenant, "investigations", id));
  return (
    <section aria-labelledby="incident-postmortems">
      <h3 id="incident-postmortems" className="mb-2 text-sm font-medium">
        Postmortems
      </h3>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No postmortem yet. Start a review to write one.
        </p>
      ) : (
        <ul className="space-y-1 text-sm">
          {items.map((p) => (
            <li key={p.reviewId}>
              {p.published ? (
                <button
                  className="text-left underline-offset-2 hover:underline"
                  onClick={() => open(p.reviewId)}
                >
                  {p.reviewTitle}
                </button>
              ) : (
                <span>
                  Draft in progress by {p.savedBy.name || p.savedBy.subject}
                </span>
              )}{" "}
              <span className="text-xs text-muted-foreground">
                saved {formatTime(p.savedAt)}
                {p.openActions
                  ? ` · ${p.openActions} open action item${p.openActions > 1 ? "s" : ""}`
                  : " · all action items done"}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
