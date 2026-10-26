import { useId, useState } from "react";
import { duration } from "@/lib/history";
import { Plus, Trash2 } from "lucide-react";
import { percent, type Manifest, type SLO } from "@/lib/api";
import {
  journeyDimensions,
  removeJourneyLink,
  removeJourneyStep,
  renameJourneyStep,
} from "@/lib/journeys";
import { FormField, FormSection } from "./ui/form-layout";
import { Input } from "./ui/input";
import { Textarea } from "./ui/textarea";
import { Label } from "./ui/label";
import { Button } from "./ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";

function Choice({
  id,
  value,
  options,
  onChange,
}: {
  id: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
}) {
  const all =
    options.some((o) => o.value === value) || !value
      ? options
      : [...options, { value, label: `${value} (not available)` }];
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger id={id}>
        <SelectValue placeholder="Select an option" />
      </SelectTrigger>
      <SelectContent>
        {all.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
const choices = (values: readonly string[]) =>
  values.map((value) => ({
    value,
    label: value.charAt(0).toUpperCase() + value.slice(1),
  }));
export function JourneyFields({
  value: m,
  objectives,
  loading,
  loadError,
  fixedName,
  disabled,
  onChange,
}: {
  value: Manifest;
  objectives: SLO[];
  loading: boolean;
  loadError: string;
  fixedName: boolean;
  disabled: boolean;
  onChange: (value: Manifest) => void;
}) {
  const prefix = useId();
  const id = (name: string) => `${prefix}-${name}`;
  const spec = (patch: Partial<Manifest["spec"]>) =>
    onChange({ ...m, spec: { ...m.spec, ...patch } });
  const [linkKeys, setLinkKeys] = useState(() =>
    m.spec.slos.map(() => crypto.randomUUID()),
  );
  const [stepKeys, setStepKeys] = useState(() =>
    m.spec.steps.map(() => crypto.randomUUID()),
  );
  const links = m.spec.slos;
  const steps = m.spec.steps;
  const available = objectives.filter((s) => !links.some((l) => l.id === s.id));
  const a = m.spec.agentic;
  return (
    <div className="space-y-6">
      <FormSection
        title="Journey details"
        description="Describe the user outcome and the team responsible for it."
        disabled={disabled}
      >
        <div className="form-grid">
          <FormField htmlFor={id("name")} label="Identifier">
            <Input
              id={id("name")}
              value={m.metadata.name}
              disabled={fixedName}
              maxLength={63}
              placeholder="checkout"
              onChange={(e) =>
                onChange({
                  ...m,
                  metadata: { ...m.metadata, name: e.target.value },
                })
              }
            />
          </FormField>
          <FormField htmlFor={id("project")} label="Project">
            <Input
              id={id("project")}
              value={m.metadata.project}
              maxLength={200}
              onChange={(e) =>
                onChange({
                  ...m,
                  metadata: { ...m.metadata, project: e.target.value },
                })
              }
            />
          </FormField>
          <FormField htmlFor={id("displayName")} label="Display name">
            <Input
              id={id("displayName")}
              value={m.spec.displayName}
              maxLength={200}
              placeholder="Complete checkout"
              onChange={(e) => spec({ displayName: e.target.value })}
            />
          </FormField>
          <FormField htmlFor={id("owner")} label="Owner">
            <Input
              id={id("owner")}
              value={m.spec.owner}
              maxLength={200}
              placeholder="team-commerce"
              onChange={(e) => spec({ owner: e.target.value })}
            />
          </FormField>
          <FormField htmlFor={id("criticality")} label="Criticality">
            <Choice
              id={id("criticality")}
              value={m.spec.criticality}
              options={choices(["critical", "high", "medium"])}
              onChange={(criticality) => spec({ criticality })}
            />
          </FormField>
        </div>
        <FormField htmlFor={id("description")} label="User outcome">
          <Textarea
            id={id("description")}
            value={m.spec.description}
            maxLength={4000}
            placeholder="A customer can pay and receive an order confirmation."
            onChange={(e) => spec({ description: e.target.value })}
          />
        </FormField>
      </FormSection>
      <FormSection
        title="Linked SLOs"
        description="Select existing SLO definitions. Required SLOs determine journey health; advisory SLOs provide context. At least one must be required."
        disabled={disabled}
      >
        {loading ? (
          <p role="status" className="text-sm text-muted-foreground">
            Loading SLO definitions…
          </p>
        ) : null}
        {loadError ? (
          <p role="alert" className="text-sm text-destructive">
            Could not load SLO definitions: {loadError}. You can retry by
            reopening the editor or use YAML.
          </p>
        ) : null}
        {!loading && !loadError && !objectives.length ? (
          <p className="text-sm text-muted-foreground">
            Define an SLO in Service level objectives first, then link it here.
          </p>
        ) : null}
        {links.map((link, i) => (
          <div key={linkKeys[i]} className="space-y-4 rounded-md border p-4">
            <div className="form-grid">
              <FormField htmlFor={id(`slo-${i}`)} label={`SLO ${i + 1}`}>
                <Choice
                  id={id(`slo-${i}`)}
                  value={link.id}
                  options={objectives
                    .filter(
                      (s) =>
                        s.id === link.id || !links.some((l) => l.id === s.id),
                    )
                    .map((s) => ({
                      value: s.id,
                      label: `${s.name} · ${s.id}`,
                    }))}
                  onChange={(next) =>
                    spec({
                      slos: links.map((l, n) =>
                        n === i ? { ...l, id: next } : l,
                      ),
                      steps: steps.map((s) => ({
                        ...s,
                        sloRefs: s.sloRefs.map((ref) =>
                          ref === link.id ? next : ref,
                        ),
                      })),
                    })
                  }
                />
              </FormField>
              <FormField
                htmlFor={id(`dimension-${i}`)}
                label={`Dimension for SLO ${i + 1}`}
              >
                <Choice
                  id={id(`dimension-${i}`)}
                  value={link.dimension}
                  options={choices(journeyDimensions)}
                  onChange={(dimension) =>
                    spec({
                      slos: links.map((l, n) =>
                        n === i ? { ...l, dimension } : l,
                      ),
                    })
                  }
                />
              </FormField>
            </div>
            {objectives
              .filter((s) => s.id === link.id && s.target !== undefined)
              .map((s) => (
                <p
                  key={s.id}
                  className="text-xs leading-5 text-muted-foreground"
                >
                  {s.service} · {s.project} · {percent(s.target, 3)} target ·{" "}
                  {duration(s.windowSeconds)} rolling window
                </p>
              ))}
            <div className="flex flex-wrap items-center justify-between gap-3">
              <Label className="form-check">
                <input
                  type="checkbox"
                  checked={link.required}
                  onChange={(e) =>
                    spec({
                      slos: links.map((l, n) =>
                        n === i ? { ...l, required: e.target.checked } : l,
                      ),
                    })
                  }
                />
                Required for journey health
              </Label>
              <Button
                type="button"
                variant="outline"
                size="sm"
                aria-label={`Remove SLO ${link.id}`}
                onClick={() => {
                  setLinkKeys((keys) => keys.filter((_, n) => n !== i));
                  onChange(removeJourneyLink(m, link.id));
                }}
              >
                <Trash2 className="size-4" />
                Remove SLO
              </Button>
            </div>
          </div>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={loading || !available.length || links.length >= 100}
          onClick={() => {
            setLinkKeys((keys) => [...keys, crypto.randomUUID()]);
            spec({
              slos: [
                ...links,
                {
                  id: available[0].id,
                  dimension: "availability",
                  required: true,
                },
              ],
            });
          }}
        >
          <Plus className="size-4" />
          Link SLO
        </Button>
      </FormSection>
      <FormSection
        title="Journey steps"
        description="Map each step to its SLOs. Every linked SLO must appear in a step. Dependencies describe order and must not form a cycle."
        disabled={disabled}
      >
        {steps.map((step, i) => (
          <fieldset
            key={stepKeys[i]}
            className="min-w-0 space-y-4 rounded-md border p-4"
          >
            <legend className="px-1 text-sm font-medium">Step {i + 1}</legend>
            <div className="form-grid">
              <FormField
                htmlFor={id(`step-id-${i}`)}
                label={`Step ${i + 1} identifier`}
              >
                <Input
                  id={id(`step-id-${i}`)}
                  value={step.id}
                  maxLength={63}
                  onChange={(e) =>
                    onChange(renameJourneyStep(m, i, e.target.value))
                  }
                />
              </FormField>
              <FormField
                htmlFor={id(`step-name-${i}`)}
                label={`Step ${i + 1} name`}
              >
                <Input
                  id={id(`step-name-${i}`)}
                  value={step.name}
                  maxLength={200}
                  placeholder="Confirm payment"
                  onChange={(e) =>
                    spec({
                      steps: steps.map((s, n) =>
                        n === i ? { ...s, name: e.target.value } : s,
                      ),
                    })
                  }
                />
              </FormField>
              <FormField
                htmlFor={id(`step-actor-${i}`)}
                label={`Step ${i + 1} actor`}
              >
                <Choice
                  id={id(`step-actor-${i}`)}
                  value={step.actor}
                  options={choices(["user", "service", "agent", "human"])}
                  onChange={(actor) =>
                    spec({
                      steps: steps.map((s, n) =>
                        n === i
                          ? { ...s, actor: actor as typeof step.actor }
                          : s,
                      ),
                    })
                  }
                />
              </FormField>
            </div>
            <fieldset className="space-y-3">
              <legend className="mb-3 text-sm font-medium">
                SLOs for step {i + 1}
              </legend>
              {!links.length ? (
                <p className="text-sm text-muted-foreground">
                  Link an SLO above to assign it to this step.
                </p>
              ) : null}
              {links.map((l) => (
                <Label key={l.id} className="form-check">
                  <input
                    type="checkbox"
                    checked={step.sloRefs.includes(l.id)}
                    onChange={(e) =>
                      spec({
                        steps: steps.map((s, n) =>
                          n === i
                            ? {
                                ...s,
                                sloRefs: e.target.checked
                                  ? [...s.sloRefs, l.id]
                                  : s.sloRefs.filter((ref) => ref !== l.id),
                              }
                            : s,
                        ),
                      })
                    }
                  />
                  {objectives.find((s) => s.id === l.id)?.name ?? l.id}
                </Label>
              ))}
            </fieldset>
            {steps.length > 1 ? (
              <fieldset className="space-y-3">
                <legend className="mb-3 text-sm font-medium">Depends on</legend>
                {steps.map((other, j) =>
                  j === i ? null : (
                    <Label key={stepKeys[j]} className="form-check">
                      <input
                        type="checkbox"
                        checked={step.dependsOn?.includes(other.id) ?? false}
                        onChange={(e) =>
                          spec({
                            steps: steps.map((s, n) =>
                              n === i
                                ? {
                                    ...s,
                                    dependsOn: e.target.checked
                                      ? [...(s.dependsOn ?? []), other.id]
                                      : s.dependsOn?.filter(
                                          (dep) => dep !== other.id,
                                        ),
                                  }
                                : s,
                            ),
                          })
                        }
                      />
                      {other.name || other.id}
                    </Label>
                  ),
                )}
              </fieldset>
            ) : null}
            <div className="flex justify-end">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={steps.length === 1}
                aria-label={`Remove step ${i + 1}`}
                onClick={() => {
                  setStepKeys((keys) => keys.filter((_, n) => n !== i));
                  onChange(removeJourneyStep(m, i));
                }}
              >
                <Trash2 className="size-4" />
                Remove step
              </Button>
            </div>
          </fieldset>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={steps.length >= 100}
          onClick={() => {
            setStepKeys((keys) => [...keys, crypto.randomUUID()]);
            let n = steps.length + 1;
            while (steps.some((s) => s.id === `step-${n}`)) n++;
            spec({
              steps: [
                ...steps,
                { id: `step-${n}`, name: "", actor: "service", sloRefs: [] },
              ],
            });
          }}
        >
          <Plus className="size-4" />
          Add step
        </Button>
      </FormSection>
      <FormSection
        title="Agent contract"
        description="Optional limits for evaluating reported agent runs. These checks do not execute agents or enforce approval."
        disabled={disabled}
      >
        <Label className="form-check">
          <input
            type="checkbox"
            checked={!!a}
            onChange={(e) =>
              spec({
                agentic: e.target.checked
                  ? { successDefinition: "", approvalRequired: false }
                  : undefined,
              })
            }
          />
          Evaluate agent run evidence
        </Label>
        {a ? (
          <>
            <FormField
              htmlFor={id("success")}
              label="Successful run definition"
            >
              <Textarea
                id={id("success")}
                value={a.successDefinition}
                maxLength={2000}
                onChange={(e) =>
                  spec({ agentic: { ...a, successDefinition: e.target.value } })
                }
              />
            </FormField>
            <div className="form-grid">
              {(
                [
                  {
                    key: "maxDurationSeconds",
                    label: "Maximum duration (seconds)",
                    max: 604800,
                  },
                  {
                    key: "maxCostUSD",
                    label: "Maximum cost per run (USD)",
                    max: 1000000,
                  },
                  { key: "maxSteps", label: "Maximum steps", max: 100000 },
                ] as const
              ).map((f) => (
                <FormField key={f.key} htmlFor={id(f.key)} label={f.label}>
                  <Input
                    id={id(f.key)}
                    type="number"
                    min={0}
                    max={f.max}
                    step={f.key === "maxCostUSD" ? "any" : 1}
                    value={a[f.key] ?? ""}
                    placeholder="No limit"
                    onChange={(e) =>
                      spec({
                        agentic: {
                          ...a,
                          [f.key]:
                            e.target.value === ""
                              ? undefined
                              : Number(e.target.value),
                        },
                      })
                    }
                  />
                </FormField>
              ))}
            </div>
            <Label className="form-check">
              <input
                type="checkbox"
                checked={a.approvalRequired}
                onChange={(e) =>
                  spec({
                    agentic: { ...a, approvalRequired: e.target.checked },
                  })
                }
              />
              Require approval evidence
            </Label>
          </>
        ) : null}
      </FormSection>
    </div>
  );
}
