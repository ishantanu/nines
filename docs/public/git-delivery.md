# Git delivery

Git delivery is an optional way to save a reviewed SLO or journey. Instead of applying it in Nines, the portal opens a pull request (GitHub) or merge request (GitLab) in your repository, and your own review and pipeline deploy it. It adds to the existing capabilities. **Save reviewed SLO** and the CLI keep working unchanged, and a workspace without Git delivery configured shows none of this.

## What it does

1. You define an SLO or journey in the portal and verify it as usual.
2. Choose **Propose via Git** instead of **Save reviewed SLO**.
3. Nines creates a branch, commits the exact reviewed definition at `<path>/<id>.slo.yaml` (`<id>.journey.yaml` for a journey), and opens a pull request against the configured base branch.
4. Your repository's review decides whether it merges. Your pipeline deploys it, for example the [Kubernetes operator](deployment/operator.md) reading the file, or `nines apply` in CI.
5. The SLO or journey page lists the proposals and their state, and whether the repository file matches the live definition.

Nothing is applied in Nines by proposing or by merging. The pull request description says so, and an SLO is shown as **Applied** only when the live definition's digest equals the proposed digest. A merged proposal whose digest differs from the live definition is shown as **Merged, not applied**, because your pipeline has not deployed it or a later change replaced it.

## Configure it

An administrator configures one repository per tenant in the server's `TENANTS_FILE`. Callers can never choose the repository, branch, path or credential.

```yaml
tenants:
  - id: commerce
    name: Commerce
    gitOps:
      provider: github
      repository: your-org/slo-definitions
      baseBranch: main
      path: slos/commerce
      tokenEnv: COMMERCE_GITHUB_TOKEN
```

- `provider` is `github` or `gitlab`.
- `repository` is `owner/name` for GitHub and `group/project`, optionally with subgroups, for GitLab. `baseBranch` defaults to `main` and `path` to `slos`. The path is a relative directory without dot segments.
- `tokenEnv` names an environment variable that holds the credential. Use a GitHub App installation token or a fine-grained token limited to that repository, with contents and pull-request write access. For GitLab, use a project access token with the `api` scope and the Developer role. The credential is read at startup, never returned by the API and never logged.
- `apiURL` optionally selects a GitHub Enterprise Server API origin, or a self-managed GitLab API origin that ends in `/api/v4`. It must be HTTPS without credentials.
- Startup fails if the configuration or credential is invalid.

## Use it

- In **Define objective**, after **Verify telemetry and review SLO** and the acknowledgement, **Propose via Git** is enabled beside **Save reviewed SLO**. In the Settings and YAML tabs it sits beside **Apply definition**.
- A GitLab merge request is reported as a pull request throughout, and the branch it opens is not deleted on merge.
- Owners and editors can propose. Viewers and agents cannot, so a model or agent can never open a pull request on its own.
- A proposal for an SLO that is unchanged from the saved definition is refused.
- The branch is `nines/<tenant>/<id>-<digest>` for an SLO and `nines/<tenant>/journeys/<id>-<digest>` for a journey, so the two never collide. Proposing the same definition again returns the existing pull request instead of opening a duplicate.
- If a step fails after the branch is created, Nines removes the branch so no orphan is left. If GitHub may have completed the request, retry the identical proposal to find it.
- The commit contains the definition as Nines planned it, and the pull request records the definition digest and the live revision it was reviewed against.

## Keep the repository and Nines in step

- **Revision and digest checks still apply.** The server re-plans the definition and refuses the proposal if the revision or digest differs from what you reviewed.
- **Apply in your pipeline with the revision it expects.** `nines apply --expected-revision N` refuses if the live SLO moved, so a stale merge cannot silently overwrite a newer change.
- **Drift is reported, not resolved.** The SLO or journey page compares the file on the base branch with the live definition by digest and shows **In sync**, **Differs from repository**, **Not in repository**, **In repository, not applied** or **Repository file unreadable**. A difference never says which side is right, and only the definition is compared: deployed rules and the observed SLI are separate evidence. Only the base branch is read, so unmerged proposals are not considered.
- **Two sources of truth.** If a repository owns an SLO through the operator, keep portal edits for that SLO off, because the next sync would revert them. Operator-owned and portal-owned resources already have distinct ownership.

## CLI

```sh
nines git info
nines plan -f reviewed.yaml --agent > plan.json
nines git propose -f reviewed.yaml --expected-revision 0 --digest PLAN_DIGEST
nines git proposals checkout-success
nines git drift checkout-success
nines git proposals complete-checkout --journey
```

`propose` requires `--expected-revision` and `--digest` from the plan you reviewed and never defaults them. It opens the request and does not apply anything.

## API

| Operation                                                                        | Purpose                                                                                                                                                                                                                                                     |
| -------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/gitops`                                                                | Whether Git delivery is configured, with the repository, base branch and path. No credential.                                                                                                                                                               |
| `POST /api/gitops/proposals`                                                     | Open a pull request for an SLO or journey. Send the YAML with `Content-Type: application/yaml`, `If-Match` set to the quoted expected revision and `X-Nines-Plan-Digest` set to the plan digest. Returns 201 for a new request and 200 for an existing one. |
| `GET /api/slos/{id}/gitops/proposals`, `GET /api/journeys/{id}/gitops/proposals` | Recent proposals with their state and whether each is applied.                                                                                                                                                                                              |
| `GET /api/slos/{id}/gitops/drift`, `GET /api/journeys/{id}/gitops/drift`         | Repository file compared with the live definition by digest.                                                                                                                                                                                                |

These operations are in the [OpenAPI specification](api.md) and the Go client (`client.ProposeYAML`, `GitProposalsFor`, `GitDriftFor`).

## Limits

- GitHub and GitLab, one repository per tenant, SLO and journey definitions only. Bindings and dependency maps are not proposed.
- Nines does not merge, auto-merge or deploy.
- Status and drift are read from the provider when you open the page and list up to 10 recent proposals. Nines stores no copy.
- The definition, name and plan warnings are written to the repository you configured. Confirm that repository may receive them.
