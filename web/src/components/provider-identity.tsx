import { Database } from "lucide-react";
import { providerLabel } from "@/lib/telemetry-providers";
import type { SLO, Workspace } from "@/lib/api";
import prometheus from "@/assets/providers/prometheus.svg?no-inline";
import mimir from "@/assets/providers/mimir.svg?no-inline";
import datadog from "@/assets/providers/datadog.svg?no-inline";
import newrelic from "@/assets/providers/newrelic.svg?no-inline";
import victoriametrics from "@/assets/providers/victoriametrics.svg?no-inline";
import dash0 from "@/assets/providers/dash0.svg?no-inline";
import googlecloud from "@/assets/providers/googlecloud.svg?no-inline";
const marks: Record<string, string> = {
  prometheus,
  mimir,
  datadog,
  newrelic,
  victoriametrics,
  dash0,
  googlecloud,
};

/** Text carries identity; the bundled SVG image is decorative. */
export function ProviderIdentity({
  kind,
  connection,
}: {
  kind: string;
  connection?: string;
}) {
  return (
    <span className="inline-flex max-w-full items-center gap-1.5 align-middle text-xs text-muted-foreground">
      {marks[kind] ? (
        <img
          src={marks[kind]}
          alt=""
          aria-hidden="true"
          width={16}
          height={16}
          className={
            kind === "mimir"
              ? "size-4 shrink-0 object-contain"
              : "size-4 shrink-0 object-contain dark:invert"
          }
        />
      ) : (
        <Database aria-hidden="true" className="size-4 shrink-0" />
      )}
      <span className="min-w-0 break-words">
        {providerLabel(kind)}
        {connection ? ` · ${connection}` : ""}
      </span>
    </span>
  );
}

export function SLOProvider({
  slo,
  connections = [],
}: {
  slo: SLO;
  connections?: Workspace["connections"];
}) {
  const name = slo.source || "primary";
  const source = connections.find((c) => c.id === name);
  // A query language does not identify a vendor or prove native publication.
  return source ? (
    <ProviderIdentity kind={source.kind} connection={name} />
  ) : (
    <span className="text-xs text-muted-foreground">Connection: {name}</span>
  );
}
