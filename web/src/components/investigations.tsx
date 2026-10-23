import { useResourceRoute } from "@/lib/navigation";
import { useEffect, useRef, useState } from "react";
import {
  MessageSquare,
  Plus,
  RefreshCw,
  ArrowUpRight,
  FileText,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  message,
  APIError,
  type RequestScope,
  type Objective,
  type Journey,
} from "@/lib/api";
import { backtestTime } from "@/lib/intent";
import {
  captureInvestigationEvidence,
  createInvestigation,
  listInvestigations,
  readInvestigation,
  saveInvestigationMessage,
  type Investigation,
  type InvestigationEntry,
  type InvestigationSummary,
  type EvidenceContext,
  type InvestigationScope,
} from "@/lib/investigations";
import { ReviewRuns } from "./review-runs";
import { cn } from "@/lib/utils";

type Resource = {
  key: string;
  name: string;
  kind: InvestigationScope["kind"];
  id: string;
  revision: number;
  step: number;
};
function cutoff(step: number) {
  return new Date((Math.floor(Date.now() / 1000 / step) * step - step) * 1000)
    .toISOString()
    .replace(".000Z", "Z");
}
const pct = (v: number | null | undefined) =>
  v == null ? "Unknown" : `${(v * 100).toFixed(3)}%`;
