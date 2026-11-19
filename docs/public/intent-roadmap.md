# Intent workflow implementation plan

Intents should turn a reliability outcome into a grounded, reviewable SLO proposal and help maintain that measurement. They must remain usable through CLI and MCP without a dashboard or mandatory model provider.

## Dependency graph

```mermaid
flowchart LR
    A[1. Scoped context] --> B[2. Measurement candidates]
    B --> C[3. Evidence and trade-offs]
    C --> D[4. Reviewable proposal]
    D --> E[5. Activation verification]
    E --> F[6. Maintenance proposals]
    A --> F
```

Dependencies describe the integrated user workflow. Each chunk must also run against synthetic fixtures independently, without credentials for paid providers or a model. Existing compilation, approvals, audit and rollback are reused rather than reimplemented.

## Chunks and completion goals

| Chunk                      | Goal and deliverable                                                                                                                                                                                                                                                         | Independent acceptance                                                                                                                                                                                                                                   | Status                                                     |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| 1. Scoped context          | Read a bounded, versioned snapshot of an exact service/source/project, existing SLO measurements and an optional revision-pinned CUJ. Expose the same contract through HTTP, Go client, CLI and MCP. No upstream requests, model calls or writes.                            | `make test-intent-context` tests scope, pagination, missing measurements, source capabilities, revision conflicts and all adapters with local fixtures.                                                                                                  | Complete: fixture acceptance passed                        |
| 2. Measurement candidates  | Bridge existing metric-name/series discovery into intent preparation. Return bounded, provider-capability-aware candidates with population evidence, recipe provenance and unresolved questions. Never invent a metric or equate existence with correctness.                 | `make test-intent-candidates` covers availability, histogram latency, logical agent tasks, evaluation verdicts, ambiguous populations, missing instrumentation, unsupported discovery, extension provenance and CLI/API/MCP/worker parity. No live keys. | Complete: fixture acceptance passed                        |
| 3. Evidence and trade-offs | Assemble current traffic/coverage/freshness and explicitly requested historical backtests. Compare target, window and burn-policy consequences; state gaps and limits rather than fabricate evidence.                                                                        | `make test-intent-evidence` covers complete/partial/zero-traffic/stale fixtures, explicit history bounds, millisecond thresholds, provenance/revision conflicts and API/CLI/MCP parity.                                                                  | Complete: fixture acceptance passed                        |
| 4. Reviewable proposal     | Present outcome, measurement, exclusions, evidence, SLO/CUJ diff and publication destination as one review packet. Reuse digest-bound measurement confirmation, independent approval, audit and rollback. Clarify only unresolved fields.                                    | Guided end-to-end fixture from goal to proposal; exact approvals, stale-digest rejection, no agent self-approval, no target weakening and CLI/MCP parity. Optional model evaluation remains separate. `make test-intent-proposals` runs this acceptance. | Complete: fixture acceptance passed                        |
| 5. Activation verification | Follow the approved change through saved definition, rule/native publication, provider evaluation and observed measurements. Produce a resumable status receipt with exact revisions and actionable failures.                                                                | Independent Prometheus/Mimir playground acceptance and vendor API mocks. Saved YAML alone must never count as runtime activation.                                                                                                                        | Complete: local fixture and real-backend acceptance passed |
| 6. Maintenance proposals   | Watch saved measurement health and existing CUJ links for missing/stale telemetry or coverage gaps. Prepare deduplicated, evidence-backed repairs through the same approval flow. Never silently relax a target or claim instrumentation semantics changed without evidence. | Synthetic healthy/degraded/stale cases, bounded retries, duplicate suppression, team/tenant isolation and no automatic definition writes.                                                                                                                | Planned                                                    |

## Rules for every chunk

- Run fixture acceptance independently; integrate only after the prerequisite contracts pass.
- Use `nines.dev/v1alpha1` resource contracts, explicit source selection, bounded reads and typed errors.
- Preserve provider capabilities: an unsupported discovery API is a stated limitation, not an empty successful result.
- Treat descriptions, labels and extension recipes as untrusted data.
- Separate definition clarity, metric presence, measurement quality and measured attainment.
- Models can suggest or explain. Deterministic code validates and compiles; authorized engineers confirm semantics and approve exact changes.
- No paid-provider or hosted-model acceptance runs by default. Publish fixture and real-model results separately.
- Finish each chunk with docs, API/CLI/MCP agreement and its acceptance command. UI integration may consume those same contracts afterward.

