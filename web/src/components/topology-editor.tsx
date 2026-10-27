import { useEffect, useId, useRef, useState } from "react";
import { api, message, type RequestScope } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { stringify } from "yaml";
import { DependencyDesigner } from "./dependency-designer";
import { DependencyDiscovery } from "./dependency-discovery";
import { dependencyManifest } from "@/lib/dependency-editor";
import { mapKind } from "@/lib/topology";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
interface ResourcePlan {
  action: string;
  expectedRevision: number;
  digest: string;
  manifest: { kind: string; metadata: { name: string } };
  previous?: { manifest: unknown };
}
export function TopologyEditor({
  scope,
  initial,
  kind,
  label,
  onApplied,
}: {
  scope: RequestScope;
  initial: string;
  kind: string;
  label: string;
  onApplied: (id: string) => void;
}) {
  const id = useId();
  const epoch = useRef(0);
  const dialog = useRef<HTMLDivElement>(null);
  const [tab, setTab] = useState("visual");
  const [open, setOpen] = useState(false),
    [text, setText] = useState(initial),
    [plan, setPlan] = useState<ResourcePlan | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    epoch.current++;
    setOpen(false);
    setPlan(null);
    setError("");
    setBusy(false);
  }, [scope]);
  const writable = scope.role === "owner" || scope.role === "editor";
  let parsed: ReturnType<typeof dependencyManifest> | null = null;
  if (kind === mapKind) {
    try {
      parsed = dependencyManifest(text);
    } catch {
      /* YAML remains editable when the visual draft is incomplete. */
    }
  }
  function change(next: string) {
    epoch.current++;
    setText(next);
    setPlan(null);
    setError("");
  }
  const yamlEditor = (
    <>
      <Label htmlFor={`${id}-manifest`}>YAML manifest</Label>
      <Textarea
        id={`${id}-manifest`}
        value={text}
        disabled={busy}
        onChange={(e) => change(e.target.value)}
        spellCheck={false}
        className="min-h-80 font-mono text-sm"
      />
    </>
  );
  async function review() {
    const generation = ++epoch.current;
    setBusy(true);
    setError("");
    try {
      const p = await api<ResourcePlan>("/api/resources/plan", scope, {
        method: "POST",
        headers: { "Content-Type": "application/yaml" },
        body: text,
      });
      if (p.manifest.kind !== kind)
        throw new Error(`Expected a ${kind} manifest`);
      if (generation === epoch.current && !scope.signal.aborted) setPlan(p);
    } catch (e) {
      if (generation === epoch.current && !scope.signal.aborted)
        setError(message(e));
    } finally {
      if (generation === epoch.current) setBusy(false);
    }
  }
  async function apply() {
    if (!plan) return;
    const generation = ++epoch.current;
    setBusy(true);
    setError("");
    try {
      await api("/api/resources/apply", scope, {
        method: "POST",
        headers: {
          "Content-Type": "application/yaml",
          "If-Match": `"${plan.expectedRevision}"`,
        },
        body: text,
      });
      if (generation === epoch.current && !scope.signal.aborted) {
        setOpen(false);
        onApplied(plan.manifest.metadata.name);
      }
    } catch (e) {
      if (generation === epoch.current && !scope.signal.aborted) {
        setError(message(e));
        setPlan(null);
      }
    } finally {
      if (generation === epoch.current) setBusy(false);
    }
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (busy) return;
        epoch.current++;
        setOpen(v);
        if (v) {
          setTab("visual");
          setText(initial);
          setPlan(null);
          setError("");
        }
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          {label}
        </Button>
      </DialogTrigger>
      <DialogContent
        ref={dialog}
        data-private
        data-replay-block
        className={`max-h-[90vh] overflow-y-auto ${kind === mapKind ? "sm:max-w-6xl" : "sm:max-w-3xl"}`}
      >
        <DialogHeader>
          <DialogTitle>{label}</DialogTitle>
          <DialogDescription>
            Edit the draft, review its plan, then save a new revision. Only
            workspace owners and editors can apply definitions.
          </DialogDescription>
        </DialogHeader>
        {parsed ? (
          <fieldset disabled={busy} className="grid gap-4 sm:grid-cols-3">
            <legend className="sr-only">Map definition</legend>
            <div className="space-y-2">
              <Label htmlFor={`${id}-name`}>Map identifier</Label>
              <Input
                id={`${id}-name`}
                value={parsed.metadata.name}
                maxLength={63}
                onChange={(e) =>
                  change(
                    stringify({
                      ...parsed,
                      metadata: { ...parsed.metadata, name: e.target.value },
                    }),
                  )
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${id}-project`}>Project</Label>
              <Input
                id={`${id}-project`}
                value={parsed.metadata.project}
                maxLength={200}
                onChange={(e) =>
                  change(
                    stringify({
                      ...parsed,
                      metadata: { ...parsed.metadata, project: e.target.value },
                    }),
                  )
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${id}-display`}>Map display name</Label>
              <Input
                id={`${id}-display`}
                value={parsed.spec.displayName}
                maxLength={200}
                onChange={(e) =>
                  change(
                    stringify({
                      ...parsed,
                      spec: { ...parsed.spec, displayName: e.target.value },
                    }),
                  )
                }
              />
            </div>
          </fieldset>
        ) : null}
        {kind === mapKind ? (
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList
              aria-label="Dependency map authoring"
              className="w-full flex-wrap gap-1 overflow-visible group-data-[orientation=horizontal]/tabs:h-auto"
            >
              <TabsTrigger value="visual">Visual editor</TabsTrigger>
              <TabsTrigger value="yaml">YAML</TabsTrigger>
              <TabsTrigger value="discover">
                Discover from Kubernetes
              </TabsTrigger>
            </TabsList>
            <TabsContent value="visual">
              {parsed ? (
                <DependencyDesigner
                  value={parsed}
                  disabled={busy}
                  onChange={(next) => change(stringify(next))}
                />
              ) : (
                <p role="status" className="text-sm text-muted-foreground">
                  Correct the map's YAML structure to use the visual editor.
                </p>
              )}
            </TabsContent>
            <TabsContent value="yaml" className="space-y-2">
              {yamlEditor}
            </TabsContent>
            <TabsContent value="discover">
              {parsed ? (
                <DependencyDiscovery
                  scope={scope}
                  value={parsed}
                  disabled={busy}
                  onChange={(next) => change(stringify(next))}
                  onAdopt={() => {
                    setTab("visual");
                    dialog.current?.scrollTo({ top: 0 });
                  }}
                />
              ) : (
                <p className="text-sm text-muted-foreground">
                  Set valid map metadata in YAML before discovering
                  dependencies.
                </p>
              )}
            </TabsContent>
          </Tabs>
        ) : (
          yamlEditor
        )}
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {plan ? (
          <div className="rounded-md border p-3 text-sm">
            <p className="font-medium">
              {plan.action} · {plan.manifest.metadata.name}
            </p>
            <p className="mt-1 text-muted-foreground">
              Revision {plan.expectedRevision} →{" "}
              {plan.action === "unchanged"
                ? plan.expectedRevision
                : plan.expectedRevision + 1}
            </p>
            <p className="mt-2 break-all font-mono text-xs">
              SHA-256 {plan.digest}
            </p>
            {plan.previous ? (
              <details className="mt-3">
                <summary>Previous definition</summary>
                <pre className="code-block mt-2">
                  {JSON.stringify(plan.previous.manifest, null, 2)}
                </pre>
              </details>
            ) : null}
          </div>
        ) : null}
        <div className="dialog-actions">
          <Button
            variant="outline"
            disabled={busy || !text.trim()}
            onClick={review}
          >
            {busy ? "Working…" : "Validate and plan"}
          </Button>
          <Button disabled={busy || !plan || !writable} onClick={apply}>
            {writable
              ? "Apply reviewed definition"
              : "Owner or editor required"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
