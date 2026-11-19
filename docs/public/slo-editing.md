# Edit an SLO without changing its name

An SLO's name is its stable identity for critical user journey links and clients.

- Targets, rolling windows, lifecycle, alert policies, service, source, sampling interval and queries can be edited as new configuration revisions.
- Correcting a counter selector does not require a new SLO name.

## Portal review and apply

1. Open the existing SLO and select **Edit definition**.
2. Edit its structured measurement in **Measurement**, use **Settings** for custom queries, or edit **YAML**. Keep the same `metadata.name`.
3. In **Measurement**, select **Verify telemetry and review SLO**. In **Settings** or **YAML**, select **Validate and plan**. Review the resulting definition and expected revision.
4. In **Measurement**, confirm that you reviewed the measurement and evidence limitations, then **Save reviewed SLO**. Other tabs require the measurement-reset acknowledgement when applicable before **Apply definition**.
5. Check the telemetry connection deployment policy. Automatic mode queues the new revision after save; manual mode requires **Rules → Apply reviewed rules**. Check runtime evaluation separately.

- The tabs edit one definition.
  - Guided measurement edits appear in YAML; YAML changes repopulate the matching guided fields, including availability versus latency.
  - Settings changes to targets, windows and policies carry back into Measurement.
  - Switching tabs or editing any field discards the previous review and acknowledgement, so saving requires a fresh review.
  - Invalid YAML or a filter that cannot be represented unambiguously keeps the current editor and its input intact until corrected.
  - Existing custom-query and shared-measurement SLOs stay in Settings/YAML; opening Measurement never guesses a replacement measurement.
- The API/CLI uses the same revision checks.
  - A stale editor or client receives a conflict instead of overwriting another change.
  - A matching definition is a no-op and does not create an archive or reset coverage.
- The default SLO owns `spec.measurement`; edit it and the objective together. No separate measurement revision needs changing.
  - A guided edit retains the stored intent description and journey reference, and synchronizes its measurement, scope, objective and policy fields with the resulting reviewed SLO.
  - This authoring context is not a new model recommendation or approval: the original conversation, frozen recommendation evidence and audit history remain attached to their original revisions.
  - Use an intent conversation when a fresh goal/evidence/approval trail is needed.
  - For an explicitly shared legacy measurement, update `spec.measurementBinding` through a verified compilation using the selected binding revision, or select **Use custom queries** in the editor.
  - Switching to custom queries explicitly removes the binding and intent provenance in the proposed revision.
  - Editing queries while retaining a binding that declares different semantics is rejected.
  - Previous revisions retain their original binding reference.

## Review a target change against delivered reliability

Users come to rely on the service level they actually receive, not the one written in a definition. When a plan changes an existing SLO's target, **Validate and plan**, `nines plan` and the plan API add notes that compare the new target with the reliability delivered over the SLO window.

- A target raised above what has been delivered is flagged, because the SLO would start out unmet and its budget would burn from the first minute.
- A target relaxed so far that it allows about ten times more failures than the service delivers is flagged, because users who rely on the current level can experience the drift as a regression. Communicate the change before using the extra room.
- Any change that moves the allowed failure rate by ten times or more is called large, in either direction.
- If the window is not fully measured, the note says the change cannot be compared instead of guessing. If the measurement itself changes, delivered reliability describes the old measurement and is not compared.
- The notes are advice shown with the plan. They never block an apply, and an unchanged target adds nothing.

## What happens to existing evidence

- Every replaced server-managed SLO revision is frozen with its definition, the observations still retained at edit time, collection/alert state, replacement time and authenticated replacing actor.
  - The archive and new definition commit in the same memory-store snapshot or PostgreSQL transaction, including atomic intent apply/rollback.
  - Failed writes and aborted multi-resource batches do not leave a partial archive.
- A measurement edit clears active interval samples, provider-window observations and measurement health.
  - The new revision begins with unknown attainment until fresh collection supplies enough evidence.
  - Outstanding collectors cannot insert old observations or overwrite the new revision's collection health.
  - Target/policy-only interval-count edits retain their compatible underlying samples; provider-window observations remain configuration-revision scoped and require fresh collection.
