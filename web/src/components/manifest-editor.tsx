import { useState } from "react";
import { stringify } from "yaml";
import { RESOURCE_API_VERSION } from "@/lib/definitions";
import { Code2, FileCheck2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  api,
  type RequestScope,
  message,
  type Manifest,
  type Plan,
  type Revision,
} from "@/lib/api";
const starter: Manifest = {
  apiVersion: RESOURCE_API_VERSION,
  kind: "CriticalUserJourney",
  metadata: { name: "new-journey", project: "production" },
  spec: {
    displayName: "My critical user journey",
    description: "Describe the outcome your user needs.",
    owner: "team-platform",
    criticality: "high",
    slos: [
      { id: "api-availability", dimension: "availability", required: true },
    ],
    steps: [
      {
        id: "request",
        name: "Complete request",
        actor: "user",
        sloRefs: ["api-availability"],
      },
    ],
  },
};
export function ManifestEditor({
  scope,
  resource,
  onApplied,
  onProposed,
}: {
  scope: RequestScope;
  resource?: Revision;
  onApplied: () => void;
  onProposed?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [plan, setPlan] = useState<Plan | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  function changeOpen(value: boolean) {
    if (busy) return;
    setOpen(value);
    if (value) {
      setText(stringify(resource?.manifest ?? starter));
      setPlan(null);
      setError("");
    }
  }
  async function review() {
    setBusy(true);
    setError("");
    try {
      const result = await api<Plan>("/api/resources/plan", scope, {
        method: "POST",
        headers: { "Content-Type": "application/yaml" },
        body: text,
      });
      if (result.manifest.kind !== "CriticalUserJourney")
        throw new Error("Use a CriticalUserJourney manifest in this editor.");
      setPlan(result);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function apply() {
    if (!plan) return;
    setBusy(true);
    setError("");
    try {
      if (scope.role === "agent") {
        await api("/api/workflows", scope, {
          method: "POST",
          body: JSON.stringify({
            id: `proposal-${Date.now()}`,
            kind: "propose_journey",
            manifest: plan.manifest,
          }),
        });
      } else {
        await api("/api/resources/apply", scope, {
          method: "POST",
          headers: {
            "Content-Type": "application/yaml",
            "If-Match": `"${plan.expectedRevision}"`,
          },
          body: text,
        });
      }
      setOpen(false);
      onApplied();
      if (scope.role === "agent") onProposed?.();
    } catch (e) {
      setError(message(e));
      setPlan(null);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button variant={resource ? "outline" : "default"} size="sm">
          <Code2 className="size-4" />
          {resource ? "Edit manifest" : "Define journey"}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {resource
              ? "Edit journey as code"
              : "Define a critical user journey"}
          </DialogTitle>
          <DialogDescription>
            Validate the graph and linked objectives, review the change, then
            {scope.role === "agent"
              ? " submit a proposal for owner review."
              : " apply a versioned manifest."}
          </DialogDescription>
        </DialogHeader>
        <Label htmlFor="manifest">YAML manifest</Label>
        <Textarea
          id="manifest"
          disabled={busy}
          value={text}
          spellCheck={false}
          onChange={(e) => {
            setText(e.target.value);
            setPlan(null);
          }}
          className="min-h-80 font-mono text-xs"
        />
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {plan ? (
          <div className="rounded-lg border bg-slate-50 p-4 text-sm">
            <div className="flex items-center gap-2 font-medium">
              <FileCheck2 className="size-4 text-emerald-600" />
              Validated · {plan.action}
            </div>
            <p className="mt-1 text-muted-foreground">
              {plan.manifest.metadata.name} · revision {plan.expectedRevision} →{" "}
              {plan.action === "unchanged"
                ? plan.expectedRevision
                : plan.expectedRevision + 1}
            </p>
            {plan.previous ? (
              <details className="mt-3">
                <summary className="cursor-pointer">
                  Review previous manifest
                </summary>
                <pre className="code-block mt-2">
                  {stringify(plan.previous.manifest)}
                </pre>
              </details>
            ) : null}
            <p className="mt-2 break-all font-mono text-xs text-muted-foreground">
              SHA-256 {plan.digest}
            </p>
          </div>
        ) : null}
        <div className="dialog-actions">
          <Button
            variant="outline"
            onClick={review}
            disabled={busy || !text.trim()}
          >
            {busy ? "Working…" : "Validate & plan"}
          </Button>
          <Button
            onClick={apply}
            disabled={
              busy ||
              !plan ||
              scope.role === "viewer" ||
              (scope.role === "agent" && !onProposed)
            }
          >
            {scope.role === "agent"
              ? "Submit proposal for review"
              : plan?.action === "unchanged"
                ? "Keep current revision"
                : scope.role === "owner" || scope.role === "editor"
                  ? "Apply manifest"
                  : "Owner role required to apply"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