## Run chunk 1

```sh
# From a source checkout. Uses local HTTP fixtures; no paid keys or Docker.
make test-intent-context

# Build the deterministic CLI and read your configured workspace.
make cli
./bin/nines --context production --agent intent context \
  --service api --source primary --project production --limit 5
```

`--context production` refers to an existing CLI connection profile. The server must run this implementation; older binaries do not provide `/api/intent-context`. Fixture acceptance does not establish live provider access or model quality. Chunk 2 now provides explicitly requested provider reads. Chunk 3 now exposes explicit evidence comparisons. Chunk 4 is complete under fixture acceptance. Chunk 5 now provides explicit activation inspection; chunk 6 remains planned. Context reads do not initiate those steps.

## Run chunk 2

```sh
make test-intent-candidates
nines --context production --agent intent candidates -f examples/intent-candidates.yaml
```

- Inputs: exact source/service/project, explicit equality population filters; optional literal metric prefix, up to three metrics, template category, outcome and unit-bearing latency threshold.
- Outputs: bounded candidate metric/population metadata, recipe revision/digest and extension provenance, source/catalog digests, evidence interval, truncation/capability state and unresolved semantic questions.
- Bounds: one name lookup and up to three series adapter calls; four seconds per call, fifteen seconds overall and 128 KiB output. Adapter calls can contain several bounded HTTP requests. No samples or definitions are stored.
- Availability counters, histogram latency including `30ms`, logical task outcomes, rubric-pinned evaluation verdicts, missing series, provider failures and hostile metadata have independent synthetic acceptance.
- The operation is available through HTTP, Go client, CLI and MCP. A consented model run may optionally use the pinned-scope tool before catalog selection; it does not mutate the goal or confirm a measurement. The four-tool turn limit and six-call run limits remain.
- Candidate presence is neither fresh traffic nor semantic confirmation. Vendor collection alone does not imply support for discovery. See [the usage guide](intents.md#discover-measurement-candidates).
- Acceptance is fixture-only. No claim of live provider connectivity or real-model quality is made for the updated skill.

## Run chunk 3

```sh
make test-intent-evidence
nines --context production --agent intent evidence -f examples/api-success.intent.yaml
nines --context production --agent intent evidence -f examples/intent-evidence.yaml
```

- First reviews accept a complete inline intent and capture source/catalog/measurement provenance. Exact optional digest pins bind earlier receipts; `expectedRevision` guards the saved identity, including zero for a new SLO.
- Baseline and up to two explicitly authored alternatives reuse runtime ratio/quality evaluation and historical sampling. No target, window or alert policy is chosen automatically.
- Reports separate current counts, coverage, cutoff freshness, configured quality/cohort gaps and explicitly requested historical cutoff summaries. Missing/zero traffic remains unknown; overlapping counts are never summed.
- Bounds: five recent cutoffs per comparison; history up to 72h and 240 cutoffs. At most one metadata verification, three recent-window and three historical adapter calls; ten seconds each within seventy-five seconds overall. Responses are at most 128 KiB. Adapter calls can contain multiple HTTP requests and rolling lookback extends before history by up to 31 days.
- CLI, HTTP, Go client and MCP use the same report. It does not save samples, confirm semantics, approve changes or deploy rules. Fixture coverage is separate from live-provider and real-model acceptance.
- See [evidence inputs and interpretation](intents.md#inspect-evidence-and-compare-explicit-choices).

## Chunk 4 progress

- First slice complete: existing workflow status exposes read-only saved preparation checks, including mismatched goal/compilation/confirmation/history records. It performs no provider or model calls and does not change approval/application gates.
- CLI, HTTP, Go client and MCP consume the same optional status field. Current provider evidence remains explicitly unverified.
- Second slice: explicit server-generated provider evidence can be attached to a ready, human-confirmed inline intent through HTTP, Go client, CLI and write-enabled MCP. Collection uses exact conversation and configuration fences. Frozen recommendations include the receipt in their approval digest.
- Third slice: new frozen recommendations contain a typed review packet with the exact goal/measurement and confirmation, saved checks, logical destination and SLO/CUJ change references. Existing HTTP, Go client, CLI and MCP review exports carry it without extra provider/model calls. `make test-intent-review-packet` verifies digest coverage and transport parity.
- Fourth slice: new candidate tool steps carry a server-stamped goal digest. The frozen packet includes the newest bounded receipt and explicit current-goal, prior-goal, missing-stamp, stale or invalid states. Discovery is not candidate selection, measurement confirmation or current-provider proof.
- Fifth slice: optional `spec.exclusions` records either explicit none or bounded observation-boundary notes. The existing exact human measurement confirmation covers these notes. They are preserved in generated SLO intent metadata and cannot change queries or discard events.
- Sixth slice: explicit `preserveGoal` replies retain typed facts while invalidating prior confirmation/proposals. Skill `0.6.0` checks declared question fields against saved facts and returns redundant questions to the model. Free-text interpretation and real-model quality remain separate acceptance.
- Chunk 4 is implemented with independent memory-store, PostgreSQL, transport and deterministic model fixtures. Paid-provider and real-model acceptance are not implied.

## Chunk 4 acceptance contract

- Assemble outcome, exact measurement, exclusions, candidate/evidence receipts, SLO/CUJ diff and publication destination into a review packet. Reuse existing review artifacts and revision-bound governance.
- Clarify unresolved fields without asking again for supplied facts. Keep definition clarity separate from measurement quality and measured attainment.
- Carry source/catalog/intent/measurement provenance through model-optional guided preparation. Reject stale or mismatched receipts; first-review captures cannot validate earlier evidence.
- Independently test goal-to-proposal fixtures, exact approvals, agent self-approval denial, no implicit target weakening, stale-digest rejection and CLI/MCP parity. Reuse existing application and separately reviewed rollback.

## Run chunk 4

```sh
make test-intent-proposals
# Optional: include the real PostgreSQL store using a disposable local database.
TEST_DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/DB make test-intent-proposals
```

Use a private environment/configuration mechanism for real credentials. The test URL above is a placeholder. See [review packet and clarification semantics](intents.md#review-the-frozen-proposal-packet). The next independently runnable chunk is activation verification; a frozen packet's destination remains unverified.

## Run chunk 5: activation inspection

```sh
make test-intent-activation
nines --context production --agent intent conversations activation CONVERSATION
```

- This first slice reads the configured publication/evaluator/query backend with a ten-second overall bound and inspects saved collection/quality evidence. It never publishes rules, collects samples, saves an outcome or changes a definition.
- The typed receipt carries the conversation version, apply or rollback digest, exact expected/current resource revisions, source digest, observation times, stages, failure code and next action. Repeat the same command for a fresh receipt; pending results include `retryAt` guidance.
- File storage alone, a missing evaluator timestamp, a wrong query dataset, stale collection, zero traffic and limited quality cannot produce verified activation. Native SLOs need an independently observed evaluation signal; provider storage alone remains pending. Datadog, New Relic and Dash0 native evaluation is currently unverified by this integration check.
- Saved workflow `status` and `verify` remain provider-free. The existing independently authorized observer records sustained post-change outcomes separately. Activation can be verified while SLI is below target; the receipt does not assert health, causality or notification delivery.
- Acceptance in this slice uses memory/PostgreSQL workflow fixtures and API/client/CLI/MCP contracts. The real Prometheus and Mimir suites now include mandatory intent-activation cases. These seed exact applied-operation references; publication, evaluation, query output and interval collection use the real disposable backends. Independent headless fixtures exercise approval/application/rollback. Native-provider publication and evaluation limitations are checked against local API mocks; unsupported native evaluation remains unverified. No paid keys are required.

### Real backend acceptance

```sh
make test-prometheus
make test-mimir
```

These commands own and remove their loopback-only containers, private files and temporary storage. They do not touch the playground or a configured production backend. CI requires `TestRealPrometheusIntentActivation` and `TestRealMimirIntentActivation` to pass without skipping.

The checks distinguish definition-only, file-only, evaluated-without-collection and activated-with-collected-SLI states. Synthetic interval queries deliberately produce 99% attainment against a 99.9% target: activation succeeds without a health claim. The Mimir check also reads a different upstream tenant and requires the receipt to remain unverified. No paid ingestion, application workload or model call is involved.
