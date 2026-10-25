import type { ErrorBudgetDecision } from "@/lib/api";
import { percent } from "@/lib/api";
import { Card, CardHeader, CardTitle, CardContent } from "./ui/card";
export function BudgetDecisionPanel({
  decision,
  frozen = false,
}: {
  decision: ErrorBudgetDecision;
  frozen?: boolean;
}) {
  const label =
    {
      not_configured: "No response policy",
      insufficient_evidence: "Insufficient evidence",
      review_required: "Owner review required",
      blocked: "Budget exhausted",
      proceed: "Within budget policy",
    }[decision.state] || "Decision unavailable";
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b p-4">
        <CardTitle className="text-sm">
          {frozen
            ? "Budget evidence at proposal preparation"
            : "Error-budget response"}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 p-4 text-sm">
        <p className="font-medium">{label}</p>
        <p>{decision.message}</p>
        <dl className="grid gap-3 sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground">Remaining budget</dt>
            <dd>{percent(decision.budgetRemainingFraction, 2)}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Responsible owner</dt>
            <dd>{decision.owner || "Unassigned"}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Definition revision</dt>
            <dd>{decision.revision}</dd>
          </div>
        </dl>
        {decision.runbookURL && /^https?:\/\//i.test(decision.runbookURL) ? (
          <a
            className="inline-block underline underline-offset-4"
            href={decision.runbookURL}
            target="_blank"
            rel="noopener noreferrer"
          >
            Open runbook
          </a>
        ) : null}
        {decision.evidence ? (
          <p className="text-xs text-muted-foreground">
            {percent(decision.evidence.coverage, 2)} window coverage ·{" "}
            {decision.evidence.total.toLocaleString()} eligible events /{" "}
            {decision.evidence.minimumEvents.toLocaleString()} minimum ·{" "}
            {decision.evidence.freshness.state} samples
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">
          Evidence cutoff: {new Date(decision.evaluatedAt).toISOString()}.{" "}
          {frozen
            ? "This saved decision is historical. Read current evidence before release."
            : "A budget decision grants no deployment permission. CI can enforce it with nines budget --check."}
        </p>
      </CardContent>
    </Card>
  );
}