function ScopeLine({ value }: { value: InvestigationSummary }) {
  return (
    <div className="space-y-1 text-xs text-muted-foreground">
      <p className="break-words">
        {value.scope.kind === "ServiceLevelObjective" ? "SLO" : "CUJ"}:{" "}
        {value.scope.resourceId} · revision {value.scope.expectedRevision}
      </p>
      <p>Evidence cutoff: {new Date(value.scope.at).toLocaleString()}</p>
    </div>
  );
}
export function InvestigationConversation({
  investigation,
}: {
  investigation: Investigation;
}) {
  return (
    <div className="space-y-4 p-4" aria-label="Saved conversation">
      {investigation.entries.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Record the question you are reviewing, then capture the resource
          evidence.
        </p>
      ) : (
        investigation.entries.map((entry) => (
          <article
            key={entry.id}
            className="rounded-md border bg-background p-3"
          >
            <div className="mb-2 flex flex-wrap justify-between gap-2 text-xs text-muted-foreground">
              <span className="font-medium text-foreground">
                {entry.kind === "evidence"
                  ? "Platform evidence"
                  : entry.actor.name || "Workspace member"}
              </span>
              <time dateTime={entry.createdAt}>
                {new Date(entry.createdAt).toLocaleString()}
              </time>
            </div>
            {entry.kind === "message" ? (
              <p className="whitespace-pre-wrap break-words text-sm">
                {entry.text}
              </p>
            ) : (
              <p className="text-sm">
                Resource context captured at the selected cutoff. Inspect the
                saved evidence and its coverage.
              </p>
            )}
          </article>
        ))
      )}
    </div>
  );
}
function ContextEvidence({ context }: { context: EvidenceContext }) {
  const m = context.measurement;
  return (
    <div className="space-y-4">
      <p className="text-sm font-medium">
        {context.resource.displayName || context.resource.id}
      </p>
      {m ? (
        <>
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <dt className="text-xs text-muted-foreground">Observed SLI</dt>
              <dd className="mt-1 font-medium tabular-nums">
                {pct(m.reliability)}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">SLO target</dt>
              <dd className="mt-1 font-medium tabular-nums">{pct(m.target)}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Data coverage</dt>
              <dd className="mt-1 tabular-nums">{pct(m.coverage)}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Burn rate</dt>
              <dd className="mt-1 tabular-nums">
                {m.burnRate == null ? "Unknown" : `${m.burnRate.toFixed(2)}×`}
              </dd>
            </div>
          </dl>
          <p className="text-xs text-muted-foreground">
            Data state: {m.dataStatus.replaceAll("_", " ")}. Measurement:{" "}
            {(context.definition?.measurementMode ?? "unspecified").replaceAll(
              "_",
              " ",
            )}
            .
          </p>
          <p className="text-xs text-muted-foreground">
            Evaluated {new Date(m.evaluatedAt).toLocaleString()}. Window begins{" "}
            {new Date(m.windowStart).toLocaleString()}.
          </p>
        </>
      ) : (
        <>
          <p className="text-sm">
            Journey health: {context.health}. Linked objectives:{" "}
            {context.totalLinks ?? 0}.
          </p>
          <p className="text-xs text-muted-foreground">{context.healthScope}</p>
          {context.links?.map((link) => (
            <p key={link.link.id} className="text-xs">
              {link.link.id}:{" "}
              {link.context?.health ?? link.error?.message ?? "Unknown"}
            </p>
          ))}
          {context.nextCursor ? (
            <p className="text-xs text-amber-800">
              This snapshot is bounded. More linked objectives require
              additional evidence.
            </p>
          ) : null}
        </>
      )}
      {context.warnings?.length ? (
        <ul className="space-y-2 text-xs text-muted-foreground">
          {context.warnings.map((w, i) => (
            <li key={i}>
              {w === "zero_traffic_is_not_success"
                ? "No traffic was observed. Zero traffic does not establish success."
                : w === "current_definition_applied_to_requested_time"
                  ? "Current definition applied to the requested time."
                  : w.replaceAll("_", " ")}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
export function InvestigationEvidenceView({
  entry,
}: {
  entry?: InvestigationEntry;
}) {
  if (!entry?.evidence)
    return (
      <p className="p-4 text-sm text-muted-foreground">
        Capture evidence to preserve the current measurement context at the
        review cutoff. Missing data remains unknown.
      </p>
    );
  const e = entry.evidence;
  return (
    <div className="space-y-5 p-4">
      <ContextEvidence context={e.context} />
      <div className="space-y-1 border-t pt-3 text-xs text-muted-foreground">
        <p>Captured {new Date(e.capturedAt).toLocaleString()}</p>
        <p className="break-all">SHA-256: {e.digest}</p>
      </div>
      <details className="text-xs">
        <summary className="cursor-pointer font-medium">
          View saved context JSON
        </summary>
        <pre className="mt-3 max-h-64 overflow-auto rounded-md bg-muted p-3">
          {JSON.stringify(e.context, null, 2)}
        </pre>
      </details>
    </div>
  );
}
export function Investigations({
  scope,
  objectives,
  journeys,
  onSLO,
  onJourney,
}: {
  scope: RequestScope;
  objectives: Objective[];
  journeys: Journey[];
  onSLO: (id: string) => void;
  onJourney: (id: string) => void;
}) {
  const [items, setItems] = useState<InvestigationSummary[]>([]);
  const [cursor, setCursor] = useState("");
  const [selected, setSelected] = useResourceRoute(
    scope.tenant,
    "investigations",
  );
  const [storedDoc, setDoc] = useState<Investigation | null>(null);
  const doc = storedDoc?.id === selected ? storedDoc : null;
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [dialog, setDialog] = useState(false);
  const [text, setText] = useState("");
  const [title, setTitle] = useState("");
  const [resource, setResource] = useState("");
  const [at, setAt] = useState(() => cutoff(60));
  const [captureId, setCaptureId] = useState("");
  const mounted = useRef(true);
  const selectedRef = useRef(selected);
  selectedRef.current = selected;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const pendingMessage = useRef<{
    investigation: string;
    text: string;
    id: string;
    expectedVersion: number;
  } | null>(null);
  const pendingCapture = useRef<{
    investigation: string;
    id: string;
    expectedVersion: number;
  } | null>(null);
  const pendingCreate = useRef<{ fingerprint: string; id: string } | null>(
    null,
  );
  const writable =
    scope.role === "owner" || scope.role === "editor" || scope.role === "agent";
  const resources: Resource[] = [
    ...objectives.map((o) => ({
      key: "slo/" + o.slo.id,
      name: o.slo.name,
      kind: "ServiceLevelObjective" as const,
      id: o.slo.id,
      revision: o.slo.revision ?? 1,
      step: o.slo.stepSeconds,
    })),
    ...journeys.map((j) => ({
      key: "journey/" + j.resource.manifest.metadata.name,
      name: j.resource.manifest.spec.displayName,
      kind: "CriticalUserJourney" as const,
      id: j.resource.manifest.metadata.name,
      revision: j.resource.revision,
      step: 60,
    })),
  ];
  const chosenResource =
    resources.find((r) => r.key === resource) ?? resources[0];
  function choose(id: string) {
    selectedRef.current = id;
    setSelected(id);
    setDoc(null);
    setText("");
    setCaptureId("");
    setError("");
    pendingMessage.current = null;
    pendingCapture.current = null;
  }
  function accept(value: Investigation) {
    if (scope.signal.aborted || !mounted.current) return;
    if (selectedRef.current === value.id) setDoc(value);
    const summary: InvestigationSummary = {
      id: value.id,
      title: value.title,
      version: value.version,
      scope: value.scope,
      createdAt: value.createdAt,
      updatedAt: value.updatedAt,
      createdBy: value.createdBy,
    };
    setItems((prev) => [summary, ...prev.filter((i) => i.id !== value.id)]);
  }
  async function refresh() {
    if (!selected) return;
    try {
      accept(await readInvestigation(scope, selected));
      setError("");
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    }
  }
  useEffect(() => {
    let active = true;
    setLoading(true);
    listInvestigations(scope)
      .then((list) => {
        if (!active) return;
        setItems(list.items);
        setCursor(list.nextCursor ?? "");
        if (!selected && list.items.length) setSelected(list.items[0].id);
      })
      .catch((e) => {
        if (active && !scope.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [scope]);
  useEffect(() => {
    let active = true;
    setText("");
    setCaptureId("");
    setError("");
    pendingMessage.current = null;
    pendingCapture.current = null;
    if (!selected) {
      setDoc(null);
      return;
    }
    setLoading(true);
    readInvestigation(scope, selected)
      .then((v) => {
        if (active) {
          setDoc(v);
        }
      })
      .catch((e) => {
        if (active && !scope.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [scope, selected]);
  async function create() {
    if (!chosenResource) return;
    setBusy(true);
    setError("");
    try {
      const inputScope: InvestigationScope = {
        kind: chosenResource.kind,
        resourceId: chosenResource.id,
        expectedRevision: chosenResource.revision,
        at: backtestTime(at),
      };
      const fingerprint = JSON.stringify({ title, scope: inputScope });
      if (pendingCreate.current?.fingerprint !== fingerprint)
        pendingCreate.current = { fingerprint, id: crypto.randomUUID() };
      const value = await createInvestigation(scope, {
        id: pendingCreate.current.id,
        title,
        scope: inputScope,
      });
      if (scope.signal.aborted || !mounted.current) return;
      choose(value.id);
      accept(value);
      setDialog(false);
      setTitle("");
      pendingCreate.current = null;
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    if (!doc) return;
    setBusy(true);
    setError("");
    try {
      if (
        pendingMessage.current?.investigation !== doc.id ||
        pendingMessage.current.text !== text
      )
        pendingMessage.current = {
          investigation: doc.id,
          text,
          id: crypto.randomUUID(),
          expectedVersion: doc.version,
        };
      const value = await saveInvestigationMessage(scope, doc.id, {
        id: pendingMessage.current.id,
        expectedVersion: pendingMessage.current.expectedVersion,
        text: pendingMessage.current.text,
      });
      accept(value);
      setText("");
      pendingMessage.current = null;
    } catch (e) {
      if (!scope.signal.aborted) {
        setError(message(e));
        if (e instanceof APIError && e.status === 409) {
          pendingMessage.current = null;
          try {
            accept(await readInvestigation(scope, doc.id));
          } catch {
            /* Keep the original conflict visible and the typed message intact. */
          }
        }
      }
    } finally {
      setBusy(false);
    }
  }
  async function capture() {
    if (!doc) return;
    setBusy(true);
    setError("");
    try {
      if (pendingCapture.current?.investigation !== doc.id)
        pendingCapture.current = {
          investigation: doc.id,
          id: crypto.randomUUID(),
          expectedVersion: doc.version,
        };
      const value = await captureInvestigationEvidence(scope, doc.id, {
        id: pendingCapture.current.id,
        expectedVersion: pendingCapture.current.expectedVersion,
      });
      accept(value);
      setCaptureId(pendingCapture.current.id);
      pendingCapture.current = null;
    } catch (e) {
      if (!scope.signal.aborted) {
        setError(message(e));
        if (e instanceof APIError && e.status === 409) {
          pendingCapture.current = null;
          try {
            accept(await readInvestigation(scope, doc.id));
          } catch {
            /* Keep the original conflict visible and the typed message intact. */
          }
        }
      }
    } finally {
      setBusy(false);
    }
  }
  async function more() {
    setBusy(true);
    try {
      const list = await listInvestigations(scope, cursor);
      if (scope.signal.aborted) return;
      setItems((prev) => [
        ...prev,
        ...list.items.filter((i) => !prev.some((v) => v.id === i.id)),
      ]);
      setCursor(list.nextCursor ?? "");
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const evidence = doc?.entries.filter((e) => e.kind === "evidence") ?? [];
  const chosenEvidence =
    evidence.find((e) => e.id === captureId) ?? evidence.at(-1);
  return (
    <>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          Shared with authorized members of this tenant. Runs are read-only.
        </p>
        <Button
          onClick={() => {
            setAt(cutoff(chosenResource?.step ?? 60));
            setDialog(true);
          }}
          disabled={!writable || busy || !resources.length}
        >
          <Plus className="size-4" />
          New review
        </Button>
      </div>
      {error ? (
        <p
          role="alert"
          className="mb-4 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900"
        >
          {error}
        </p>
      ) : null}
      <div className="investigation-workspace">
        <Card className="investigation-list">
          <div className="panel-header">
            <h2 className="section-label">Reviews</h2>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-2">
            {items.map((item) => (
              <button
                key={item.id}
                disabled={busy}
                className={cn(
                  "mb-1 w-full rounded-md border border-transparent p-3 text-left hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring",
                  selected === item.id && "border-border bg-muted",
                )}
                onClick={() => choose(item.id)}
                aria-current={selected === item.id ? "true" : undefined}
              >
                <p className="mb-2 break-words text-sm font-medium">
                  {item.title}
                </p>
                <ScopeLine value={item} />
              </button>
            ))}
            {loading && !items.length ? (
              <p role="status" className="p-3 text-sm">
                Loading reviews…
              </p>
            ) : !items.length ? (
              <p className="p-3 text-sm text-muted-foreground">
                No investigations have been recorded.
              </p>
            ) : null}
            {cursor ? (
              <Button
                variant="outline"
                size="sm"
                className="m-2"
                disabled={busy}
                onClick={() => void more()}
              >
                Load more
              </Button>
            ) : null}
          </div>
        </Card>
        {doc ? (
          <Card className="investigation-main">
            <div className="panel-header">
              <div className="min-w-0 space-y-2">
                <h2 className="section-label break-words">{doc.title}</h2>
                <ScopeLine value={doc} />
              </div>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() => void refresh()}
                  aria-label="Reload review"
                >
                  <RefreshCw className="size-4" />
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    doc.scope.kind === "ServiceLevelObjective"
                      ? onSLO(doc.scope.resourceId)
                      : onJourney(doc.scope.resourceId)
                  }
                >
                  <ArrowUpRight className="size-4" />
                  Resource
                </Button>
              </div>
            </div>
            <Tabs defaultValue="conversation" className="investigation-content">
              <TabsList className="mx-4 mt-3 xl:hidden">
                <TabsTrigger value="conversation">Conversation</TabsTrigger>
                <TabsTrigger value="evidence">Evidence</TabsTrigger>
              </TabsList>
              <div className="investigation-panes">
                <TabsContent
                  forceMount
                  value="conversation"
                  className="investigation-pane"
                >
                  <div className="flex min-h-14 items-center gap-2 border-b px-4 text-sm font-medium">
                    <MessageSquare className="size-4" />
                    Conversation
                  </div>
                  <div className="min-h-0 flex-1 overflow-y-auto">
                    <InvestigationConversation investigation={doc} />
                  </div>
                  <form
                    className="space-y-2 border-t p-4"
                    onSubmit={(e) => {
                      e.preventDefault();
                      void save();
                    }}
                  >
                    <Label htmlFor="investigation-message">Message</Label>
                    <Textarea
                      id="investigation-message"
                      maxLength={4000}
                      rows={3}
                      value={text}
                      onChange={(e) => setText(e.target.value)}
                      disabled={!writable || busy}
                      placeholder={
                        writable
                          ? "Record a question, finding or decision."
                          : "Your role can read this review."
                      }
                    />
                    <div className="flex justify-end">
                      <Button
                        type="submit"
                        size="sm"
                        disabled={!writable || busy || !text.trim()}
                      >
                        Save message
                      </Button>
                    </div>
                  </form>
                </TabsContent>
                <TabsContent
                  forceMount
                  value="evidence"
                  className="investigation-pane"
                >
                  <div className="flex min-h-14 flex-wrap items-center justify-between gap-2 border-b px-4 py-2">
                    <h3 className="flex items-center gap-2 text-sm font-medium">
                      <FileText className="size-4" />
                      Evidence
                    </h3>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={!writable || busy || evidence.length >= 20}
                      onClick={() => void capture()}
                    >
                      Capture context
                    </Button>
                  </div>
                  <div className="min-h-0 flex-1 overflow-y-auto">
                    {evidence.length > 1 ? (
                      <div className="px-4 pt-3">
                        <Label htmlFor="evidence-snapshot">
                          Saved snapshot
                        </Label>
                        <Select
                          value={chosenEvidence?.id}
                          onValueChange={setCaptureId}
                        >
                          <SelectTrigger
                            id="evidence-snapshot"
                            className="mt-2"
                          >
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {evidence.map((e) => (
                              <SelectItem key={e.id} value={e.id}>
                                {new Date(e.createdAt).toLocaleString()}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    ) : null}
                    <ReviewRuns
                      key={doc.id}
                      scope={scope}
                      review={doc}
                      writable={writable}
                      onConflict={() => void refresh()}
                    />
                    {evidence.length ? (
                      <h3 className="border-t px-4 pt-4 text-sm font-medium">
                        Saved snapshots
                      </h3>
                    ) : null}
                    <InvestigationEvidenceView entry={chosenEvidence} />
                  </div>
                </TabsContent>
              </div>
            </Tabs>
          </Card>
        ) : (
          <Card className="flex items-center justify-center p-6 text-sm text-muted-foreground">
            {loading
              ? "Loading review…"
              : "Select a review or start one for an objective or journey."}
          </Card>
        )}
      </div>
      <Dialog open={dialog} onOpenChange={setDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New review</DialogTitle>
            <DialogDescription>
              Select an exact resource revision and evidence cutoff. The scope
              is retained with every saved message and snapshot.
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              void create();
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="investigation-title">Title</Label>
              <Input
                id="investigation-title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                maxLength={160}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="investigation-resource">Resource</Label>
              <Select
                value={chosenResource?.key}
                onValueChange={(v) => {
                  setResource(v);
                  setAt(cutoff(resources.find((r) => r.key === v)?.step ?? 60));
                }}
              >
                <SelectTrigger id="investigation-resource">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {resources.map((r) => (
                    <SelectItem key={r.key} value={r.key}>
                      {r.kind === "ServiceLevelObjective" ? "SLO" : "CUJ"}:{" "}
                      {r.name} · revision {r.revision}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="investigation-at">
                Evidence cutoff (RFC 3339)
              </Label>
              <Input
                id="investigation-at"
                value={at}
                onChange={(e) => setAt(e.target.value)}
                required
              />
              <p className="text-xs text-muted-foreground">
                Point-in-time context at this cutoff. Each SLO retains its
                configured rolling window.
              </p>
            </div>
            {error ? (
              <p role="alert" className="text-sm text-amber-900">
                {error}
              </p>
            ) : null}
            <div className="dialog-actions">
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialog(false)}
              >
                Cancel
              </Button>
              <Button
                disabled={busy || !title.trim() || !chosenResource}
                type="submit"
              >
                Create review
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
