import { useEffect, useState } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { ProgressBar } from "@astryxdesign/core/ProgressBar";
import { api, message, type RequestScope } from "@/lib/api";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";

// Mirrors api/v1alpha1/intent_usage.go. Aggregates only; no goal text or actors.
export type IntentUsage = {
  tenant: string;
  from: string;
  to: string;
  windowDays: number;
  inspected: number;
  truncated?: boolean;
  started: number;
  proposed: number;
  approved: number;
  applied: number;
  rejected: number;
  appliedFirstProposal: number;
  rolledBack: number;
  backtestAttached: number;
  guidedRuns: number;
  modelRuns: number;
  medianSecondsToProposal?: number;
  medianSecondsToApplied?: number;
  limitations: string[];
};

export function humanDuration(seconds: number): string {
  if (seconds < 90) return `${Math.max(0, Math.round(seconds))} s`;
  if (seconds < 5400) return `${Math.round(seconds / 60)} min`;
  if (seconds < 129600) return `${(seconds / 3600).toFixed(1)} h`;
  return `${(seconds / 86400).toFixed(1)} d`;
}

const share = (part: number, whole: number) =>
  whole ? Math.round((part / whole) * 100) : 0;

function Stat({
  label,
  value,
  detail,
}: {
  label: string;
  value: string;
  detail?: string;
}) {
  return (
    <div className="min-w-0">
      <Text as="span" size="sm" color="secondary" className="block">
        {label}
      </Text>
      <Heading level={3} className="tabular-nums">
        {value}
      </Heading>
      {detail ? (
        <Text as="span" size="sm" color="secondary" className="block">
          {detail}
        </Text>
      ) : null}
    </div>
  );
}

export function IntentUsagePanel({ scope }: { scope: RequestScope }) {
  const [days, setDays] = useState("90");
  const [usage, setUsage] = useState<IntentUsage | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    setError("");
    api<IntentUsage>(`/api/intent-conversations/usage?days=${days}`, scope, {
      signal: controller.signal,
    })
      .then((next) => {
        if (!controller.signal.aborted) setUsage(next);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(message(e));
      });
    return () => controller.abort();
  }, [scope, days]);
  const funnel = usage
    ? [
        { label: "Started", value: usage.started },
        { label: "Proposed", value: usage.proposed },
        { label: "Approved", value: usage.approved },
        { label: "Applied", value: usage.applied },
      ]
    : [];
  return (
    <Card className="gap-0 py-0" aria-label="Workflow progress">
      <CardHeader className="border-b py-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <Heading level={2}>Workflow progress</Heading>
            <Text as="p" size="sm" color="secondary">
              How goals in this tenant moved from draft to applied change. These
              counts describe the workflow, not whether an SLO is healthy.
            </Text>
          </div>
          <div className="w-40">
            <Label htmlFor="usage-days">Window</Label>
            <NativeSelect
              id="usage-days"
              value={days}
              onChange={(e) => setDays(e.target.value)}
            >
              <option value="7">Last 7 days</option>
              <option value="30">Last 30 days</option>
              <option value="90">Last 90 days</option>
              <option value="365">Last 365 days</option>
            </NativeSelect>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-6 py-5">
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : !usage ? (
          <Text color="secondary" role="status">
            Loading workflow progress…
          </Text>
        ) : !usage.started ? (
          <Text color="secondary" role="status">
            No goals were started in this window.
          </Text>
        ) : (
          <>
            <ol className="grid gap-4 sm:grid-cols-4" aria-label="Stages">
              {funnel.map((stage) => (
                <li key={stage.label} className="space-y-2">
                  <Stat
                    label={stage.label}
                    value={String(stage.value)}
                    detail={`${share(stage.value, usage.started)}% of started`}
                  />
                  <ProgressBar
                    label={`${stage.label}: ${share(stage.value, usage.started)}% of started`}
                    value={share(stage.value, usage.started)}
                    isLabelHidden
                  />
                </li>
              ))}
            </ol>
            <div className="grid gap-4 border-t pt-5 sm:grid-cols-3">
              <Stat
                label="Median time to proposal"
                value={
                  usage.medianSecondsToProposal === undefined
                    ? "No data yet"
                    : humanDuration(usage.medianSecondsToProposal)
                }
              />
              <Stat
                label="Median time to applied"
                value={
                  usage.medianSecondsToApplied === undefined
                    ? "No data yet"
                    : humanDuration(usage.medianSecondsToApplied)
                }
                detail="Includes time waiting for review"
              />
              <Stat
                label="Applied on first proposal"
                value={`${usage.appliedFirstProposal} of ${usage.applied}`}
                detail={
                  usage.applied
                    ? `${share(usage.appliedFirstProposal, usage.applied)}% needed no re-proposal`
                    : "Nothing applied yet"
                }
              />
              <Stat
                label="Proposals with a backtest"
                value={`${usage.backtestAttached} of ${usage.proposed}`}
              />
              <Stat
                label="Rejected / rolled back"
                value={`${usage.rejected} / ${usage.rolledBack}`}
              />
              <Stat
                label="Guided / model-assisted runs"
                value={`${usage.guidedRuns} / ${usage.modelRuns}`}
              />
            </div>
            {usage.truncated ? (
              <Text as="p" size="sm" color="secondary" role="status">
                Showing the {usage.inspected} most recent conversations. Choose
                a shorter window for complete counts.
              </Text>
            ) : null}
            <ul className="list-disc space-y-1 pl-5">
              {usage.limitations.map((text) => (
                <li key={text}>
                  <Text as="span" size="sm" color="secondary">
                    {text}
                  </Text>
                </li>
              ))}
            </ul>
          </>
        )}
      </CardContent>
    </Card>
  );
}