- Archives preserve evidence that had not already aged out of active retention.
  - They cannot recover expired samples, recreate revisions edited before this feature was enabled, or undo consumed error budget.
  - Archives persist with PostgreSQL and last only for the process lifetime in memory mode.
  - They have no automatic expiry or application purge endpoint in this alpha; include them in database capacity and retention planning.

## Read previous revisions

- The edit dialog's **Previous revisions** section lists archived definitions, actors and evidence counts.
  - The list is paginated and each revision exposes its original YAML.
  - Current SLO charts continue to show the current definition only.
- Read-only, tenant-authorized API endpoints:

* `GET /api/slos/SLO/revisions?limit=20&beforeRevision=0` lists archived revisions, newest first. Pass `nextBeforeRevision` back as `beforeRevision` for the next page.
* `GET /api/slos/SLO/revisions/REVISION` returns one archived definition and metadata.
* `GET /api/slos/SLO/history?revision=REVISION&rangeSeconds=3600` evaluates its retained observations using that revision's original definition and archive-time horizon. Existing alignment, chart limits and optional policy filters apply.

CLI example:

```sh
nines history my-objective --context playground \
  --revision 1 --range 1h --resolution 1m --agent
```

- An archived-revision read does not query a provider, collect data, apply a definition or deploy rules.
  - Authorized owners/editors may deliberately reapply reviewed previous YAML as another new revision.
  - Agent-assisted changes and rollback continue through their separately approved exact-digest workflow.

## Recording rules and operator behavior

- Both interval-count and provider-window recording outputs carry `nines_revision`.
  - Interval aggregate and burn-alert selectors pin the revision as well as tenant, source, SLO and operator resource UID.
  - New rules cannot sum recordings produced by a different measurement revision.
  - Historical series remain in Prometheus/Mimir according to the provider's retention.
- After a definition edit, existing rule deployment/evaluation evidence is out of date until the new revision is deployed and verified.
  - An automatic source queues that deployment after save; a manual source requires reviewed apply.
  - Deployment, successful rule evaluation and measured attainment remain distinct states.
- The Kubernetes operator accepts same-name measurement edits and reconciles rules using the new generation (or the portal's explicit configuration revision).
  - Kubernetes/GitOps remains the definition history for standalone operator resources; server-side archives cover portal/server-managed definitions. Resource ownership checks remain enforced.
  - Moving an operator resource between source/destination routes still requires withdrawal/cleanup on the original route before recreation; an edit does not transfer ownership or leave unmanaged rules behind.
- Server-managed rule destinations stay pinned after publication.
  - Withdraw the owned rules before changing their source/destination.
  - Retiring an objective is a separate [reviewed archive/delete operation](slo-retirement.md); archived objectives retain historical reads and cannot be edited.

## SLO cards and table

The objectives page offers Cards and Table views with the same lifecycle filter and budget ordering. Cards show measured SLI, target, objective window, coverage, budget and linked critical user journeys. Missing measurements remain unavailable; partial windows are labelled. The trend range selects 15 minutes, one hour, six hours or 24 hours of stored Nines history, not a new provider read. Cards display up to 12 objectives per page and request bounded, tenant-scoped history for the displayed current revisions. Archived cards retain their summary without requesting a current trend. The table remains available for comparison across larger inventories.

The portal Home page shows at most six compact SLO cards, prioritised by alert state and remaining budget. All / Needs review filters retain objectives with incomplete evidence. Cards use the same measurement and provider labels as the full inventory, with a fixed last-15-minute stored-history trend; View all SLOs opens the complete Cards / Table view. Archived objectives are excluded.

SLO card values use a short reveal on entry or when their formatted SLI, budget or burn value changes. They show exact reported values immediately to assistive technology and never count through intermediate measurements. Unchanged values do not replay during polling; reduced-motion preferences disable the animation.
