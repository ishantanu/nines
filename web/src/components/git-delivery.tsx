import { useEffect, useState } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { Link } from "@astryxdesign/core/Link";
import { Text } from "@astryxdesign/core/Text";
import { api, canEdit, message, type RequestScope } from "@/lib/api";
import { stringifyDefinition as stringify } from "@/lib/definition-yaml";
import {
  proposalStatus,
  safePullURL,
  driftDetail,
  driftLabel,
  type GitDrift,
  type GitInfo,
  type GitProposalStatus,
  type GitReceipt,
} from "@/lib/gitops";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

// Optional Git delivery. Unconfigured workspaces never render any of this.
export function useGitDelivery(scope: RequestScope): GitInfo | null {
  const [info, setInfo] = useState<GitInfo | null>(null);
  useEffect(() => {
    const c = new AbortController();
    setInfo(null);
    api<GitInfo>("/api/gitops", scope, { signal: c.signal })
      .then((v) => !c.signal.aborted && setInfo(v))
      .catch(() => !c.signal.aborted && setInfo(null));
    return () => c.abort();
  }, [scope]);
  return info;
}

export function GitProposeButton({
  scope,
  manifest,
  plan,
  disabled,
}: {
  scope: RequestScope;
  manifest: unknown;
  plan: { expectedRevision: number; digest: string; action?: string } | null;
  disabled?: boolean;
}) {
  const info = useGitDelivery(scope);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<GitReceipt | null>(null);
  useEffect(() => {
    setResult(null);
    setError("");
  }, [plan?.digest]);
  if (!info?.configured || !canEdit(scope)) return null;
  async function propose() {
    if (!plan) return;
    setBusy(true);
    setError("");
    setResult(null);
    try {
      setResult(
        await api<GitReceipt>("/api/gitops/proposals", scope, {
          method: "POST",
          headers: {
            "Content-Type": "application/yaml",
            "If-Match": `"${plan.expectedRevision}"`,
            "X-Nines-Plan-Digest": plan.digest,
          },
          body: stringify(manifest as never),
        }),
      );
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const pr = result?.receipt.pullRequest;
  return (
    <>
      <Button
        type="button"
        variant="outline"
        disabled={disabled || busy || !plan || plan.action === "unchanged"}
        onClick={propose}
        title={`Open a pull request in ${info.repository} instead of applying`}
      >
        {busy ? "Opening pull request…" : "Propose via Git"}
      </Button>
      {error ? (
        <p role="alert" className="basis-full text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {pr ? (
        <div role="status" className="basis-full space-y-1 text-sm">
          <p className="font-medium">
            {result!.receipt.created
              ? "Pull request opened"
              : "Pull request already exists"}
            :{" "}
            {safePullURL(pr.url) ? (
              <Link href={pr.url} isExternalLink>
                #{pr.number}
              </Link>
            ) : (
              `#${pr.number}`
            )}{" "}
            in {info.repository}
          </p>
          <p className="text-xs text-muted-foreground">
            The SLO is not applied in Nines. Merge the pull request and let your
            pipeline deploy it; the SLO page shows it as applied once the live
            definition matches this proposal.
          </p>
        </div>
      ) : null}
    </>
  );
}

// Proposals Nines opened for one SLO, read from the Git provider on request.
export function GitProposals({
  scope,
  id,
  kind = "slo",
}: {
  scope: RequestScope;
  id: string;
  kind?: "slo" | "journey";
}) {
  const info = useGitDelivery(scope);
  const [rows, setRows] = useState<GitProposalStatus[] | null>(null);
  const [drift, setDrift] = useState<GitDrift | null>(null);
  const [error, setError] = useState("");
  const base = `/api/${kind === "journey" ? "journeys" : "slos"}/${encodeURIComponent(id)}/gitops`;
  useEffect(() => {
    if (!info?.configured) return;
    const c = new AbortController();
    setRows(null);
    setError("");
    setDrift(null);
    api<{ proposals: GitProposalStatus[] }>(`${base}/proposals`, scope, {
      signal: c.signal,
    })
      .then((v) => !c.signal.aborted && setRows(v.proposals))
      .catch((e) => !c.signal.aborted && setError(message(e)));
    // Drift is secondary evidence: a failure to read it never hides proposals.
    api<GitDrift>(`${base}/drift`, scope, { signal: c.signal })
      .then((v) => !c.signal.aborted && setDrift(v))
      .catch(() => undefined);
    return () => c.abort();
  }, [info?.configured, scope, base]);
  if (!info?.configured) return null;
  return (
    <section
      aria-label="Git proposals"
      className="space-y-2 rounded-md border p-4"
    >
      <Heading level={3}>Git proposals</Heading>
      <Text as="p" size="sm" color="secondary">
        Pull requests opened for this {kind === "journey" ? "journey" : "SLO"}{" "}
        in {info.repository}. A merge is not a deployment: an SLO is applied
        only when its live definition matches.
      </Text>
      {drift ? (
        <p className="text-sm" aria-label="Repository comparison">
          <Badge variant={drift.state === "in_sync" ? "default" : "outline"}>
            {driftLabel(drift.state)}
          </Badge>{" "}
          <span className="text-muted-foreground">
            {driftDetail(drift.state)} ({drift.path})
          </span>
        </p>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : rows === null ? (
        <Text as="p" size="sm" color="secondary">
          Reading pull requests…
        </Text>
      ) : rows.length === 0 ? (
        <Text as="p" size="sm" color="secondary">
          No proposals yet.
        </Text>
      ) : (
        <ul className="space-y-2">
          {rows.map((p) => {
            const s = proposalStatus(p);
            return (
              <li
                key={p.number}
                className="flex flex-wrap items-center gap-2 text-sm"
              >
                <Badge variant={p.applied ? "default" : "outline"}>
                  {s.label}
                </Badge>
                {safePullURL(p.url) ? (
                  <Link href={p.url} isExternalLink>
                    #{p.number} {p.title}
                  </Link>
                ) : (
                  <span>
                    #{p.number} {p.title}
                  </span>
                )}
                <span className="text-muted-foreground">{s.detail}</span>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
