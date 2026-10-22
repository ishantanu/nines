import { useState } from "react";
import { api, message, type RequestScope } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
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
  onApplied: () => void;
}) {
  const [open, setOpen] = useState(false),
    [text, setText] = useState(initial),
    [plan, setPlan] = useState<ResourcePlan | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const writable = scope.role === "owner" || scope.role === "editor";
  async function review() {
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
      setPlan(p);
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function apply() {
    if (!plan) return;
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
      if (!scope.signal.aborted) {
        setOpen(false);
        onApplied();
      }
    } catch (e) {
      if (!scope.signal.aborted) {
        setError(message(e));
        setPlan(null);
      }
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (busy) return;
        setOpen(v);
        if (v) {
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
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{label}</DialogTitle>
          <DialogDescription>
            Review the YAML and revision before applying. Workspace owners and
            editors can save definitions.
          </DialogDescription>
        </DialogHeader>
        <Label htmlFor="topology-manifest">YAML manifest</Label>
        <Textarea
          id="topology-manifest"
          value={text}
          disabled={busy}
          onChange={(e) => {
            setText(e.target.value);
            setPlan(null);
          }}
          spellCheck={false}
          className="min-h-80 font-mono text-xs"
        />
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
            {busy ? "Working…" : "Validate & plan"}
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
