import { useState } from "react";
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
          : String(goal.spec.thresholdSeconds),
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
              await onSave(goalWithMeasurement(goal, draft));
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
