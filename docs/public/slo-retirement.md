# Retire an SLO

Retirement is an explicit tenant-owner operation with a reviewed plan, exact revision and digest.

- It is separate from the `deprecated` lifecycle: deprecated objectives keep collecting measurements; archived objectives stop collection and alert evaluation.

## Archive and permanent deletion

- **Archive** freezes the objective and retains its definition, current measurements and previous revision snapshots in PostgreSQL. The summary uses the archive-time horizon.
  - Historical reads remain available and do not collect new provider data. The objective moves to **SLOs → Archived objectives**.
  - Archived definitions cannot be edited or republished.
  - This release has no unarchive action; export the retained YAML and review a new identifier if the measurement needs a new lifecycle.
- Archival increments the SLO revision, records the authenticated actor and reviewed digest, cancels pending firing notifications and queues resolution for active notification episodes.
  - An outbound request already issued before archival may finish.
  - Nines rejects late collector results and stops future collection, measurement checks, rule publication and alert evaluation.
  - It waits at least 30 seconds before managed cleanup to let bounded provider requests finish.
  - The normal server retries cleanup after restart and provider outages.
  - Provider rules can continue evaluating until cleanup is verified; pending archival does not prove that external alerts have stopped.
- **Permanent deletion** requires an archived objective whose owned-rule cleanup is complete.
  - It erases the Nines SLO, retained measurements, revision snapshots and notification outbox entries.
  - An audit receipt remains, including the reserved identifier, actor, action, times, revision, digest and cleanup result. The identifier cannot be reused.
  - Historical journey definitions and run evidence are retained; deletion does not rewrite that evidence.
  - Backups, exported files and provider-side time series follow their separate retention policies.
  - Deletion does not erase Prometheus or Mimir's historical samples.
- Only owners can archive, retry cleanup or permanently delete.
  - Viewers, editors and agent credentials can inspect the plan and receipt.
  - CLI `--agent` changes output formatting; it grants no permission.
  - The MCP server adds no retirement mutation tools.
  - Existing definition writes cannot reactivate an archived SLO.

## Resolve references first

- Open an objective's **Configuration → Retire objective → Review archival**.
  - The review lists current critical user journeys, each required/advisory link and its revision, plus dependency-map references.
  - Both required and advisory references must be removed or redirected through their normal reviewed edits before archival.
  - Nines rejects a concurrent reference added after a plan was prepared; it does not silently detach a journey or infer a substitute SLO.
- Refresh the review, inspect the objective revision, owned group and nonsecret destination fingerprint, then select **Archive reviewed objective**. **Archived objectives → Configuration** shows cleanup status, attempts and the retained audit. **Check owned-rule cleanup** retries the persisted action when its backoff is due; it cannot accept another destination. Once cleanup is complete, **Review permanent deletion** requires typing the exact identifier before applying.

## Owned rule cleanup

- Nines pins its trusted rule destination before a server-managed publication.
  - Credentials are resolved from the current trusted connection and are never included in retirement receipts.
  - Credential rotation is supported; changing the destination while cleanup is pending is not.
  - Keep the original connection, namespace, shared directory and permissions available.
  - If static configuration changed, restore the original route and retry.
  - A missing or changed destination blocks cleanup rather than following another URL.

* **Prometheus:** inspect and remove only the single regular rule file owned by this SLO and tenant, sync its directory, optionally request the configured reload, then verify that the group is absent from the evaluator API. With automatic reload disabled, arrange the reload yourself. File removal alone does not mark cleanup complete.
* **Mimir:** inspect ownership in the original ruler namespace, delete only that group, and wait for evaluator absence. Polling delay or an unavailable ruler remains pending/failed and retries with bounded backoff.
* **Operator:** withdraw only the reviewed owned parent and its frozen resource UID, reject a replacement at the same name, retain its original route through foreground deletion/finalizers, and check provider runtime absence. The Kubernetes controller remains responsible for its owned children and Mimir finalizer.
* A blocked ownership check requires inspection of the original resource UID and destination.
  - A replacement at the same Kubernetes name is not automatically removed.
  - A credential failure or ambiguous response never grants permission to remove foreign rules.
  - Raw provider errors and credentials are not returned.
  - Nines manages its configured owned group; it cannot discover arbitrary copies made outside Nines.
  - For an existing active SLO, the separate **Rules → Withdraw reviewed rules** action pauses automation and removes the reviewed owned publication while keeping measurement collection active.
  - Confirm withdrawal before changing its source or rule destination.

## CLI and Go client

Use an authenticated owner context and keep credentials in `NINES_API_KEY` or the private login store:

```sh
nines retire plan api-availability --action archive --context production --agent
# Inspect references/blockers and retain this exact revision and digest:
nines retire apply api-availability --action archive \
  --expected-revision 4 --digest REVIEWED_PLAN_DIGEST --context production --agent
nines retire status api-availability --context production --agent
nines retire retry api-availability --context production --agent

# After cleanup is complete, prepare a separate irreversible-deletion review:
nines retire plan api-availability --action delete --context production --agent
nines retire apply api-availability --action delete \
  --expected-revision 5 --digest REVIEWED_DELETE_DIGEST \
  --confirm-name api-availability --context production --agent
```

- Commands require the server and reject `--offline`.
  - The CLI never fetches and approves a replacement digest automatically.
  - Repeating an already applied exact action recovers its durable receipt.
  - A different revision/digest requires a fresh review.
- The Go client exposes `PlanRetirement`, `ApplyRetirement`, `Retirement` and `RetryRetirement` with the same typed inputs and tenant-bound responses. HTTP equivalents are:

| Method | Endpoint                                      | Purpose                                                                  |
| ------ | --------------------------------------------- | ------------------------------------------------------------------------ |
| GET    | `/api/slos/ID/retirement/plan?action=archive` | Read archive impact and exact digest                                     |
| GET    | `/api/slos/ID/retirement/plan?action=delete`  | Read permanent-deletion impact and exact digest                          |
| POST   | `/api/slos/ID/retirement/apply`               | Apply `action`, `expectedRevision`, `digest`, and deletion `confirmName` |
| GET    | `/api/slos/ID/retirement`                     | Read state/audit, including after permanent deletion                     |
| POST   | `/api/slos/ID/retirement/retry`               | Retry the persisted owned cleanup when due                               |

- Normal deployments require durable PostgreSQL. The in-memory demo resets on restart and is not an audit archive. See [backup and restore](deployment/backup-and-restore.md) before permanent deletion.
