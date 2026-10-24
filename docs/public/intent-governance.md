# Recommendation review, audit and rollback

Intent workflows prepare proposals. They do not authorize production changes. The server requires an owner or editor to approve the exact change-set digest before applying definitions. The agent role cannot confirm measurement semantics, approve, reject, apply or roll back definitions. The workflow model has no approval or resource-write tool.

## Understand a recommendation before approving it

Use **SLO intents**, open a prepared conversation and select **Review changes**. The review freezes the following into the change-set digest:

- The goal, target, window, selected measurement revision and source fingerprint.
- Before-and-after target, window, alert policy and critical user journey changes, with explanations of their implications.
- Metric discovery interval and evidence digest, plus a matching historical backtest summary when one was actually run. Missing historical validation is stated explicitly.
- The preparation run, model route and skill version/digest. A guided run is identified separately from a model-assisted run.
- The visible preparation summary, declared basis, assumptions and limitations. This records the visible supporting evidence and explicit limitations; it does not guarantee that the recommendation is correct.

The definition form is a read-only preview. **Continue to review and approval** starts the conversation workflow, where the measurement is confirmed again and the same audited change-set approval applies. It no longer applies the SLO and journey independently.

The review also states whether the proposal creates a resource with no rollback baseline, changes restorable definitions, or matches the current configuration. Target diffs show the change in allowed bad-event fraction before approval.

The target and policies follow the reviewed goal. Nines does not infer acceptable business risk, certify telemetry completeness or guarantee that a different target prevents an incident. Review excluded traffic and observation points, not just whether the metric exists. A higher target reduces the error budget for the same traffic. Changing the rolling window changes the evaluation horizon and required coverage.

Changing the goal, measurement selection or source invalidates the existing review. Old proposals remain in saved history. A changed model/skill route fences unfinished preparation runs rather than silently continuing under a different execution contract.

## Approve, reject or withdraw

The portal asks for a reviewer note and acknowledgement of the rationale, measurements, settings and limitations. Approval records the reviewer, time, note and exact digest. **Apply approved changes** checks conversation, resource, source and measurement revisions again. The SLO and explicitly proposed critical user journey link commit together or neither does.

A reviewer can reject a proposal or withdraw an approval before apply, with a reason. A rejected proposal cannot be applied. Preparing a new review creates a new change-set identity; old approval is not reused. Approval and apply require different authenticated subjects, both with an owner/editor role. This also applies to rollback. Reusing the approving credential to apply is rejected and audited. Two credentials held by one person are not proof of two human reviewers; use individually assigned SSO accounts and control credential issuance.

The server requires a nonempty review reason for approval, rejection and rollback review through every client, including CLI, API and portal. Reasons are limited to 1,000 bytes and reject recognized credential patterns. Use individual SSO accounts for personal attribution; a shared API credential identifies that credential rather than the engineer using it.

Permissions come from the authenticated server role. CLI `--agent` selects deterministic machine output; it does not downgrade an owner credential. Give automated assistants the server's `agent` role rather than an owner/editor credential.

Definition application performs no provider writes inside its transaction. After commit, an owner-configured automatic data source reconciles its recording/alert rules in the background, including after reviewed rollback. A manual data source still requires separate reviewed apply in **Rules** or the CLI. This policy does not authorize the agent to confirm or approve changes. See [automatic rule deployment](deployment/automatic-rules.md). A stored definition or receipt does not prove rule evaluation, SLO attainment or notification delivery.

## Audit records

The conversation's **Decision audit trail** identifies proposal, approval, rejection, application and rollback events, including authenticated actors, timestamps, conversation versions, exact digests, outcomes and review reasons. Authenticated governance denials, proposal-preparation failures, admitted conflicts and malformed governance requests routed to an existing conversation are recorded without request bodies or raw provider errors. Invalid requests rejected before routing and infrastructure failures may only appear in operational logs.

