import { useRef, useState } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { Upload } from "lucide-react";
import { canEdit, message, type RequestScope } from "@/lib/api";
import {
  applyOne,
  convert,
  levelLabel,
  noteCounts,
  planOne,
  type ImportState,
  type MigrationOptions,
  type MigrationResult,
} from "@/lib/migration";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { FormField } from "@/components/ui/form-layout";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

const defaults: MigrationOptions = {
  project: "production",
  source: "primary",
  lifecycle: "experimental",
  windowDays: "30",
};

// Migration is a reviewed, per-SLO flow: convert, read every note, plan each
// definition, then apply only what you selected. Nothing is saved by converting.
export function MigrationImport({
  scope,
  onApplied,
}: {
  scope: RequestScope;
  onApplied: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [tool, setTool] = useState<"sloth" | "pyrra">("sloth");
  const [content, setContent] = useState("");
  const [options, setOptions] = useState(defaults);
  const [result, setResult] = useState<MigrationResult | null>(null);
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [states, setStates] = useState<Record<string, ImportState>>({});
  const [ack, setAck] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const controller = useRef(new AbortController());
  const editable = canEdit(scope);

  function reset() {
    setResult(null);
    setSelected({});
    setStates({});
    setAck(false);
    setError("");
  }
  async function run() {
    controller.current = new AbortController();
    setBusy(true);
    reset();
    try {
      const r = await convert(
        scope,
        tool,
        content,
        options,
        controller.current.signal,
      );
      setResult(r);
      setSelected(Object.fromEntries(r.definitions.map((d) => [d.name, true])));
      setStates(
        Object.fromEntries(
          r.definitions.map((d) => [d.name, { step: "ready" }]),
        ),
      );
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const chosen = result?.definitions.filter((d) => selected[d.name]) ?? [];
  async function planSelected() {
    setBusy(true);
    setAck(false);
    const next = { ...states };
    for (const d of chosen)
      next[d.name] = await planOne(scope, d, controller.current.signal);
    setStates(next);
    setBusy(false);
  }
  const planned = chosen.filter((d) => states[d.name]?.step === "planned");
  async function applySelected() {
    if (!ack || !editable) return;
    setBusy(true);
    const next = { ...states };
    for (const d of planned) {
      const s = states[d.name];
      if (s?.step === "planned")
        next[d.name] = await applyOne(
          scope,
          d,
          s.expectedRevision,
          controller.current.signal,
        );
    }
    setStates(next);
    setBusy(false);
    if (planned.some((d) => next[d.name]?.step === "applied")) onApplied();
  }
  const upload = async (file?: File) => {
    if (!file) return;
    if (file.size > 1 << 20) return setError("Choose a file of at most 1 MiB.");
    setContent(await file.text());
    reset();
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        if (!v) controller.current.abort();
      }}
    >
      <DialogTrigger asChild>
        <Button variant="outline">
          <Upload className="size-4" aria-hidden="true" />
          Import from Sloth or Pyrra
        </Button>
      </DialogTrigger>
      <DialogContent className="definition-dialog sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Import from Sloth or Pyrra</DialogTitle>
          <DialogDescription>
            Convert existing definitions, read what changed, then plan and apply
            each SLO. Converting saves nothing.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="form-grid">
            <FormField htmlFor="migration-tool" label="Source tool">
              <NativeSelect
                id="migration-tool"
                value={tool}
                onChange={(e) => {
                  setTool(e.target.value as "sloth" | "pyrra");
                  reset();
                }}
              >
                <option value="sloth">
                  Sloth (prometheus/v1 or PrometheusServiceLevel)
                </option>
                <option value="pyrra">Pyrra (ServiceLevelObjective)</option>
              </NativeSelect>
            </FormField>
            <FormField htmlFor="migration-lifecycle" label="Lifecycle">
              <NativeSelect
                id="migration-lifecycle"
                value={options.lifecycle}
                onChange={(e) =>
                  setOptions({
                    ...options,
                    lifecycle: e.target.value as MigrationOptions["lifecycle"],
                  })
                }
              >
                <option value="experimental">
                  Experimental: measure, do not page
                </option>
                <option value="aspirational">Aspirational</option>
                <option value="committed">Committed: pages immediately</option>
              </NativeSelect>
            </FormField>
            <FormField htmlFor="migration-project" label="Project">
              <Input
                id="migration-project"
                value={options.project}
                onChange={(e) =>
                  setOptions({ ...options, project: e.target.value })
                }
              />
            </FormField>
            <FormField htmlFor="migration-source" label="Metrics connection">
              <Input
                id="migration-source"
                value={options.source}
                onChange={(e) =>
                  setOptions({ ...options, source: e.target.value })
                }
              />
            </FormField>
            {tool === "sloth" ? (
              <FormField
                htmlFor="migration-window"
                label="SLO window (days, Sloth only)"
              >
                <Input
                  id="migration-window"
                  inputMode="numeric"
                  value={options.windowDays}
                  onChange={(e) =>
                    setOptions({ ...options, windowDays: e.target.value })
                  }
                />
              </FormField>
            ) : null}
          </div>
          {options.lifecycle === "committed" ? (
            <p role="note" className="text-sm text-destructive">
              Committed SLOs enable Nines burn alerts straight away. If your
              Sloth or Pyrra rules are still running, you will be paged twice.
            </p>
          ) : null}
          <FormField
            htmlFor="migration-content"
            label="Definitions (YAML, one or more documents)"
          >
            <Textarea
              id="migration-content"
              className="min-h-40 font-mono text-xs"
              spellCheck={false}
              value={content}
              onChange={(e) => {
                setContent(e.target.value);
                reset();
              }}
            />
          </FormField>
          <div className="flex flex-wrap items-center gap-2">
            <Label className="text-sm">
              <input
                type="file"
                accept=".yaml,.yml,text/yaml"
                aria-label="Upload definitions"
                onChange={(e) => upload(e.target.files?.[0])}
              />
            </Label>
            <Button
              type="button"
              disabled={busy || !content.trim()}
              onClick={run}
            >
              {busy && !result ? "Converting…" : "Convert"}
            </Button>
          </div>
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          {result ? (
            <section aria-label="Conversion result" className="space-y-4">
              <Heading level={3}>
                {result.definitions.length} converted, {result.skipped.length}{" "}
                skipped
              </Heading>
              {result.notes.map((n, i) => (
                <Text key={i} as="p" size="sm" color="secondary">
                  {n.message}
                </Text>
              ))}
              {result.skipped.length ? (
                <div className="space-y-1 rounded-md border p-3">
                  <Heading level={4}>Skipped: not converted</Heading>
                  <ul className="space-y-1 text-sm">
                    {result.skipped.map((s) => (
                      <li key={s.origin}>
                        <strong>{s.origin}</strong>: {s.reason}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
              <ul className="space-y-3">
                {result.definitions.map((d) => {
                  const counts = noteCounts(d.notes);
                  const s = states[d.name] ?? { step: "ready" };
                  return (
                    <li
                      key={d.name}
                      className="space-y-2 rounded-md border p-3"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <Label className="flex items-center gap-2 font-medium">
                          <input
                            type="checkbox"
                            checked={!!selected[d.name]}
                            disabled={busy}
                            onChange={(e) =>
                              setSelected({
                                ...selected,
                                [d.name]: e.target.checked,
                              })
                            }
                          />
                          {d.name}
                        </Label>
                        <Text as="span" size="sm" color="secondary">
                          from {d.origin}
                        </Text>
                        {counts.lossy ? (
                          <Badge variant="destructive">
                            {counts.lossy} not carried over
                          </Badge>
                        ) : null}
                        {counts.review ? (
                          <Badge variant="outline">
                            {counts.review} to review
                          </Badge>
                        ) : null}
                        {s.step === "planned" ? (
                          <Badge>{s.action}</Badge>
                        ) : null}
                        {s.step === "applied" ? <Badge>Applied</Badge> : null}
                      </div>
                      {s.step === "failed" ? (
                        <p role="alert" className="text-sm text-destructive">
                          {s.error}
                        </p>
                      ) : null}
                      {s.step === "planned"
                        ? s.warnings.map((w, i) => (
                            <p
                              key={i}
                              className="text-xs text-muted-foreground"
                            >
                              {w}
                            </p>
                          ))
                        : null}
                      <details>
                        <summary className="cursor-pointer text-sm">
                          Notes and definition
                        </summary>
                        <ul className="mt-2 space-y-1 text-sm">
                          {d.notes.map((n, i) => (
                            <li key={i}>
                              <Badge
                                variant={
                                  n.level === "lossy"
                                    ? "destructive"
                                    : "outline"
                                }
                              >
                                {levelLabel[n.level]}
                              </Badge>{" "}
                              {n.message}
                            </li>
                          ))}
                        </ul>
                        <pre tabIndex={0} className="code-block mt-2">
                          {d.yaml}
                        </pre>
                      </details>
                    </li>
                  );
                })}
              </ul>
              {result.definitions.length ? (
                <div className="space-y-3 border-t pt-3">
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      disabled={busy || chosen.length === 0}
                      onClick={planSelected}
                    >
                      Plan {chosen.length} selected
                    </Button>
                  </div>
                  {planned.length ? (
                    <>
                      <Label className="flex items-start gap-2 text-sm">
                        <input
                          type="checkbox"
                          checked={ack}
                          onChange={(e) => setAck(e.target.checked)}
                        />
                        I reviewed the converted queries and every note. Rate
                        queries became per-minute counts, and alert thresholds
                        are standard burn-rate policies.
                      </Label>
                      <Button
                        disabled={busy || !ack || !editable}
                        onClick={applySelected}
                      >
                        Apply {planned.length} planned
                      </Button>
                      {!editable ? (
                        <p className="text-xs text-muted-foreground">
                          Your role can convert and plan but not apply.
                        </p>
                      ) : null}
                    </>
                  ) : null}
                </div>
              ) : null}
              <ul className="list-disc space-y-1 pl-5">
                {result.limitations.map((l) => (
                  <li key={l}>
                    <Text as="span" size="sm" color="secondary">
                      {l}
                    </Text>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}
