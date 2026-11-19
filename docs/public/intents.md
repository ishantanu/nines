# Create an SLO from a goal

A goal, written as an SLO intent (`kind: ServiceLevelObjectiveIntent`), expresses the reliability outcome you want to measure. Nines turns it into an SLO only after its measurement is confirmed and a separate person approves the exact change.

- **Use Define objective** to write an SLO directly: choose a type, fill in the measurement, verify telemetry and save.
- **Use Create from a goal** when the change needs telemetry evidence, a backtest and a separate approval before it is applied, or when an agent prepares it. In the portal it is **Create from a goal** on **Service level objectives**, or in the sidebar.
- A goal can be any counter-ratio or threshold type: availability, latency, correctness, durability, freshness or coverage. Sessions, good-minute and throughput are defined with Define objective. See [SLO types](slo-types.md).
- The goal, evidence, approval and apply steps are deterministic and do not need a model. Only drafting the goal from a description, and agent-driven preparation, use a model; those parts are experimental.

- Nines can help turn natural language into a draft, discover relevant telemetry and prepare a change set.
- The deterministic compiler and explicit human review determine what can be applied.
- Model-assisted drafting and tool-driven preparation remain experimental. The guided path and offline CLI do not require a model.
- Intent YAML can set `spec.lifecycle` and policy `minEvents`.
  - The compiler preserves them in the SLO and its intent provenance.
  - Review shows lifecycle changes and their paging impact.
  - Agent context includes lifecycle and suppression separately from measured reliability; a disabled alert is not evidence that an objective is healthy. See [lifecycle and traffic guards](measurements.md#lifecycle-and-alert-eligibility).

## Define a goal and measurement

- An intent includes the service, source, target, window and SLO-owned measurement. These are reviewable inputs, not values a model may silently change during deployment.

<!-- example: examples/api-success.intent.yaml -->

- [Download the complete intent](../../examples/api-success.intent.yaml).

## Follow the review flow

1. **Draft the goal.** Use the portal or author intent YAML. State the user outcome and the observation point, including excluded traffic and retries.
2. **Verify telemetry.** Model mode can first propose a measurement from the tenant’s [template catalog](slo-templates.md#templates-in-intent-conversations). The recipe and digest are recorded for review; its examples are not telemetry evidence. Discover the metric and matching populations through the tenant's configured source. Missing or ambiguous evidence requires clarification.
3. **Confirm the measurement.** An owner/editor confirms the exact goal and measurement digests. A model or agent role cannot perform this confirmation.
4. **Prepare the changes.** Inspect the proposed SLO, optional critical user journey link, parameter diff, rationale, assumptions and available backtest evidence.
5. **Approve the exact change set.** Record a reason and review its limits. Edits to the goal, measurement, source or affected revisions invalidate the previous approval.
6. **Apply and verify.** The SLO and proposed journey change commit atomically. Rule delivery then follows the telemetry connection's automatic or manual policy. Check measurement coverage, backend evaluation and output separately.

- A successful model response is not permission to write production configuration. The workflow model has no approval or direct resource-write tool.

## Choose a model provider

- Model configuration is optional and provider-specific.
  - Nines supports native Anthropic and other documented provider routes, compatible endpoints and local model configurations.
  - You do not need an OpenAI account to use the deterministic platform or a different supported adapter.
- Configure provider credentials in the server environment or secret manager.
  - Do not put keys in intent YAML, prompts, project files or exported evidence.
  - See [provider setup and measured acceptance](intent-evaluations.md) for exact configuration and current limits; drafting support does not imply full tool-calling workflow support.

## Optional semantic advice

- [System One advice with Jev](system-one.md) suggests a template category and assesses whether the description defines eligible events, success and the observation point.
  - It works in guided or model-directed preparation with separate consent.
  - Results are advisory probabilities, never telemetry evidence or permission to apply a change.

## See how the workflow is used

- The portal's **Create from a goal** page has a **Workflow progress** tab, and `nines intent conversations usage` and `GET /api/intent-conversations/usage` return the same summary.
  - It counts conversations that reached each stage in a window: started, proposed, approved and applied.
  - It also reports how many applied changes needed no re-proposal, rejections, applied rollbacks, proposals that carried a backtest, guided versus model-assisted runs, and median times to proposal and to applied.
- The summary is computed from this tenant's stored conversations. It contains aggregates only (no actor, goal text or measurement), uses reader permission, and sends nothing outside the server.
- It describes workflow progress. It does not show whether an SLO is healthy or correct, and the medians include time spent waiting for review.

## Understand rollback

- Existing definitions can be restored as new revisions through a separately reviewed rollback.
  - Newly created resources have no previous definition and block this restore path.
  - Applying a restore cannot undo consumed error budget or recover missing historical telemetry.
- See [recommendation governance](intent-governance.md) for approval withdrawal, audit export and revision-safe rollback, and [MCP and skills](mcp.md) for agent access.

## Keep duration units explicit

- Latency form fields accept units such as `30ms`, `300us` or `0.03s`; unitless form input is rejected.
- Natural-language drafting preserves the exact unit in the quoted evidence. A draft cannot substitute `30s` for an entered `30ms`.
- Versioned numeric `thresholdSeconds` fields remain seconds: `30ms` is serialized as `0.03`, not `30`. Burn windows remain whole-second windows and reject subsecond values instead of rounding them.

## Continue when a conversation is full

- Each conversation permits eight preparation runs and forty messages, with two message slots reserved when starting preparation. Preparation and stored records also have byte limits.
- The portal identifies a full conversation and offers **Start new conversation with this goal**. Only the typed goal is copied; evidence, confirmations, approvals and change sets are not copied.
- The original conversation and audit history remain available. Verify telemetry and review the new proposal again before applying it.
- API clients receive HTTP 429 with an `intent_*_limit` code and the applicable limit. A conversation-capacity error needs a new conversation; retrying the same request does not free capacity.
- The tenant-wide saved-conversation limit is separate. Contact an administrator when that limit is reached; history is not silently deleted.

## Implementation milestones

See the [intent workflow plan](intent-roadmap.md) for chunk goals, dependencies, fixture acceptance and completion status. Planned capabilities are distinct from the current preparation workflow.

## Gather service context without a model

The first implementation chunk adds a read-only context snapshot. It reads saved Nines evidence, not metric names or new upstream samples:

```sh
nines --context production --agent intent context \
  --service checkout --source primary --project commerce --limit 5

# Include a critical user journey at its exact current revision:
nines --context production --agent intent context \
  --service checkout --source primary --project commerce \
  --journey complete-checkout --journey-revision 3
```

- HTTP: `GET /api/intent-context?service=checkout&source=primary&project=commerce` with the normal tenant credential. See the [versioned API specification](api.md).
- Go client: `Client.IntentContext(ctx, v1alpha1.IntentContextInput{...})`.
- MCP: read-only `intent_context` with the same typed scope fields. Credentials and upstream endpoints cannot be supplied through tool arguments.
- `objectives` contains current saved SLO context for the exact service/source/project. An optional CUJ contributes revision and linkage metadata; other services and sources in that journey are not silently imported.
- Follow `nextCursor` with the same tenant and scope, even when a page has no objectives. Each request scans at most 100 catalog rows and returns at most ten matching contexts, within 240 KiB. Inventory pagination is not an atomic snapshot or an approval artifact.
- Adapter capabilities do not establish connectivity, query permissions or fresh telemetry. Missing saved SLOs do not establish missing instrumentation. Candidate discovery below adds explicit upstream inspection; historical evidence preparation remains a separate step.

Independent fixture acceptance: `make test-intent-context`. No Docker, provider credentials or model server is required.

## Discover measurement candidates

Use explicit, bounded provider reads when the metric or its semantics are unresolved. A model is optional:

```sh
nines --context production --agent intent candidates -f examples/intent-candidates.yaml
```

Request YAML:

```yaml
service: api
source: primary
project: production
population:
  service: api
prefix: http_
category: request-serving
outcome: availability
limit: 3
```

- `service`, `source` and `project` are exact scope fields. They do not infer telemetry labels. `population` requires 1–16 explicit equality filters; narrower filters reduce unrelated series.
- Optionally provide up to three exact `metrics` instead of listing names. `prefix` is a literal name prefix, never a query. `category` filters the tenant's installed template catalog.
- For latency, set `outcome: latency` and `threshold: 30ms`. Unitless thresholds are rejected; `30ms` remains `0.03` seconds. Classic histogram discovery checks `_bucket` and `_count` and the exact numeric `le` boundary. It never substitutes p95, p99 or the next bucket. Units and success semantics still need review.
- Each request performs at most one metric-name lookup and three series inspections, with a four-second deadline per adapter call and a fifteen-second total deadline. One adapter call may use several bounded HTTP requests. Results are limited to 128 KiB, fifty listed names and three inspected candidates. Truncation is explicit.
- Inspect `state`: `unsupported`, `provider_error`, `partial`, `no_matching_metrics` or `needs_input`. Candidate states additionally distinguish no matching series, unsupported measurement types and incompatible outcomes. CLI success means a report was returned; inspect these states before proceeding.
- Candidates include the lookup interval, matching series count, label names, source/catalog digests, recipe revision/digest and installed extension provenance. Unrequested label values, provider help text and recipe descriptions are omitted. Names and references remain untrusted data.
- Presence in the series index does not establish fresh events, coverage, quality or business success. Review retry handling, terminal task outcomes, observation point, exclusions and success labels. Evaluation recipes require pinned rubrics/evaluators and separate completion coverage.
- Unsupported discovery is a capability limitation, not absent instrumentation. Prometheus-compatible discovery adapters expose the optional interfaces; vendor collection or native SLO publication alone does not imply discovery support. Supply a reviewed inline measurement when discovery is unavailable.
- This operation stores no samples, creates no SLO, selects no template and confirms nothing. There is no automatic application or alert-policy choice.

The same contract is available through `POST /api/intents/candidates`, `Client.DiscoverIntentCandidates` and read-only MCP `discover_intent_candidates`. Normal tenant authorization applies; tool arguments cannot select credentials, endpoints or another tenant. Candidate JSON rejects duplicate keys, case aliases, unknown fields and multiple documents before provider reads.

A consented model-directed preparation run may optionally call `discover_measurement_candidates` once, before catalog selection. Source, service, project, population and requested outcome come from the pinned session; the model can only narrow a literal prefix or category. The saved receipt does not satisfy template discovery or human measurement confirmation. Existing four-tool-per-turn and six-model/six-tool run limits remain. Crash recovery may repeat a read before its receipt is saved; this is not an exactly-once provider guarantee. Guided preparation does not initiate these new reads automatically.

Independent fixture acceptance: `make test-intent-candidates`. Real-provider discovery and real-model selection quality are not established by those fixtures.

## Inspect evidence and compare explicit choices

Start with a complete versioned intent containing `spec.measurement`:

```sh
nines --context production --agent intent evidence -f examples/api-success.intent.yaml

# An existing identity requires its current saved revision:
nines --context production --agent intent evidence -f my-intent.yaml --expected-revision 3

# Baseline plus explicit comparison choices; still no writes:
nines --context production --agent intent evidence -f examples/intent-evidence.yaml
```

- Adjust the example's source, metric, labels and success criteria to your instrumentation. An inline availability counter or classic-histogram latency measurement is required; absent instrumentation remains a gap. The operation does not discover or invent good-event semantics.
- `expectedRevision: 0` requires a new SLO identity; a saved identity requires its exact current revision. The server checks for concurrent definition changes before and after reads. An optional CUJ reference must retain its exact current revision.
- A first review captures current intent, measurement, source and catalog digests. To bind an earlier candidate/plan, provide exact `intentDigest`, `measurementDigest`, `sourceDigest` and `catalogDigest` fields in the request YAML. Candidate discovery uses the same tenant/source route digest; the measurement digest comes from compilation. Different routes, catalogs or measurements reject those pins. Missing pins do not validate earlier receipts.
- Digests establish consistency, not approval. Every report has `humanConfirmationRequired: true`; this operation creates no confirmation, conversation, approval, SLO, stored sample or recording rule. Review and application remain separate.
- Results compare the baseline with at most two explicit alternatives. Alternatives can change a target, a unit-bearing window such as `24h`, or authored burn policies. Omitted policies inherit the baseline; `policies: []` compares disabling them. Targets and policies are never selected automatically.
- `current` reports one independent rolling window's good/eligible counts, attainment, coverage, burn, quality checks and returned-cutoff freshness. Up to five recent completed cutoffs are inspected to identify stale or missing results. They are never summed. Counts are provider estimates; freshness does not independently prove instrumentation freshness.
- `state: partial` covers missing, stale, zero-traffic, insufficient-quality or failed evidence. Zero traffic and missing data retain null attainment. Configured cohort labels are reported as uninspected rather than silently counted as verified. Unsupported discovery/window-preview adapters return `unsupported`, not invented SLI data.
- `allowedBadEventsPerMillion` expresses `1 - target` for a fixed eligible population. A policy's `longWindowBudgetFraction` is `burnRate × longWindow / rollingWindow` under constant eligible event rate. This is a comparison assumption, not predicted downtime, detection latency or an outage forecast. Coverage, minimum-event and pending-duration gates remain explicit. `lifecycleAllowsAlerts` describes the proposed lifecycle, not deployed rules or delivered alerts.

Historical reads require an explicit request field, for example:

```yaml
# Add to an evidence request; choose completed cutoffs retained by your provider.
history:
  from: 2026-11-03T12:00:00Z
  to: 2026-11-03T13:00:00Z
  resolutionSeconds: 300
```

- The historical cutoff interval must be positive, align to `stepSeconds`, span at most 72 hours and contain at most 240 sampled cutoffs at 1–3600 second resolution. The provider lookback extends before those cutoffs by the requested rolling window, up to 31 days.
- Each comparison uses the existing runtime evaluator and historical sampler. Summaries count complete, unknown, no-traffic, target-violation and burn-threshold cutoffs. Overlapping event counts are never added. Sampled crossings do not simulate alert pending duration, intervening cutoffs or notification delivery. Historical summaries do not establish current operational readiness.
- No `history` means no historical query. Current reads still inspect recent provider windows. The whole operation has a 75-second deadline, with ten seconds per adapter call: one metadata verification, up to three recent-window reads and up to three explicitly requested historical reads. Adapter calls can use multiple bounded HTTP requests. Input/output limits are 32/128 KiB. Reports are as-of snapshots; re-read evidence before an operational decision.
- Inspect report, comparison and history states. A CLI exit code of zero means a structured report was returned, including partial/unsupported results. Provider failures are sanitized; no raw samples, query strings, provider descriptions or unrelated label values are exported.

The same operation is `POST /api/intents/evidence`, `Client.PrepareIntentEvidence` and read-only MCP `prepare_intent_evidence`. Tenant credentials and source endpoints remain configured outside the request. The Go client's default operation timeout is 80 seconds; explicit custom HTTP timeouts and caller deadlines remain effective.

Independent acceptance: `make test-intent-evidence`. It uses local fixtures, with no paid-provider keys or model. Evidence reads are independently usable; attach a reviewed snapshot using the conversation command below.

## Inspect saved preparation gaps

```sh
nines --context production --agent intent conversations status CONVERSATION_ID
```

Before a change set is frozen, status includes `preparationChecks` for the saved typed goal, inline or legacy measurement, compilation, exact human confirmation and historical sampling. Checks distinguish missing, invalid, recorded and stale evidence. Missing history is `not_recorded`, not a failed backtest or an instruction to invent a range.

These checks read the saved conversation only. `current_provider_evidence: not_checked` explicitly avoids claiming current source/catalog validity, fresh traffic or deployed rules. Use the existing measurement-verification or backtesting workflow for a separately requested provider read. A matching saved confirmation is not change approval. The existing phase/next-action fields and server authorization gates remain authoritative; diagnostics do not start work or change those gates.

The same optional field is returned by `GET /api/intent-conversations/{id}/status`, the Go client and MCP `get_intent_workflow_status`. Frozen change sets retain their existing review and verification flow. Evidence receipt attachment is available below. Complete candidate-receipt assembly remains planned.

## Attach evidence to a reviewed proposal

After preparing an inline measurement and confirming its exact semantics, read the current conversation status. Copy its exact version and intent digest into an operational request:

```yaml
expectedVersion: 12 # Replace with the current conversation version.
intentDigest: <exact intent digest from status>
readTelemetry: true
# Optional history and explicit alternatives use the evidence request fields above.
```

```sh
nines --context production --agent intent conversations evidence CONVERSATION_ID -f evidence.yaml
nines --context production --agent intent conversations status CONVERSATION_ID
# Freeze the recommendation using the newly returned conversation version.
```

- This command reads telemetry and writes an audited preparation receipt. It does not change an SLO, confirm measurement semantics, approve a recommendation or deploy rules. Models are optional. Human confirmation must already match the ready inline measurement.
- The server generates the report from the saved goal and exact measurement/source/catalog/revision. Requests cannot upload reports, select credentials or change tenant routes. No history is queried unless explicitly requested.
- Inspect partial/unsupported states, quality gaps and timestamps. A saved receipt is an as-of snapshot, not continuously verified health. Status reports its saved consistency separately from `current_provider_evidence: not_checked`.
- Collection requires the current conversation version. Concurrent edits, revoked authorization and changed source/catalog/SLO revisions reject attachment. Clarifications, selection changes and new runs clear the current receipt. A frozen recommendation retains its own copy; its approval digest covers that copy.
- Freeze the change set after inspecting evidence. Freezing rechecks receipt provenance against current configuration. Frozen change sets cannot have their evidence refreshed; prepare a new proposal to change evidence. Existing proposals without receipts retain their existing workflow.
- The request is limited to 4 KiB and collection has an 80-second deadline. The Go client's default timeout is 85 seconds and the CLI operation allows 90 seconds within the overall command timeout. Explicit client timeouts remain effective.

The same write operation is `POST /api/intent-conversations/{id}/evidence`, `Client.RecordIntentEvidence` and write-enabled MCP `record_intent_evidence`. Owners, editors and agents may collect after human confirmation; viewers cannot. Agents still cannot confirm measurements or approve/apply their own proposals.

Fixture acceptance: `make test-intent-attachments`. PostgreSQL coverage requires `TEST_DATABASE_URL`; no live provider or model credentials are needed.

## Review the frozen proposal packet

```sh
nines --context production --agent intent conversations report CONVERSATION_ID > review.json
nines --context production intent conversations report CONVERSATION_ID --markdown > review.md
```

- New proposals include `changeSet.recommendation.reviewPacket`. It freezes the typed goal, exact human measurement confirmation, saved preparation checks and resource summaries into the existing approval digest. Inline measurements include their exact population filters, success labels, observation point, target, window, lifecycle and burn policies. Legacy proposals include their compiled shared-measurement definition.
- `resources` identifies each SLO/CUJ change by kind, name, action, expected revision and manifest digest. Review the full before/after definitions in the enclosing `changeSet.changes`; summaries do not replace those diffs.
- `destination` records the logical Nines tenant, project, service, source and opaque source fingerprint. Its state is always `not_verified`. It contains no endpoint, credential or upstream tenant-header value. Native publication, rule loading, provider evaluation and observed SLI require separate activation verification.
- Saved provider receipts remain in `changeSet.recommendation.preparationEvidence`. Packet checks distinguish recorded, missing and stale evidence; recording evidence never grants approval. Missing history remains explicit and is not queried by export.
- `candidateDiscovery` carries the newest saved discovery receipt, its run/step digests and server-stamped goal digest. States are `recorded`, `prior_goal`, `unverified`, `stale` or `invalid`. Invalid and scope/source/catalog-mismatched receipts omit their report. A newer invalid receipt never falls back to an older successful one. `recorded` describes saved provenance, not selected-candidate identity, successful telemetry, current configuration or approval.
- `exclusions` records explicitly authored observation boundaries and the exact human measurement confirmation. Omission remains `not_recorded`; an explicit assertion of none is different. The packet and its definitions remain covered by the existing approval digest.
- Export reads the saved, audit-verified change set. It makes no provider/model calls, performs no application and does not rebuild context from a later goal or source configuration. Older frozen proposals without a packet remain readable; preparing a new proposal creates one.

The HTTP review-artifact endpoint, typed Go client, CLI JSON/Markdown output and read-only MCP `get_intent_review_artifact` carry the same frozen packet. It can contain sensitive definitions and review notes; share it only with authorized reviewers. Current RBAC, exact approval/application checks and independently reviewed rollback remain unchanged.

Independent fixture acceptance: `make test-intent-review-packet`.

## Record observation exclusions

Use one of these optional fields inside an intent's `spec`:

```yaml
exclusions:
  none: true
```

Or record up to eight distinct observation boundaries, at most 500 bytes each:

```yaml
exclusions:
  observationBoundaries:
    - Requests that never reach this server are not observed by its completion counter.
```

- These are review notes about the observer's limits. They do **not** add query filters, discard traffic, convert missing data to success or prove the notes correct. Eligible events still come from the measurement's exact population, success criteria and observation point.
- Do not combine `none: true` with boundaries. Omission means not recorded, not that everything is observable.
- In the portal measurement editor, choose **Observation exclusions**. Saving a change invalidates prior measurement confirmation and pending proposals. Confirm the exact goal/measurement again before preparing its new review packet.
- Generated SLOs retain these notes in `spec.intent.exclusions`; recording and alert expressions are unchanged by the notes.

## Continue a clarification without losing typed facts

A text-only answer can explicitly preserve the current typed goal:

```yaml
id: clarify-retries
expectedVersion: 4
text: Retry attempts are counted separately.
preserveGoal: true
```

```sh
nines --context production --agent intent conversations message CONVERSATION_ID -f answer.yaml
```

- The portal uses this mode for replies to a conversation that already has a typed goal. It keeps the target, rolling window, measurement and recipe provenance. Replies add context; they do not edit typed fields. Use the definition editor or an explicit replacement `goal` to change them.
- `preserveGoal` requires an existing typed goal and cannot accompany a replacement goal. The default API/CLI behavior without the flag is unchanged.
- Every new answer clears prior measurement confirmation, compiled plans, provider evidence and pending proposals. Exact request retries remain idempotent; a different flag or body under the same message ID is a conflict.
- Authoring skill `0.6.0` requires each `ask_user` call to declare fields. A deterministic check returns `fields_already_provided` to the model if the question names supplied typed facts or pinned scope; that question is not displayed. Unknown semantics remain questions, and existing tool-call limits still apply. This guard checks declared fields, not the meaning of arbitrary prose; real-model acceptance is separate.
- The same `preserveGoal` input works through HTTP, Go client, CLI YAML and write-enabled MCP `append_intent_answer`. It does not grant confirmation, approval or application authority.

Independent fixture acceptance: `make test-intent-proposals`. Set `TEST_DATABASE_URL` to an isolated local PostgreSQL instance to include persisted-store variants. Tests create and remove unique schemas; no live provider or model key is required.

## Verify activation after application

Run `nines --context production --agent intent conversations activation CONVERSATION` after an independently approved change is applied. It checks configured publication/evaluation/output endpoints and saved collection/measurement evidence for the exact applied revisions. This is an explicit provider read; it never publishes rules or collects samples. `pending` includes a next action and retry guidance, and missing data cannot become verified activation. Inspect attainment separately: working rules do not mean the target is met. See [headless activation inspection](headless-workflows.md#inspect-current-activation).
