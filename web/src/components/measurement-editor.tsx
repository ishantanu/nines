import { useEffect, useId, useRef, useState } from "react";
import { parse, stringify } from "yaml";
import { api, canEdit, message, type RequestScope } from "@/lib/api";
import {
  bindingKind,
  bindingStarter,
  type BindingManifest,
} from "@/lib/topology";
import {
  bindingFromDraft,
  measurementDraft,
  type MeasurementDraft,
  type LabelFilter,
} from "@/lib/measurement";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

export function MeasurementFilters({
  title,
  rows,
  disabled,
  onChange,
}: {
  title: string;
  rows: LabelFilter[];
  disabled?: boolean;
  onChange: (rows: LabelFilter[]) => void;
}) {
  const id = useId();
  return (
    <fieldset className="space-y-2" disabled={disabled}>
      <legend className="mb-2 text-sm font-medium">{title}</legend>
      {rows.map((row, index) => (
        <div
          key={index}
          className="grid grid-cols-[1fr_1fr_auto] items-end gap-2"
        >
          <div>
            <Label className="text-xs" htmlFor={`${id}-${index}-label`}>
              Label {index + 1}
            </Label>
            <Input
              id={`${id}-${index}-label`}
              className="mt-1"
              value={row.label}
              onChange={(e) =>
                onChange(
                  rows.map((r, i) =>
                    i === index ? { ...r, label: e.target.value } : r,
                  ),
                )
              }
            />
          </div>
          <div>
            <Label className="text-xs" htmlFor={`${id}-${index}-value`}>
              Value {index + 1}
            </Label>
            <Input
              id={`${id}-${index}-value`}
              className="mt-1"
              value={row.value}
              onChange={(e) =>
                onChange(
                  rows.map((r, i) =>
                    i === index ? { ...r, value: e.target.value } : r,
                  ),
                )
              }
            />
          </div>
          <Button
            variant="outline"
            size="sm"
            aria-label={`Remove ${title.toLowerCase()} filter ${index + 1}`}
            onClick={() => onChange(rows.filter((_, i) => i !== index))}
          >
            Remove
          </Button>
        </div>
      ))}
      <Button
        variant="outline"
        size="sm"
        disabled={disabled || rows.length >= 16}
        onClick={() => onChange([...rows, { label: "", value: "" }])}
      >
        Add {title.toLowerCase()} filter
      </Button>
    </fieldset>
  );
}
export function MeasurementFields({
  draft,
  connections,
  disabled,
  fixedName,
  hideIdentity,
  hidePopulation,
  hideType,
  onChange,
}: {
  draft: MeasurementDraft;
  connections: { id: string; kind: string }[];
  disabled?: boolean;
  fixedName?: boolean;
  hideIdentity?: boolean;
  hidePopulation?: boolean;
  hideType?: boolean;
  onChange: (draft: MeasurementDraft) => void;
}) {
  const id = useId();
  const field = <K extends keyof MeasurementDraft>(
    key: K,
    value: MeasurementDraft[K],
  ) => onChange({ ...draft, [key]: value });
  const fields: {
    key:
      | "name"
      | "displayName"
      | "project"
      | "service"
      | "metric"
      | "step"
      | "scrape";
    label: string;
    type?: string;
  }[] = [
    { key: "name", label: "Identifier" },
    { key: "displayName", label: "Display name" },
    { key: "project", label: "Project" },
    { key: "service", label: "Service" },
    {
      key: "metric",
      label:
        draft.measurement === "latency"
          ? "Histogram metric family (seconds)"
          : "Metric name",
    },
    { key: "step", label: "Measurement interval (seconds)", type: "number" },
    {
      key: "scrape",
      label: "Source scrape interval (seconds)",
      type: "number",
    },
  ];
  return (
    <fieldset className="space-y-5" disabled={disabled}>
      <div className="grid gap-3 sm:grid-cols-2">
        {fields
          .filter(
            (f) =>
              !hideIdentity || ["metric", "step", "scrape"].includes(f.key),
          )
          .map((f) => (
            <div key={f.key}>
              <Label htmlFor={`${id}-${f.key}`}>{f.label}</Label>
              <Input
                id={`${id}-${f.key}`}
                className="mt-1"
                type={f.type ?? "text"}
                value={draft[f.key]}
                disabled={disabled || (f.key === "name" && fixedName)}
                onChange={(e) => field(f.key, e.target.value)}
              />
            </div>
          ))}
        {!hideIdentity ? (
          <div>
            <Label htmlFor={`${id}-source`}>Metrics connection</Label>
            <Select
              disabled={disabled}
              value={draft.source}
              onValueChange={(v) => field("source", v)}
            >
              <SelectTrigger id={`${id}-source`} className="mt-1 w-full">
                <SelectValue placeholder="Select a connection" />
              </SelectTrigger>
              <SelectContent>
                {connections.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.id}
                    {c.kind ? ` · ${c.kind}` : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : null}
        {!hideType ? (
          <div>
            <Label htmlFor={`${id}-measurement`}>
              What do you want to measure?
            </Label>
            <Select
              disabled={disabled}
              value={draft.measurement}
              onValueChange={(v) =>
                field("measurement", v as MeasurementDraft["measurement"])
              }
            >
              <SelectTrigger id={`${id}-measurement`} className="mt-1 w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="availability">Successful events</SelectItem>
                <SelectItem value="latency">
                  Events below a latency threshold
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        ) : null}
      </div>
      {!hidePopulation ? (
        <div className="space-y-2">
          <MeasurementFilters
            title="Population"
            rows={draft.population}
            disabled={disabled}
            onChange={(v) => field("population", v)}
          />
          <p className="text-xs text-muted-foreground">
            Exact label matches select eligible events. Without filters, every
            series in this metric family is eligible.
          </p>
        </div>
      ) : null}
      {draft.measurement === "availability" ? (
        <div className="space-y-3">
          <MeasurementFilters
            title="Success"
            rows={draft.success}
            disabled={disabled}
            onChange={(v) => field("success", v)}
          />
          <p className="text-xs text-muted-foreground">
            These additional label matches define good events within the
            population. Values are literal strings, not regular expressions.
          </p>
          <div>
            <Label htmlFor={`${id}-count`}>Event count comes from</Label>
            <Select
              disabled={disabled}
              value={draft.countFrom}
              onValueChange={(v) =>
                field("countFrom", v as MeasurementDraft["countFrom"])
              }
            >
              <SelectTrigger id={`${id}-count`} className="mt-1 w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="counter">Counter</SelectItem>
                <SelectItem value="histogram">
                  Classic histogram count
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      ) : !hideType ? (
        <div>
          <Label htmlFor={`${id}-threshold`}>
            Good events complete within (seconds)
          </Label>
          <Input
            id={`${id}-threshold`}
            className="mt-1"
            type="number"
            step="any"
            min="0"
            value={draft.threshold}
            onChange={(e) => field("threshold", e.target.value)}
          />
          <p className="mt-2 text-xs text-muted-foreground">
            Requires a classic histogram with the exact threshold bucket for
            every population. Quantiles and native histograms are not supported.
          </p>
        </div>
      ) : null}
      <div>
        <Label htmlFor={`${id}-observation`}>
          Where is this measured, and what does it miss?
        </Label>
        <Textarea
          id={`${id}-observation`}
          className="mt-1"
          value={draft.observationPoint}
          onChange={(e) => field("observationPoint", e.target.value)}
          placeholder="For example: completed requests at the API server, excluding attempts that never reach it."
        />
      </div>
      <p className="text-xs text-muted-foreground">
        Match the source scrape interval to your telemetry configuration. Each
        measurement interval must contain at least two scrapes.
      </p>
    </fieldset>
  );
}
interface BindingPlan {
  action: string;
  expectedRevision: number;
  digest: string;
  manifest: BindingManifest;
}
export function MeasurementEditor({
  scope,
  initial,
  existing = false,
  disabled = false,
  label = "Define measurement",
  onSaved,
}: {
  scope: RequestScope;
  initial?: BindingManifest;
  existing?: boolean;
  disabled?: boolean;
  label?: string;
  onSaved: (name: string) => void;
}) {
  const [open, setOpen] = useState(false),
    [mode, setMode] = useState("form");
  const [draft, setDraft] = useState(() => measurementDraft(initial));
  const [text, setText] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const [connections, setConnections] = useState<
    { id: string; kind: string }[]
  >([]);
  const [plan, setPlan] = useState<BindingPlan | null>(null);
  const controller = useRef(new AbortController());
  const yamlID = useId();
  useEffect(() => {
    setOpen(false);
    setPlan(null);
    setBusy(false);
  }, [scope]);
  useEffect(() => {
    const c = new AbortController();
    controller.current = c;
    if (open)
      api<{ connections: { id: string; kind: string }[] }>(
        "/api/connections",
        scope,
        { signal: c.signal },
      )
        .then((r) => {
          if (!c.signal.aborted) setConnections(r.connections);
        })
        .catch((e) => {
          if (!c.signal.aborted && !scope.signal.aborted) setError(message(e));
        });
    return () => c.abort();
  }, [scope, open]);
  function changeOpen(value: boolean) {
    if (busy) return;
    setOpen(value);
    if (value) {
      const binding = initial ?? bindingStarter();
      const d = measurementDraft(binding);
      if (!initial) {
        d.name = "";
        d.displayName = "";
        d.service = "";
        d.source = "";
        d.success = [];
        d.observationPoint = "";
      }
      setDraft(d);
      setText(stringify(binding));
      setMode("form");
      setPlan(null);
      setError("");
      setConnections([]);
    }
  }
  function document(): BindingManifest {
    const d =
      mode === "form"
        ? bindingFromDraft(draft)
        : (parse(text) as BindingManifest);
    if (!d || d.kind !== bindingKind)
      throw new Error("Expected a MeasurementBinding YAML definition.");
    return d;
  }
  async function review() {
    const signal = controller.current.signal;
    setBusy(true);
    setError("");
    setPlan(null);
    try {
      const result = await api<BindingPlan>("/api/resources/plan", scope, {
        method: "POST",
        signal,
        headers: { "Content-Type": "application/yaml" },
        body: stringify(document()),
      });
      if (result.manifest.kind !== bindingKind)
        throw new Error("Expected a measurement definition plan.");
      if (!signal.aborted && !scope.signal.aborted) setPlan(result);
    } catch (e) {
      if (!signal.aborted && !scope.signal.aborted) setError(message(e));
    } finally {
      if (!signal.aborted) setBusy(false);
    }
  }
  async function apply() {
    if (!plan || !canEdit(scope)) return;
    const signal = controller.current.signal;
    setBusy(true);
    setError("");
    try {
      await api("/api/resources/apply", scope, {
        method: "POST",
        signal,
        headers: {
          "Content-Type": "application/yaml",
          "If-Match": `"${plan.expectedRevision}"`,
        },
        body: stringify(plan.manifest),
      });
      if (!signal.aborted && !scope.signal.aborted) {
        setOpen(false);
        onSaved(plan.manifest.metadata.name);
      }
    } catch (e) {
      if (!signal.aborted && !scope.signal.aborted) {
        setError(message(e));
        setPlan(null);
      }
    } finally {
      if (!signal.aborted) setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline" disabled={disabled}>
          {label}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{label}</DialogTitle>
          <DialogDescription>
            Specify the eligible events and success criteria. Save after review,
            then verify telemetry when creating an SLO. Saving a measurement
            does not create an SLO or deploy rules.
          </DialogDescription>
        </DialogHeader>
        <Tabs
          value={mode}
          onValueChange={(value) => {
            try {
              if (value === "yaml")
                setText(stringify(bindingFromDraft(draft, false)));
              else {
                const d = document();
                setDraft(measurementDraft(d));
              }
              setMode(value);
              setPlan(null);
              setError("");
            } catch (e) {
              setError(message(e));
            }
          }}
        >
          <TabsList>
            <TabsTrigger value="form" disabled={busy}>
              Measurement
            </TabsTrigger>
            <TabsTrigger value="yaml" disabled={busy}>
              Advanced YAML
            </TabsTrigger>
          </TabsList>
          <TabsContent value="form">
            <MeasurementFields
              draft={draft}
              connections={connections}
              disabled={busy}
              fixedName={existing}
              onChange={(d) => {
                setDraft(d);
                setPlan(null);
              }}
            />
          </TabsContent>
          <TabsContent value="yaml">
            <Label htmlFor={yamlID}>MeasurementBinding manifest</Label>
            <Textarea
              id={yamlID}
              value={text}
              disabled={busy}
              onChange={(e) => {
                setText(e.target.value);
                setPlan(null);
              }}
              className="mt-2 min-h-80 font-mono text-xs"
              spellCheck={false}
            />
          </TabsContent>
        </Tabs>
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {plan ? (
          <div className="space-y-2 rounded-md border p-3 text-sm">
            <p>
              {plan.action} · {plan.manifest.metadata.name} · revision{" "}
              {plan.expectedRevision} →{" "}
              {plan.action === "unchanged"
                ? plan.expectedRevision
                : plan.expectedRevision + 1}
            </p>
            <details>
              <summary className="cursor-pointer">
                Review measurement YAML
              </summary>
              <pre className="code-block mt-2">{stringify(plan.manifest)}</pre>
            </details>
            <p className="break-all font-mono text-xs">SHA-256 {plan.digest}</p>
            <p className="text-xs text-muted-foreground">
              This saves a reusable definition. Metric presence, event semantics
              and histogram boundaries have not yet been verified.
            </p>
          </div>
        ) : null}
        <div className="dialog-actions">
          <Button variant="outline" disabled={busy} onClick={review}>
            {busy ? "Working…" : "Review measurement"}
          </Button>
          <Button disabled={busy || !plan || !canEdit(scope)} onClick={apply}>
            {canEdit(scope)
              ? "Save reviewed measurement"
              : "Owner or editor required"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