**Download audit JSON** retrieves a stable, paginated snapshot of all decision events, with the change-set definitions/recommendations still retained in the conversation. The portal refuses an incomplete export above 10,000 events and directs you to the CLI. The independently stored journal retains proposal snapshots even when the conversation no longer contains them; use **Audit log → View** or `nines audit show RECORD_ID` to retrieve those snapshots. A new proposal snapshot records the state at the time of that event; subsequent approval/application are separate journal events. When the journal is first introduced, complete valid event chains retained in existing conversations are recovered once, with `recoveredFromConversation: true`. These reconstructed proposal snapshots preserve definition/recommendation digests but may contain later lifecycle metadata from the retained object; decision events identify the historical state. Ordinary startup does not recreate deleted journal rows. Incomplete or invalid legacy chains remain invalid and are not silently repaired. The server verifies the event sequence/hash chain and proposal/rollback digests. A failed integrity check blocks further change-control mutations until the records are inspected. The export reports `integrity: invalid` rather than concealing a detected mismatch.

Successful definition transactions include approval identity, reason, operation digest and recommendation digest in their database receipt. If resource commit succeeds but conversation persistence fails, an explicit retry reads the receipt without applying twice and preserves the original applying identity. Inspect readback and retry the exact request after an ambiguous response; never assume a transport error means no change occurred.

Events are append-only through the application. There is no audit-clear endpoint. They persist in a separate PostgreSQL journal, in the same transaction as each conversation update. The explicit in-memory store retains them only for the process lifetime. Conversations keep the latest 256 events for display; older events and proposal snapshots remain in the journal. The 2 MiB conversation size and bounded proposal history still apply, but the recent-event display limit no longer blocks reviews. Infrastructure failures or a full conversation document can prevent recording an attempt. Review transitions and journal inserts commit together. Resource application has a separate durable transaction receipt: if resource commit succeeds but conversation/journal persistence fails, the response is ambiguous. When possible, the journal records `uncertain`, never a claim that definitions were unchanged. An exact authorized retry recovers the receipt and the original applying identity without applying twice. No retention job removes journal rows. Monitor database size and include the journal in backups.

This is an application integrity check, not signed storage or an externally protected immutable archive. A database administrator can modify or remove database contents. Include the database in backups and keep exports under your organization's retention/access policy. An optional owner-configured [S3 Object Lock archive](deployment/audit-archive.md) delivers journal records in the background after verified version/content/retention readback. It is disabled by default. Export to a private temporary file, check the command exit status, then move the complete result into your organization's separately managed archive. Do not treat the application hash chain as an external trust anchor. Exported actor names, review notes and configuration leave server-side access and retention controls.

## Reviewed configuration rollback

For an applied change set, select **Prepare rollback review** and provide a reason. The server constructs the restore plan from its saved previous definitions; a model does not supply the restore payload. Review the diff, enter a rollback approval note, approve its new digest, then explicitly apply it.

Rollback restores existing SLO/CUJ definitions as **new revisions**. It checks the exact revisions and digests from the original apply and refuses to overwrite any later edit. The restore batch is atomic and has its own retry receipt. Configuration verification identifies the restored revision, and the original apply and rollback remain in the audit trail. Historical change sets remain eligible for review while retained in the conversation.

Limits are explicit:

- **New resources:** creation has no previous definition. A rollback containing a newly created resource is blocked as a whole, with an explanation. It does not delete the SLO or partially unlink its journey. Resource retirement/deletion is not implemented by this feature.
- **External rules:** rollback restores Nines definitions only. Existing Prometheus/Mimir rules continue until deployment updates them: automatic sources queue the restored revision after commit; manual sources require separate reviewed apply. Their integration status can show an out-of-date definition until then.
- **Measurements:** a restored definition follows normal revision-scoped measurement behavior. Fresh collection/coverage is required; cached observations are not reinterpreted as evidence for a different revision.
- **Error budgets:** restoring configuration cannot undo consumed budget, restore lost requests or erase an incident. There is no automatic rollback triggered by budget burn.
- **Intervening edits:** a conflict requires investigation and a new deliberate configuration change. Rollback does not rebase over another engineer's work.

Rollback approval can also be rejected or withdrawn before application. The rejected plan remains in history. A new preparation requires a new digest and review.

## CLI and Go client

All operations use the explicit server/tenant context. Operational YAML is separate from resource YAML. For example, inspect and approve an existing proposal:

