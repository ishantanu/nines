import { useResourceRoute } from "@/lib/navigation";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { Play, RefreshCw, Archive, Link2, Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
  SelectValue,
} from "@/components/ui/select";
import { message, type RequestScope } from "@/lib/api";
import { downloadReplay } from "@/lib/replay-download";
import { MaskedReplayCapture } from "@/lib/replay-capture";
import {
  archiveReplay,
  createReplay,
  linkReplay,
  listReplays,
  replayEvents,
  type SessionReplay,
  type ReplayRef,
  type ReplayEventSet,
} from "@/lib/replays";
export type ReplayOption = { label: string; ref: ReplayRef };
const refKey = (r: ReplayRef) => r.kind + ":" + r.id;
function ReplayPlayer({
  scope,
  replay,
}: {
  scope: RequestScope;
  replay: SessionReplay;
}) {
  const frame = useRef<HTMLIFrameElement>(null);
  const [data, setData] = useState<ReplayEventSet | null>(null);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState("");
  const [downloadError, setDownloadError] = useState("");
  const [downloading, setDownloading] = useState(false);
  useEffect(() => {
    let current = true;
    replayEvents(scope, replay.id)
      .then((value) => {
        if (
          value.tenant !== scope.tenant ||
          value.id !== replay.id ||
          value.digest !== replay.digest ||
          value.events.length > 2000
        )
          throw Error("Replay scope or digest mismatch");
        if (current) setData(value);
      })
      .catch((e) => {
        if (current && !scope.signal.aborted) setError(message(e));
      });
    return () => {
      current = false;
    };
  }, [scope, replay.id, replay.digest]);
  useEffect(() => {
    const receive = (e: MessageEvent) => {
      if (
        e.source === frame.current?.contentWindow &&
        e.origin === window.location.origin &&
        e.data?.type === "nines-replay-ready"
      )
        setReady(true);
    };
    window.addEventListener("message", receive);
    return () => window.removeEventListener("message", receive);
  }, []);
  useEffect(() => {
    if (ready && data)
      frame.current?.contentWindow?.postMessage(
        { type: "nines-replay-load", events: data.events },
        window.location.origin,
      );
  }, [ready, data]);
  return (
    <div
      className="flex shrink-0 flex-col overflow-hidden rounded-md border bg-muted/30"
      data-replay-block
    >
      {error ? (
        <p role="alert" className="p-4 text-sm text-destructive">
          {error}
        </p>
      ) : (
        <>
          <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b p-2 text-xs">
            <span>Recorded page · text and inputs masked</span>
            <Button
              size="sm"
              variant="outline"
              disabled={!data || downloading}
              onClick={async () => {
                if (!data) return;
                setDownloading(true);
                setDownloadError("");
                try {
                  await downloadReplay(replay.id, data.events, scope.signal);
                } catch (e) {
                  if (!scope.signal.aborted) setDownloadError(message(e));
                } finally {
                  setDownloading(false);
                }
              }}
            >
              <Download className="size-3.5" />
              {downloading ? "Preparing download…" : "Download replay (.html)"}
            </Button>
          </div>
          {downloadError ? (
            <p role="alert" className="p-2 text-xs text-destructive">
              {downloadError}
            </p>
          ) : null}
          <iframe
            ref={frame}
            title="Session replay player"
            src="/replay-player.html"
            sandbox="allow-scripts allow-same-origin"
            onLoad={() => setReady(true)}
            className="h-[clamp(20rem,48dvh,40rem)] w-full shrink-0 rounded-md border-0"
          />
          {!data ? (
            <p role="status" className="p-3 text-xs text-muted-foreground">
              Loading recording…
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}
function ReplayLinks({
  scope,
  replay,
  options,
  onSaved,
}: {
  scope: RequestScope;
  replay: SessionReplay;
  options: ReplayOption[];
  onSaved: (r: SessionReplay) => void;
}) {
  const [refs, setRefs] = useState<ReplayRef[]>(() => replay.references);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const all = [
    ...options,
    ...replay.references
      .filter((ref) => !options.some((o) => refKey(o.ref) === refKey(ref)))
      .map((ref) => ({
        label: `${ref.kind}: ${ref.id} · revision ${ref.revision}`,
        ref,
      })),
  ];
  async function save() {
    setBusy(true);
    setError("");
    try {
      const r = await linkReplay(scope, replay, refs);
      if (!scope.signal.aborted) onSaved(r);
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <details className="shrink-0 rounded-md border p-3 text-xs">
      <summary className="cursor-pointer font-medium">
        Edit resource links
      </summary>
      <p className="mt-3 text-muted-foreground">
        Link up to ten resources or stored firing alerts. Links are
        associations, not proof of causation.
      </p>
      <div className="my-3 max-h-44 space-y-3 overflow-auto">
        {all.map(({ label, ref }) => (
          <Label key={refKey(ref)} className="flex items-start gap-2 text-xs">
            <input
              type="checkbox"
              disabled={busy}
              checked={refs.some((r) => refKey(r) === refKey(ref))}
              onChange={(e) =>
                setRefs((prev) =>
                  e.target.checked
                    ? [...prev, ref]
                    : prev.filter((r) => refKey(r) !== refKey(ref)),
                )
              }
            />
            <span>{label}</span>
          </Label>
        ))}
        {!all.length ? <p>No linkable resources in this tenant.</p> : null}
      </div>
      <Button
        size="sm"
        variant="outline"
        disabled={busy || refs.length > 10}
        onClick={() => void save()}
      >
        Save links
      </Button>
      {error ? (
        <p role="alert" className="mt-2 text-destructive">
          {error}
        </p>
      ) : null}
    </details>
  );
}
export function SessionReplays({
  scope,
  capture,
  options,
  onResource,
  initialFilter = "all",
}: {
  scope: RequestScope;
  capture: MaskedReplayCapture;
  options: ReplayOption[];
  onResource: (r: ReplayRef) => void;
  initialFilter?: string;
}) {
  const state = useSyncExternalStore(capture.subscribe, capture.getSnapshot);
  const [items, setItems] = useState<SessionReplay[]>([]);
  const [selected, setSelected] = useResourceRoute(scope.tenant, "replays");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [epoch, setEpoch] = useState(0);
  const [archived, setArchived] = useState(false);
  const [filter, setFilter] = useState(initialFilter);
  const [consent, setConsent] = useState(false);
  const [title, setTitle] = useState("");
  const [pendingID, setPendingID] = useState<string | null>(null);
  const writable = scope.role !== "viewer";
  const owner = scope.role === "owner";
  useEffect(() => {
    let current = true;
    setLoading(true);
    listReplays(scope, archived)
      .then((value) => {
        if (value.tenant !== scope.tenant)
          throw Error("Replay tenant mismatch");
        if (current) {
          setItems(value.items);
          setError("");
        }
      })
      .catch((e) => {
        if (current && !scope.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [scope, epoch, archived]);
  const visible = items.filter(
    (r) =>
      filter === "all" || r.references.some((ref) => refKey(ref) === filter),
  );
  const chosen = selected ? items.find((r) => r.id === selected) : visible[0];
  function accept(r: SessionReplay) {
    setItems((prev) => [r, ...prev.filter((p) => p.id !== r.id)]);
    setSelected(r.id);
  }
  async function start() {
    setError("");
    try {
      await capture.start(consent);
      setConsent(false);
    } catch (e) {
      capture.discard();
      setError(message(e));
    }
  }
  async function save() {
    setBusy(true);
    setError("");
    const id = pendingID ?? crypto.randomUUID();
    setPendingID(id);
    try {
      const r = await createReplay(scope, {
        id,
        title,
        consent: true,
        policy: "masked-v1",
        references: [],
        events: capture.export(),
      });
      if (scope.signal.aborted) return;
      capture.discard();
      accept(r);
      setFilter("all");
      setPendingID(null);
      setTitle("");
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function archive() {
    if (!chosen) return;
    setBusy(true);
    try {
      const r = await archiveReplay(scope, chosen);
      if (!scope.signal.aborted) {
        accept(r);
        setEpoch((v) => v + 1);
      }
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="space-y-4">
      <Card className="gap-0 py-0" data-replay-block>
        <CardContent className="space-y-3 p-4">
          <details open={state.status !== "idle"}>
            <summary className="cursor-pointer text-sm font-medium">
              Record this tab
            </summary>
            <div className="mt-3 space-y-3 text-xs text-muted-foreground">
              <p>
                Record only after consent. Text, input values, media and URLs
                are masked or removed. Layout styles and class names are
                retained. The recording stays in this tab until you save it to
                this tenant. Saved sessions expire after seven days.
              </p>
              {state.status === "idle" ? (
                <>
                  <Label className="flex items-start gap-2 text-xs">
                    <input
                      type="checkbox"
                      checked={consent}
                      disabled={!writable}
                      onChange={(e) => setConsent(e.target.checked)}
                    />
                    <span>
                      I agree to record this tab using the masked recording
                      policy.
                    </span>
                  </Label>
                  <Button
                    size="sm"
                    disabled={!writable || !consent}
                    onClick={() => void start()}
                  >
                    Start recording
                  </Button>
                  <p>
                    This records this application tab from now on. Browse your
                    workspace, then return here to stop and save. Recording
                    pauses on access, data-source and API credential screens.
                  </p>
                </>
              ) : state.status === "recording" || state.status === "paused" ? (
                <>
                  <p role="status">
                    {state.status === "paused"
                      ? "Recording paused on this screen."
                      : "Recording this tab."}{" "}
                    {state.eventCount} events ·{" "}
                    {Math.round(state.duration / 1000)} seconds
                  </p>
                  <div className="flex gap-2">
                    <Button size="sm" onClick={() => capture.stop()}>
                      Stop recording
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => capture.discard()}
                    >
                      Discard recording
                    </Button>
                  </div>
                </>
              ) : (
                <>
                  <p role="status">
                    Recording stopped. {state.eventCount} events ·{" "}
                    {(state.bytes / 1024).toFixed(0)} KiB. {state.reason}
                  </p>
                  <Label htmlFor="replay-title">Recording title</Label>
                  <Input
                    id="replay-title"
                    value={title}
                    maxLength={160}
                    disabled={busy}
                    onChange={(e) => {
                      setTitle(e.target.value);
                      setPendingID(null);
                    }}
                    placeholder="Describe the session"
                  />
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      disabled={busy || !title.trim() || !writable}
                      onClick={() => void save()}
                    >
                      Save recording
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={busy}
                      onClick={() => {
                        capture.discard();
                        setPendingID(null);
                      }}
                    >
                      Discard recording
                    </Button>
                  </div>
                </>
              )}
            </div>
          </details>
        </CardContent>
      </Card>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <Label htmlFor="replay-filter" className="shrink-0 text-xs">
            Linked resource
          </Label>
          <Select value={filter} onValueChange={setFilter}>
            <SelectTrigger id="replay-filter" className="w-full max-w-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All sessions</SelectItem>
              {options.map((o) => (
                <SelectItem key={refKey(o.ref)} value={refKey(o.ref)}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-3">
          {owner ? (
            <Label className="flex items-center gap-2 text-xs">
              <input
                type="checkbox"
                checked={archived}
                onChange={(e) => setArchived(e.target.checked)}
              />
              Include archived
            </Label>
          ) : null}
          <Button
            size="icon"
            variant="outline"
            aria-label="Refresh recordings"
            onClick={() => setEpoch((v) => v + 1)}
          >
            <RefreshCw className="size-4" />
          </Button>
        </div>
      </div>
      {error ? (
        <p
          role="alert"
          className="rounded-md border p-3 text-sm text-destructive"
        >
          {error}
        </p>
      ) : null}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(16rem,0.7fr)_minmax(0,2fr)]">
        <Card className="min-w-0 gap-0 py-0 xl:h-[calc(100dvh-20rem)] xl:min-h-[36rem]">
          <CardHeader className="border-b p-4">
            <CardTitle className="text-sm">Recordings</CardTitle>
            <p className="text-xs text-muted-foreground">
              Saved sessions · seven-day retention · 50 per tenant
            </p>
          </CardHeader>
          <CardContent className="min-h-0 space-y-2 overflow-auto p-3">
            {loading ? (
              <p role="status" className="p-2 text-sm">
                Loading recordings…
              </p>
            ) : !visible.length ? (
              <p className="p-2 text-sm text-muted-foreground">
                No recordings match this view. Record this tab or ingest masked
                events through the API.
              </p>
            ) : (
              visible.map((r) => (
                <button
                  key={r.id}
                  aria-pressed={chosen?.id === r.id}
                  onClick={() => {
                    setSelected(r.id);
                  }}
                  className={`w-full rounded-md border p-3 text-left ${chosen?.id === r.id ? "bg-muted" : "hover:bg-muted/50"}`}
                >
                  <p className="break-words text-sm font-medium">{r.title}</p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {new Date(r.startedAt).toLocaleString()} ·{" "}
                    {(r.durationMillis / 1000).toFixed(1)} seconds
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {r.eventCount} events · {r.references.length} links
                    {r.archived ? " · archived" : ""}
                  </p>
                </button>
              ))
            )}
          </CardContent>
        </Card>
        <Card className="min-w-0 gap-0 py-0 xl:h-[calc(100dvh-20rem)] xl:min-h-[36rem]">
          <CardHeader className="flex flex-row items-start justify-between gap-3 border-b p-4">
            <div className="min-w-0">
              <CardTitle className="break-words text-sm">
                {chosen?.title ?? "Session player"}
              </CardTitle>
              {chosen ? (
                <p className="mt-2 text-xs text-muted-foreground">
                  Masked recording · expires{" "}
                  {new Date(chosen.expiresAt).toLocaleDateString()}
                </p>
              ) : null}
            </div>
            {chosen && owner ? (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => void archive()}
              >
                <Archive className="size-3.5" />
                {chosen.archived ? "Restore" : "Archive"}
              </Button>
            ) : null}
          </CardHeader>
          <CardContent className="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-4">
            {chosen ? (
              <>
                {chosen.recordedBy.name === "Synthetic demo fixture" ? (
                  <p className="rounded-md border bg-muted/30 p-3 text-xs text-muted-foreground">
                    Sample recording: a synthetic checkout page, not your
                    current screen. Use Record this tab above to capture your
                    own session.
                  </p>
                ) : null}
                <ReplayPlayer
                  key={scope.tenant + chosen.id}
                  scope={scope}
                  replay={chosen}
                />
                <details className="shrink-0 text-xs">
                  <summary className="cursor-pointer font-medium">
                    Resource links ({chosen.references.length})
                  </summary>
                  <div className="mt-2 flex flex-col items-start gap-2">
                    {chosen.references.length ? (
                      chosen.references.map((ref) => (
                        <Button
                          key={refKey(ref)}
                          variant="outline"
                          size="sm"
                          className="h-auto max-w-full justify-start whitespace-normal text-left"
                          onClick={() => onResource(ref)}
                        >
                          <Link2 className="size-3.5 shrink-0" />
                          {ref.kind === "AlertEvent"
                            ? `Violation: ${ref.sloId} · ${ref.at ? new Date(ref.at).toLocaleString() : ref.id}`
                            : `${ref.kind === "CriticalUserJourney" ? "CUJ" : "SLO"}: ${ref.id} · revision ${ref.revision}`}
                        </Button>
                      ))
                    ) : (
                      <p className="text-muted-foreground">
                        No resources linked.
                      </p>
                    )}
                  </div>
                </details>
                {writable ? (
                  <ReplayLinks
                    key={chosen.id + ":" + chosen.version}
                    scope={scope}
                    replay={chosen}
                    options={options}
                    onSaved={accept}
                  />
                ) : null}
                <details className="shrink-0 text-xs text-muted-foreground">
                  <summary className="cursor-pointer">
                    Recording details
                  </summary>
                  <p className="mt-2 break-all">SHA-256 {chosen.digest}</p>
                  <p className="mt-1">
                    {chosen.eventCount} events ·{" "}
                    {(chosen.sizeBytes / 1024).toFixed(0)} KiB · policy{" "}
                    {chosen.policy}
                  </p>
                  <p className="mt-1">
                    Submitted by{" "}
                    {chosen.recordedBy.name || "an authorized tenant principal"}
                    . Links are manually associated and do not establish
                    causation.
                  </p>
                </details>
              </>
            ) : (
              <div className="flex min-h-72 flex-1 items-center justify-center text-muted-foreground">
                <Play className="mr-2 size-4" />
                {selected && !loading
                  ? "Recording not found or unavailable in this workspace."
                  : "Select a recording to play."}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
