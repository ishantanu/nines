import { Label } from "./ui/label";
import type {
  SystemOneAssessment,
  SystemOneRoute,
} from "@/lib/intent-conversations";

export function SystemOneConsent({
  route,
  checked,
  disabled,
  onChange,
}: {
  route: SystemOneRoute;
  checked: boolean;
  disabled: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <Label className="flex items-start gap-2">
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>
        Send this run’s goal description and observation point to{" "}
        {route.provider} ({route.model}) for optional category and definition
        advice. This is separate from model-directed preparation.
      </span>
    </Label>
  );
}
const percent = (value: number) => `${(value * 100).toFixed(1)}%`;
export function SystemOneAdvice({
  assessment,
  route,
}: {
  assessment: SystemOneAssessment;
  route?: SystemOneRoute;
}) {
  const judgments = assessment.judgments;
  return (
    <section
      aria-label="System One assessment"
      className="my-3 space-y-2 rounded-md border p-3 text-xs"
    >
      <h4 className="font-medium">System One advice</h4>
      <p className="text-muted-foreground">
        {assessment.resolvedModel ?? route?.model ?? "Jev"} ·{" "}
        {assessment.status === "assessed"
          ? "Assessment recorded"
          : "Unavailable; preparation continues"}
      </p>
      {judgments ? (
        <>
          <dl className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">
                Suggested template category
              </dt>
              <dd>{judgments.category.choice.replaceAll("-", " ")}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Category confidence</dt>
              <dd>{percent(judgments.category.confidence)}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">
                Eligible population defined
              </dt>
              <dd>{percent(judgments.populationDefined)} probability</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Good events defined</dt>
              <dd>{percent(judgments.goodEventsDefined)} probability</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">
                Observation point defined
              </dt>
              <dd>{percent(judgments.observationPointDefined)} probability</dd>
            </div>
          </dl>
          <p className="text-muted-foreground">
            These probabilities describe definition clarity, not SLI values or
            measurement quality. Review the definition and verify telemetry
            before confirming it.
          </p>
        </>
      ) : (
        <p className="text-muted-foreground">
          {assessment.errorCode === "system_one_paused"
            ? "Jev is paused by a platform administrator. "
            : assessment.errorCode === "system_one_budget_exhausted"
              ? "The tenant’s daily Jev request limit has been reached. "
              : assessment.errorCode === "system_one_policy_unavailable"
                ? "Jev policy storage is unavailable. "
                : "No judgment is available. "}
          Continue with deterministic validation and human review.
        </p>
      )}
      <details>
        <summary className="cursor-pointer">Assessment receipt</summary>
        <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all">
          {JSON.stringify({ route, ...assessment }, null, 2)}
        </pre>
      </details>
    </section>
  );
}