```sh
nines intent conversations show CONVERSATION --context production --agent
nines intent conversations audit CONVERSATION --context production --agent

# approval.yaml
# expectedVersion: <conversation version>
# digest: <exact reviewed change-set digest>
# confirm: true
# reason: "Reviewed population, excluded traffic, target impact and available history."
nines intent conversations approve CONVERSATION --context production -f approval.yaml --agent

# Apply uses the returned conversation version and a different owner/editor identity.
# Both contexts must point to the same server and tenant.
nines intent conversations apply CONVERSATION --context production-applier -f apply.yaml --agent
```

For rollback, `rollback-plan` takes the original applied change-set digest and a reason. `rollback-approve` takes the returned rollback digest and a reviewer reason. `rollback-apply` takes that rollback digest and the latest approved conversation version. Each request contains `confirm: true`; new rollback reviews never reuse original approval.

```sh
nines intent conversations reject CONVERSATION --context production -f rejection.yaml --agent
nines intent conversations rollback-plan CONVERSATION --context production -f rollback-plan.yaml --agent
nines intent conversations rollback-approve CONVERSATION --context production -f rollback-approval.yaml --agent
nines intent conversations rollback-apply CONVERSATION --context production -f rollback-apply.yaml --agent
nines intent conversations rollback-reject CONVERSATION --context production -f rollback-rejection.yaml --agent
```

Tenant-wide search is available to owners/editors under **Audit log**, at `/tenants/TENANT/audit`, and through `GET /api/audit`. Conversation-level audit reads retain existing conversation read permissions. Search supports exact actor subject, resource name, conversation, action, outcome and RFC3339 time range. The default range is the last 30 days; a request can span at most 366 days. Page size is 1–100, and a cursor is bound to the same tenant, filters and frozen time range. It is a pagination marker, not authorization.

```sh
nines audit list --context production --resource api-availability --agent
nines audit list --context production --outcome denied --from 2026-10-01T00:00:00Z --to 2026-10-24T23:59:59Z --agent
nines audit show RECORD_ID --context production --agent
# Full matching records, including proposal snapshots, as NDJSON.
# Redirect into a private temporary file and check success before archiving.
(umask 077; nines audit export --context production --from 2026-10-01T00:00:00Z --to 2026-10-24T23:59:59Z > decisions.partial.ndjson) && mv decisions.partial.ndjson decisions.ndjson
```

`GET /api/audit/{recordID}` returns one tenant-scoped record and its frozen proposal when present. Tenant-wide list responses omit the large proposal bodies. `GET /api/intent-conversations/{id}/audit` returns at most 256 events, `through` and an optional `nextAfter`; continue with `?after=NEXT_AFTER&through=THROUGH` until `nextAfter` is absent. The server verifies each page and its link to the preceding event. Do not retain only the first page as a complete history. The portal handles this pagination automatically.

The Go client exposes `Audit`, `AuditRecord`, `IntentAuditPage`, `IntentAudit`, `RejectIntentChangeSet`, `PrepareIntentRollback`, `ApproveIntentRollback`, `ApplyIntentRollback`, `RejectIntentRollback` and `ExportIntentRollbackKubernetes`. The MCP adapter exposes paginated `get_intent_audit` reads (`id`, optional `after` and `through`) and existing read-only conversation evidence; it does not offer approval, apply or rollback mutation tools.

## Kubernetes operator and GitOps

For portal-managed operator destinations, apply the reviewed configuration rollback first, then review/publish its new SLO revision through **Rules**. The operator reconciles the new desired configuration; configuration persistence, rule activation and output evidence remain distinct.

For standalone operator/GitOps users, the rollback panel or CLI can export the exact restore definitions with the original/rollback digest annotations:

```sh
nines intent conversations rollback-kubernetes CONVERSATION \
  --context production --namespace team-commerce --digest ROLLBACK_DIGEST > rollback.yaml
```

This only creates a local artifact for a separate GitOps review. It does not connect to Kubernetes, apply resources, rewind Kubernetes generations or take ownership of portal-managed resources. Keep the existing GitOps revision and provider status evidence as part of the review.
