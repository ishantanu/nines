# GitHub reviews for SLO changes

Publish a reviewed Nines proposal as an immutable comment on a chosen GitHub pull request. The report includes the recommendation, exact proposed definitions and previous values, safety and budget evidence, approval and rollback state, verified audit and limitations.

This integration associates a Nines snapshot with a repository and PR commit. It does not compare the repository's files with the proposed definitions. A GitHub comment, review or merge does not approve or apply an SLO in Nines. Use the independently authorized [headless workflow](headless-workflows.md) for those operations.

## Prepare the review

Use a Nines credential authorized to read the intended conversation and an explicit server and tenant context. No GitHub credential is needed to generate the plan. Obtain the full PR head commit from GitHub and supply it explicitly:

```sh
umask 077
nines intent conversations github-plan CONVERSATION \
  --context production \
  --repository your-team/your-service \
  --pull-request 42 \
  --head FULL_PR_COMMIT_SHA \
  --agent > github-review-plan.json
```

The command reads the audit-verified Nines artifact and returns a plan containing the report Markdown, conversation version, change-set digest and report digest. It does not contact GitHub. Review the full report and confirm that the selected repository may receive its sensitive definitions, names, review notes and audit evidence. Plan files should be access-controlled.

- A prepared change set with recommendation evidence and a verified audit is required.
- The report is limited to 60,000 bytes; oversized evidence is refused without truncation. Share the full report through an access-controlled artifact workflow instead.
- The export timestamp is excluded from the GitHub report so retries for an unchanged version produce the same report digest.
- User and model text stays inside JSON code blocks. Publication does not execute that content.

## Publish the exact reviewed plan

Set `NINES_GITHUB_TOKEN` privately. Use a GitHub App installation token, GitHub Actions token or suitably scoped fine-grained token with pull-request read/write permissions on the intended repository. Nines' credential and GitHub's credential are separate; neither is accepted as a command-line token flag.

```sh
nines intent conversations github-publish CONVERSATION \
  --context production \
  --repository your-team/your-service \
  --pull-request 42 \
  --head FULL_PR_COMMIT_SHA \
  --expected-version REVIEWED_CONVERSATION_VERSION \
  --expected-digest REVIEWED_CHANGE_SET_DIGEST \
  --expected-report-digest REVIEWED_REPORT_DIGEST \
  --confirm-sharing \
  --agent
```

The publisher fetches a fresh Nines report, compares the reviewed values, checks the PR's repository, open state and head commit, and rechecks Nines status before the external write. It creates a comment and returns its URL, ID, head commit and report digest. Tokens, raw GitHub errors and response bodies do not enter error output. Literal or JSON-escaped copies of either configured credential in the report prevent publication.

## Retries and stale evidence

- Comments are immutable snapshots. A new conversation version or PR head needs a new plan and explicit review.
- Retrying an identical plan finds its existing matching comment and returns `already_published`. A modified comment with the same snapshot marker causes a conflict; Nines does not overwrite it.
- Comment lookup is bounded to 1,000 comments. Publication stops if it cannot complete that search.
- A timeout or server failure after posting can be uncertain. While the Nines version and PR head still match, retry the identical reviewed plan to inspect for its existing comment; do not generate fresh versions or change destinations to evade a conflict.
- GitHub has no atomic head precondition for comment creation. The head or Nines state can change between their final checks and the post. Every comment labels the reviewed commit; readers must verify the current head and Nines state before acting.
- Concurrent publishers can race and create duplicate comments because GitHub's comment API has no idempotency key. Serialize publication per PR, for example with GitHub Actions concurrency.
- Receipts are returned to the caller; publication is not currently a separate event in Nines' audit journal. Retain receipts under your workflow's access and retention policy.

## GitHub Actions setup

The [example workflow](../../examples/github-slo-review.workflow.yaml) is an opt-in manual workflow for a dedicated self-hosted runner with the candidate Nines CLI and `jq` installed. It does not check out or execute PR code. Configure the `nines-review` environment, its access/protection policy, `NINES_URL` and `NINES_TENANT` variables, and a `NINES_REVIEW_API_KEY` secret authorized to read the conversation. The runner must reach the Nines server.

Run plan mode, review the job summary and capture its version and digests, then run publish mode with those exact values. Both the summary and the PR comment contain sensitive evidence. Installing the workflow is an explicit decision to share it with users who can access the repository and workflow runs.

The CLI publisher targets GitHub.com. GitHub Enterprise Server, native chat approval, automatic webhook-to-PR dispatch and required status checks are not implemented by this change.

## Local verification

```sh
go test -race ./internal/githubreview ./cmd/nines -run 'Test.*GitHub|TestPlan|TestPublish|TestPublication'
```

These tests use isolated synthetic HTTP servers and a fake GitHub transport. They verify consent and digest checks, stale-head and stale-version denial, safe Markdown, bounded report size, retry readback, redirects and uncertain write handling. They do not publish real comments. Real GitHub App installation and repository-policy acceptance must be tested separately.
