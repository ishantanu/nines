import { latencyDuration } from "@/lib/latency-duration";
import { NativeSelect } from "@/components/ui/select";
import { useId, useState } from "react";
import type { IntentDocument } from "@/lib/intent";
import { duration } from "@/lib/history";
import { message } from "@/lib/api";
import {
  measurementDraft,
  sloFromMeasurementDraft,
  type MeasurementDraft,
} from "@/lib/measurement";
import { MeasurementFields } from "./measurement-editor";
import { Button } from "./ui/button";
import { Label } from "./ui/label";
import { Textarea } from "./ui/textarea";

export function goalWithMeasurement(
  goal: IntentDocument,
  draft: MeasurementDraft,
): IntentDocument {
  const manifest = sloFromMeasurementDraft(
    {
      ...draft,
      name: goal.metadata.name,
      displayName: goal.metadata.name,
      project: goal.metadata.project,
      service: goal.spec.service,
      source: goal.spec.source,
      measurement: goal.spec.outcome as "availability" | "latency",
      population: Object.entries(goal.spec.population).map(
        ([label, value]) => ({ label, value }),
      ),
      threshold:
        goal.spec.thresholdSeconds == null
          ? ""
          : latencyDuration(goal.spec.thresholdSeconds),
    },
    String(Number((goal.spec.target * 100).toPrecision(12))),
    duration(goal.spec.windowSeconds),
    [],
  );
  return {
    ...goal,
    spec: {
      ...goal.spec,
      measurement: manifest.spec.measurement,
      stepSeconds: manifest.spec.stepSeconds,
    },
  };
}
export function IntentMeasurementFields({
  draft,
  disabled,
  onChange,
}: {
  draft: MeasurementDraft;
  disabled?: boolean;
  onChange: (draft: MeasurementDraft) => void;
}) {
  return (
    <MeasurementFields
      draft={draft}
      connections={[]}
      hideIdentity
      hidePopulation
      hideType
      disabled={disabled}
      onChange={onChange}
    />
  );
}
export type ExclusionMode = "not_recorded" | "none" | "boundaries";
export function goalWithExclusions(
  goal: IntentDocument,
  mode: ExclusionMode,
  text: string,
): IntentDocument {
  const spec = { ...goal.spec };
  if (mode === "not_recorded") delete spec.exclusions;
  else if (mode === "none") spec.exclusions = { none: true };
  else {
    const boundaries = text
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    if (
      !boundaries.length ||
      boundaries.length > 8 ||
      boundaries.some((line) => new TextEncoder().encode(line).length > 500) ||
      new Set(boundaries.map((line) => line.toLowerCase())).size !==
        boundaries.length
    )
      throw new Error(
        "Enter 1–8 distinct observation boundaries, one per line, up to 500 bytes each.",
      );
    spec.exclusions = { observationBoundaries: boundaries };
  }
  return { ...goal, spec };
}
export function IntentMeasurementEditor({
  goal,
  disabled,
  onSave,
}: {
  goal: IntentDocument;
  disabled?: boolean;
  onSave: (goal: IntentDocument) => Promise<unknown>;
}) {
  const [draft, setDraft] = useState<MeasurementDraft>(() => ({
    ...measurementDraft(),
    metric: goal.spec.measurement?.metric ?? "",
    measurement: goal.spec.outcome as "availability" | "latency",
    step: String(goal.spec.stepSeconds ?? 60),
    scrape: String(goal.spec.measurement?.scrapeIntervalSeconds ?? 15),
    observationPoint: goal.spec.measurement?.observationPoint ?? "",
    countFrom:
      goal.spec.measurement?.countFrom === "histogram"
        ? "histogram"
        : "counter",
    success: Object.entries(goal.spec.measurement?.successLabels ?? {}).map(
      ([label, value]) => ({ label, value }),
    ),
  }));
  const [error, setError] = useState("");
  const exclusionId = useId();
  const [exclusionMode, setExclusionMode] = useState<ExclusionMode>(
    goal.spec.exclusions?.none
      ? "none"
      : goal.spec.exclusions?.observationBoundaries?.length
        ? "boundaries"
        : "not_recorded",
  );
  const [boundaries, setBoundaries] = useState(
    (goal.spec.exclusions?.observationBoundaries ?? []).join("\n"),
  );
  return (
    <details
      className="rounded-md border p-3 text-sm"
      open={!goal.spec.measurement}
    >
      <summary className="cursor-pointer font-medium">
        {goal.spec.measurement
          ? "Edit SLO measurement"
          : "Define SLO measurement"}
      </summary>
      <div className="mt-4 space-y-4">
        <p className="text-xs text-muted-foreground">
          Use the source and population in this conversation. Saving an edit
          clears prior evidence and requires a fresh review.
        </p>
        <IntentMeasurementFields
          draft={draft}
          disabled={disabled}
          onChange={setDraft}
        />
        <div className="space-y-2">
          <Label htmlFor={exclusionId}>Observation exclusions</Label>
          <NativeSelect
            id={exclusionId}
            disabled={disabled}
            value={exclusionMode}
            onChange={(e) => setExclusionMode(e.target.value as ExclusionMode)}
            className="h-11 w-full rounded-md border border-input bg-background px-3 text-sm"
          >
            <option value="not_recorded">Not recorded</option>
            <option value="none">No additional observation exclusions</option>
            <option value="boundaries">Record observation boundaries</option>
          </NativeSelect>
          <p className="text-xs text-muted-foreground">
            Describe what this observer cannot measure. These notes do not
            filter telemetry or discard events; eligible events come from the
            measurement above. Saving requires a new measurement confirmation.
          </p>
          {exclusionMode === "boundaries" && (
            <>
              <Label htmlFor={`${exclusionId}-boundaries`}>
                Boundaries, one per line
              </Label>
              <Textarea
                id={`${exclusionId}-boundaries`}
                disabled={disabled}
                rows={3}
                maxLength={4096}
                value={boundaries}
                onChange={(e) => setBoundaries(e.target.value)}
              />
            </>
          )}
        </div>
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <Button
          size="sm"
          variant="outline"
          disabled={disabled}
          onClick={async () => {
            setError("");
            try {
              await onSave(
                goalWithExclusions(
                  goalWithMeasurement(goal, draft),
                  exclusionMode,
                  boundaries,
                ),
              );
            } catch (e) {
              setError(message(e));
            }
          }}
        >
          Save measurement in goal
        </Button>
      </div>
    </details>
  );
}
