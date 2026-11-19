export interface GitInfo {
  configured: boolean;
  provider?: string;
  repository?: string;
  baseBranch?: string;
  path?: string;
}
export interface GitPullRequest {
  number: number;
  url: string;
  branch: string;
  state: "open" | "merged" | "closed";
  title: string;
  definitionDigest?: string;
  expectedRevision?: number;
  createdAt: string;
}
export interface GitReceipt {
  receipt: {
    created: boolean;
    path: string;
    pullRequest: GitPullRequest;
  };
  definitionDigest: string;
  expectedRevision: number;
  limitations: string[];
}
export interface GitProposalStatus extends GitPullRequest {
  /** True only when the merged definition equals the live one. */
  applied: boolean;
}

/** Only https links from the Git provider are rendered as links. */
export const safePullURL = (url: string) => /^https:\/\/[^\s]+$/.test(url);

/** What a proposal means for the SLO, in words, without implying deployment. */
export function proposalStatus(p: GitProposalStatus): {
  label: string;
  detail: string;
} {
  if (p.applied)
    return {
      label: "Applied",
      detail: "The live definition matches this proposal.",
    };
  if (p.state === "merged")
    return {
      label: "Merged, not applied",
      detail:
        "Merged, but the live definition differs. Your pipeline has not deployed it, or a later change replaced it.",
    };
  if (p.state === "open")
    return { label: "Open", detail: "Waiting for review in the repository." };
  return { label: "Closed", detail: "Closed without merging." };
}

export interface GitDrift {
  state: "in_sync" | "drifted" | "not_in_repo" | "not_applied" | "unreadable";
  path: string;
  repositoryDigest?: string;
  liveDigest?: string;
}
const drifts: Record<GitDrift["state"], [string, string]> = {
  in_sync: [
    "In sync",
    "The repository file and the live definition are identical.",
  ],
  drifted: [
    "Differs from repository",
    "The live definition and the repository file differ. Either may be the intended one.",
  ],
  not_in_repo: [
    "Not in repository",
    "The repository has no file for this definition.",
  ],
  not_applied: [
    "In repository, not applied",
    "The repository has a file that Nines has not applied.",
  ],
  unreadable: [
    "Repository file unreadable",
    "The repository file is not a valid definition for this name.",
  ],
};
export const driftLabel = (s: GitDrift["state"]) => drifts[s]?.[0] ?? s;
export const driftDetail = (s: GitDrift["state"]) => drifts[s]?.[1] ?? "";
