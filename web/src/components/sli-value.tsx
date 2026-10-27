import { ReplayLabel } from "@/components/replay-label";
import { percent, type Snapshot } from "@/lib/api";

export type SLIMeasurement = Pick<Snapshot, "reliability" | "dataStatus"> &
  Partial<Pick<Snapshot, "coverage" | "alert">>;

// Measured attainment is independent of whether a full-window verdict is possible.
export function SLIValue({
  status,
  target,
  compact = false,
  showAlerts = true,
}: {
  status: SLIMeasurement;
  target?: number;
  compact?: boolean;
  showAlerts?: boolean;
}) {
  const measured =
    status.reliability !== null && Number.isFinite(status.reliability);
  const coverage = status.coverage;
  const windowLabel =
    status.dataStatus === "partial" || (coverage !== undefined && coverage < 1)
      ? "Partial window"
      : status.dataStatus === "complete"
        ? "Full window"
        : "";
  return (
    <span className="inline-flex min-w-0 flex-col gap-0.5 text-left">
      <span
        className={`text-sm font-medium tabular-nums${measured && target !== undefined && status.reliability! < target ? " status-degraded" : ""}`}
      >
        {measured ? (
          `${percent(status.reliability, 3)} SLI`
        ) : status.dataStatus === "no_traffic" ? (
          "No traffic"
        ) : (
          <ReplayLabel>No SLI data</ReplayLabel>
        )}
      </span>
      {!compact ? (
        <span className="text-xs text-muted-foreground">
          {target !== undefined ? `${percent(target, 3)} target` : ""}
          {measured && windowLabel
            ? `${target !== undefined ? " · " : ""}${windowLabel}`
            : ""}
        </span>
      ) : null}
      {compact && measured && windowLabel === "Partial window" ? (
        <span className="text-xs text-muted-foreground">
          <ReplayLabel>Partial window</ReplayLabel>
        </span>
      ) : null}
      {!compact && coverage !== undefined && Number.isFinite(coverage) ? (
        <span className="text-xs text-muted-foreground tabular-nums">
          {coverage > 0 && coverage < 0.001 ? "<0.1%" : percent(coverage, 1)}{" "}
          coverage
        </span>
      ) : null}
      {showAlerts &&
      status.alert &&
      (status.alert.suppressedReason ||
        ["firing", "pending"].includes(status.alert.phase)) ? (
        <SLOAlertState
          status={{ alert: status.alert, reliability: status.reliability }}
        />
      ) : null}
    </span>
  );
}
export function SLOAlertState({
  status,
}: {
  status: Pick<Snapshot, "alert" | "reliability">;
}) {
  if (status.alert.phase === "archived")
    return (
      <span className="text-sm text-muted-foreground">
        Archived · alerts stopped
      </span>
    );
  const lifecycleLabel = (
    {
      lifecycle_experimental: "Experimental",
      lifecycle_aspirational: "Aspirational",
      lifecycle_deprecated: "Deprecated",
    } as Record<string, string>
  )[status.alert.suppressedReason ?? ""];
  if (status.alert.suppressedReason)
    return (
      <span className="text-xs text-muted-foreground">
        {lifecycleLabel ? (
          <>
            <ReplayLabel>{lifecycleLabel}</ReplayLabel> ·{" "}
          </>
        ) : null}
        <ReplayLabel>Alerts disabled</ReplayLabel>
      </span>
    );
  if (status.alert.phase === "firing")
    return (
      <span className="text-xs font-medium status-degraded">
        <ReplayLabel>Alert firing</ReplayLabel>
      </span>
    );
  if (status.alert.phase === "pending")
    return (
      <span className="text-xs font-medium status-at-risk">
        <ReplayLabel>Alert pending</ReplayLabel>
      </span>
    );
  return (
    <span className="text-xs text-muted-foreground">
      <ReplayLabel>
        {status.reliability === null ? "Not evaluated" : "No active alerts"}
      </ReplayLabel>
    </span>
  );
}
