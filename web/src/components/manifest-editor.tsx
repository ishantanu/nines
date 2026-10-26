import { useEffect, useRef, useState } from "react";
import { stringify } from "yaml";
import { Plus, Settings2, FileCheck2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { FormField } from "@/components/ui/form-layout";
import {
  api,
  canEdit,
  type RequestScope,
  message,
  type Manifest,
  type Plan,
  type Revision,
  type Objective,
  type SLO,
} from "@/lib/api";
import { newJourney, parseJourney } from "@/lib/journeys";
import { JourneyFields } from "./journey-fields";

export function ManifestEditor({
  scope,
  resource,
  onApplied,
  onProposed,
}: {
  scope: RequestScope;
  resource?: Revision;
  onApplied: (id?: string) => void;
  onProposed?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState("form");
  const [draft, setDraft] = useState<Manifest>(newJourney);
  const [text, setText] = useState("");
  const [plan, setPlan] = useState<Plan | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [objectives, setObjectives] = useState<SLO[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState("");
  const request = useRef(new AbortController());
  const planRef = useRef<HTMLElement>(null);
  const errorRef = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    (error ? errorRef.current : planRef.current)?.scrollIntoView?.({
      block: "nearest",
    });
  }, [plan, error]);
  const editable = canEdit(scope) || (scope.role === "agent" && !!onProposed);
  useEffect(() => {
    setOpen(false);
    setPlan(null);
    setBusy(false);
  }, [scope, resource?.manifest.metadata.name]);
  useEffect(() => {
    const c = new AbortController();
    request.current = c;
    if (open) {
      setObjectives([]);
      setLoading(true);
      setLoadError("");
      api<Objective[]>("/api/slos", scope, { signal: c.signal })
        .then((rows) => {
          if (!c.signal.aborted) setObjectives(rows.map((row) => row.slo));
        })
        .catch((e) => {
          if (!c.signal.aborted) setLoadError(message(e));
        })
        .finally(() => {
          if (!c.signal.aborted) setLoading(false);
        });
    }
    return () => c.abort();
  }, [open, scope]);
  function changeOpen(value: boolean) {
    if (busy) return;
    setOpen(value);
    if (value) {
      const initial = resource?.manifest ?? newJourney();
      setDraft(initial);
      setText(stringify(initial));
      setMode("form");
      setPlan(null);
      setError("");
    }
  }
  function change(value: Manifest) {
    setDraft(value);
    setPlan(null);
    setError("");
  }
  function changeMode(value: string) {
    try {
      if (value === "yaml") setText(stringify(draft));
      else setDraft(parseJourney(text));
      setMode(value);
      setPlan(null);
      setError("");
    } catch (e) {
      setError(message(e));
      setPlan(null);
    }
  }
  async function review() {
    const c = request.current;
    setBusy(true);
    setPlan(null);
    setError("");
    try {
      const body = mode === "form" ? stringify(draft) : text;
      const result = await api<Plan>("/api/resources/plan", scope, {
        method: "POST",
        headers: { "Content-Type": "application/yaml" },
        body,
        signal: c.signal,
      });
      if (result.manifest.kind !== "CriticalUserJourney")
        throw Error("Use a CriticalUserJourney manifest in this editor.");
      if (!c.signal.aborted) setPlan(result);
    } catch (e) {
      if (!c.signal.aborted) setError(message(e));
    } finally {
      if (!c.signal.aborted) setBusy(false);
    }
  }
  async function apply() {
    if (!plan || !editable) return;
    const c = request.current;
    setBusy(true);
    setError("");
    try {
      if (scope.role === "agent") {
        await api("/api/workflows", scope, {
          method: "POST",
          signal: c.signal,
          body: JSON.stringify({
            id: `proposal-${crypto.randomUUID()}`,
            kind: "propose_journey",
            manifest: plan.manifest,
          }),
        });
      } else {
        await api("/api/resources/apply", scope, {
          method: "POST",
          signal: c.signal,
          headers: {
            "Content-Type": "application/yaml",
            "If-Match": `"${plan.expectedRevision}"`,
          },
          body: stringify(plan.manifest),
        });
      }
      if (!c.signal.aborted) {
        setOpen(false);
        onApplied(
          scope.role === "agent" ? undefined : plan.manifest.metadata.name,
        );
        if (scope.role === "agent") onProposed?.();
      }
    } catch (e) {
      if (!c.signal.aborted) {
        setError(message(e));
        setPlan(null);
      }
    } finally {
      if (!c.signal.aborted) setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button
          variant={resource ? "outline" : "default"}
          size="sm"
          disabled={!editable}
        >
          {resource ? (
            <Settings2 className="size-4" />
          ) : (
            <Plus className="size-4" />
          )}
          {resource ? "Edit definition" : "Define journey"}
        </Button>
      </DialogTrigger>
      <DialogContent className="definition-dialog sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {resource
              ? "Edit critical user journey"
              : "Define critical user journey"}
          </DialogTitle>
          <DialogDescription>
            Describe the user outcome, link SLO definitions and map the steps.
            Review the change before{" "}
            {scope.role === "agent"
              ? "submitting it for owner approval."
              : "saving a new revision."}
          </DialogDescription>
        </DialogHeader>
        <div className="dialog-body space-y-5">
          <Tabs value={mode} onValueChange={changeMode} className="gap-5">
            <TabsList>
              <TabsTrigger value="form" disabled={busy}>
                Definition
              </TabsTrigger>
              <TabsTrigger value="yaml" disabled={busy}>
                YAML
              </TabsTrigger>
            </TabsList>
            <TabsContent value="form">
              <JourneyFields
                value={draft}
                objectives={objectives}
                loading={loading}
                loadError={loadError}
                fixedName={!!resource}
                disabled={busy || !editable}
                onChange={change}
              />
            </TabsContent>
            <TabsContent value="yaml">
              <FormField
                htmlFor="journey-manifest"
                label="CriticalUserJourney manifest (YAML)"
              >
                <Textarea
                  id="journey-manifest"
                  disabled={busy}
                  value={text}
                  spellCheck={false}
                  className="min-h-80 font-mono text-xs"
                  onChange={(e) => {
                    setText(e.target.value);
                    setPlan(null);
                    setError("");
                  }}
                />
              </FormField>
            </TabsContent>
          </Tabs>
          {error ? (
            <p ref={errorRef} role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          {plan ? (
            <section
              ref={planRef}
              aria-label="Journey change review"
              className="space-y-3 rounded-md border bg-muted/30 p-4 text-sm"
            >
              <div className="flex items-center gap-2 font-medium">
                <FileCheck2 className="size-4" />
                Validated · {plan.action}
              </div>
              <p className="text-muted-foreground">
                {plan.manifest.metadata.name} · revision {plan.expectedRevision}{" "}
                →{" "}
                {plan.action === "unchanged"
                  ? plan.expectedRevision
                  : plan.expectedRevision + 1}
              </p>
              <p>
                {plan.manifest.spec.slos.length} linked SLOs ·{" "}
                {plan.manifest.spec.steps.length} journey steps
              </p>
              <details>
                <summary className="cursor-pointer">
                  Review resulting YAML
                </summary>
                <pre className="code-block mt-2">
                  {stringify(plan.manifest)}
                </pre>
              </details>
              {plan.previous ? (
                <details>
                  <summary className="cursor-pointer">
                    Review previous manifest
                  </summary>
                  <pre className="code-block mt-2">
                    {stringify(plan.previous.manifest)}
                  </pre>
                </details>
              ) : null}
              <p className="break-all font-mono text-xs text-muted-foreground">
                SHA-256 {plan.digest}
              </p>
            </section>
          ) : null}
        </div>
        <div className="dialog-actions">
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => changeOpen(false)}
          >
            Cancel
          </Button>
          <Button
            variant="outline"
            onClick={review}
            disabled={busy || !editable}
          >
            {busy ? "Working…" : "Validate and plan"}
          </Button>
          <Button onClick={apply} disabled={busy || !plan || !editable}>
            {scope.role === "agent"
              ? "Submit proposal for review"
              : plan?.action === "unchanged"
                ? "Keep current revision"
                : "Save reviewed journey"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
