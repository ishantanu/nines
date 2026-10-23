import { useEffect, useState } from "react";
import { stringify } from "yaml";
import { api, message, type RequestScope } from "@/lib/api";
import type { SLODefinition } from "@/lib/definitions";
import { Button } from "@/components/ui/button";

type Revision = {
  revision: number;
  digest: string;
  manifest: SLODefinition;
  archivedAt: string;
  archivedBy: { subject: string; name?: string };
  intervalSamples: number;
  windowObservations: number;
};
export function SLORevisionHistory({
  scope,
  id,
}: {
  scope: RequestScope;
  id: string;
}) {
  const [items, setItems] = useState<Revision[]>([]);
  const [before, setBefore] = useState(0);
  const [next, setNext] = useState(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setItems([]);
    setBefore(0);
    setNext(0);
  }, [scope, id]);
  useEffect(() => {
    const controller = new AbortController();
    setBusy(true);
    setError("");
    api<{ sloId: string; revisions: Revision[]; nextBeforeRevision: number }>(
      `/api/slos/${encodeURIComponent(id)}/revisions?limit=20&beforeRevision=${before}`,
      scope,
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted || scope.signal.aborted) return;
        if (result.sloId !== id)
          throw new Error("Revision response does not match this objective.");
        setItems((current) =>
          before ? [...current, ...result.revisions] : result.revisions,
        );
        setNext(result.nextBeforeRevision);
      })
      .catch((cause) => {
        if (!controller.signal.aborted && !scope.signal.aborted)
          setError(message(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted && !scope.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [scope, id, before]);
  return (
    <details className="rounded-md border p-3 text-sm">
      <summary className="cursor-pointer font-medium">
        Previous revisions
      </summary>
      <p className="mt-2 text-xs text-muted-foreground">
        Replaced definitions and the observations retained at edit time are
        archived separately. The history API accepts a revision to read its
        original evidence. Earlier edits made before revision archiving was
        enabled are unavailable.
      </p>
      {error ? (
        <p role="alert" className="mt-2 text-destructive">
          {error}
        </p>
      ) : null}
      {!busy && !error && !items.length ? (
        <p className="mt-2 text-xs">No archived revisions.</p>
      ) : null}
      {items.map((revision) => (
        <details key={revision.revision} className="mt-3 border-t pt-2">
          <summary className="cursor-pointer">
            Revision {revision.revision} ·{" "}
            {new Date(revision.archivedAt).toLocaleString()}
          </summary>
          <p className="mt-2 text-xs">
            Replaced by{" "}
            {revision.archivedBy.name ||
              revision.archivedBy.subject ||
              "an unattributed client"}
            . Retained {revision.intervalSamples} interval samples and{" "}
            {revision.windowObservations} window observations.
          </p>
          <pre className="code-block mt-2">{stringify(revision.manifest)}</pre>
        </details>
      ))}
      {busy ? (
        <p className="mt-2 text-xs" role="status">
          Loading revisions…
        </p>
      ) : null}
      {next ? (
        <Button
          className="mt-3"
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={() => setBefore(next)}
        >
          Show earlier revisions
        </Button>
      ) : null}
    </details>
  );
}
