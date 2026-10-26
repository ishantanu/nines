import { FormField } from "./ui/form-layout";
import { useEffect, useId, useState, useRef } from "react";
import { api, message, type RequestScope } from "@/lib/api";
import type { SLOTemplate, SLOTemplateCatalog } from "@/lib/slo-templates";
import { extensionRequestInputs } from "@/lib/slo-templates";
import { ExtensionInputFields } from "./extension-input-fields";
import { Button } from "./ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";

export function SLOTemplatePicker({
  scope,
  disabled,
  onSelect,
  onBusyChange,
}: {
  scope: RequestScope;
  disabled: boolean;
  onSelect: (template: SLOTemplate) => void;
  onBusyChange?: (busy: boolean) => void;
}) {
  const id = useId();
  const [catalog, setCatalog] = useState<SLOTemplateCatalog | null>(null);
  const [category, setCategory] = useState("request-serving");
  const [selected, setSelected] = useState("");
  const [error, setError] = useState("");
  const [inputs, setInputs] = useState<Record<string, unknown>>({});
  const [rendering, setRendering] = useState(false);
  const request = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    setCatalog(null);
    setSelected("");
    setInputs({});
    setRendering(false);
    setError("");
    api<SLOTemplateCatalog>("/api/slo-templates", scope, {
      signal: controller.signal,
    })
      .then((value) => {
        if (controller.signal.aborted) return;
        if (
          value.tenant !== scope.tenant ||
          value.apiVersion !== "nines.dev/v1alpha1"
        )
          throw new Error("Template catalog scope mismatch");
        setCatalog(value);
      })
      .catch((e) => {
        if (!controller.signal.aborted && !scope.signal.aborted)
          setError(message(e));
      });
    return () => {
      controller.abort();
      request.current?.abort();
    };
  }, [scope]);
  const templates =
    catalog?.templates.filter((t) => t.category === category) ?? [];
  const template = templates.find((t) => t.id === selected);
  async function useTemplate() {
    if (!template || disabled || rendering) return;
    if (!template.extension) {
      onSelect(template);
      return;
    }
    const controller = new AbortController();
    request.current = controller;
    setRendering(true);
    onBusyChange?.(true);
    setError("");
    try {
      const result = await api<{
        apiVersion: string;
        tenant: string;
        templateId: string;
        templateDigest: string;
        manifest: SLOTemplate["manifest"];
      }>(
        `/api/slo-templates/${encodeURIComponent(template.id)}/render`,
        scope,
        {
          method: "POST",
          signal: controller.signal,
          body: JSON.stringify({
            expectedTemplateDigest: template.digest,
            inputs: extensionRequestInputs(template, inputs),
          }),
        },
      );
      if (controller.signal.aborted || scope.signal.aborted) return;
      if (
        result.tenant !== scope.tenant ||
        result.apiVersion !== "nines.dev/v1alpha1" ||
        result.templateId !== template.id ||
        result.templateDigest !== template.digest
      )
        throw new Error("Extension render scope or digest mismatch");
      onSelect({ ...template, manifest: result.manifest });
    } catch (e) {
      if (!controller.signal.aborted && !scope.signal.aborted)
        setError(message(e));
    } finally {
      if (!controller.signal.aborted && !scope.signal.aborted) {
        setRendering(false);
        onBusyChange?.(false);
      }
    }
  }
  return (
    <section
      className="space-y-3 rounded-md border p-4"
      aria-label="SLO templates"
    >
      <div>
        <p className="text-sm font-medium">Start from a template</p>
        <p className="mt-1 text-xs text-muted-foreground">
          Choose an SLI, then adapt its example instrumentation. Templates start
          as experimental SLOs with no paging commitment.
        </p>
      </div>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          Templates unavailable: {error}. You can still define a measurement
          below.
        </p>
      ) : null}
      <div className="form-grid">
        <div>
          <FormField htmlFor={`${id}-category`} label={<> Category </>}>
            <Select
              value={category}
              disabled={disabled || rendering || !catalog}
              onValueChange={(value) => {
                setCategory(value);
                setSelected("");
                setInputs({});
                setError("");
              }}
            >
              <SelectTrigger id={`${id}-category`} className="w-full">
                <SelectValue placeholder="Choose category" />
              </SelectTrigger>
              <SelectContent>
                {catalog?.categories.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.title}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
        </div>
        <div>
          <FormField htmlFor={`${id}-sli`} label={<> SLI template </>}>
            <Select
              value={selected}
              disabled={disabled || rendering || !catalog}
              onValueChange={(value) => {
                setSelected(value);
                setInputs({});
                setError("");
              }}
            >
              <SelectTrigger id={`${id}-sli`} className="w-full">
                <SelectValue placeholder="Choose an SLI" />
              </SelectTrigger>
              <SelectContent>
                {templates.map((t) => (
                  <SelectItem key={t.id} value={t.id}>
                    {t.title}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
        </div>
      </div>
      {template ? (
        <div className="space-y-2 text-sm">
          <p>{template.description}</p>
          {template.extension ? (
            <>
              <p className="text-xs text-muted-foreground">
                Extension {template.extension.name} ·{" "}
                {template.extension.version}
              </p>
              <ExtensionInputFields
                template={template}
                values={inputs}
                disabled={disabled || rendering}
                onChange={(name, value) =>
                  setInputs((prior) => ({ ...prior, [name]: value }))
                }
              />
              {template.evaluation ? (
                <details className="rounded-md border p-3 text-xs">
                  <summary className="cursor-pointer font-medium">
                    Evaluation contract · {template.evaluation.version}
                  </summary>
                  <p className="mt-2">{template.evaluation.rubric}</p>
                  <p className="mt-2">{template.evaluation.sampling}</p>
                  <p className="mt-2">
                    Unknown verdicts: {template.evaluation.unknownVerdict}.
                    Missing verdicts: {template.evaluation.missingVerdict}.
                  </p>
                </details>
              ) : null}
            </>
          ) : null}
          <dl className="grid gap-1 text-xs">
            <dt className="font-medium">Eligible events</dt>
            <dd>{template.eligibleEvents}</dd>
            <dt className="font-medium">Good events</dt>
            <dd>{template.goodEvents}</dd>
          </dl>
          <ul className="list-disc space-y-1 pl-4 text-xs text-muted-foreground">
            {template.requirements.map((r) => (
              <li key={r}>{r}</li>
            ))}
          </ul>
          <p className="text-xs text-muted-foreground">
            Using this template replaces the unsaved measurement settings below.
            Targets and metric names are examples; telemetry has not been
            verified.
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled || rendering}
            onClick={useTemplate}
          >
            Use template
          </Button>
        </div>
      ) : null}
    </section>
  );
}
