# HTTP API

Nines provides a tenant-scoped HTTP API for SLOs, critical user journeys and reviewed intent workflows. This guide covers common endpoints. The pre-release API can change. The [versioned OpenAPI contract](../../api/openapi/v1alpha1.yaml) describes the core SLO, scoped intent context and candidate discovery, reviewed intent workflow and Datadog/New Relic native-result/catalog/draft operations and CLI device-authentication operations listed in that file; other endpoints and complete resource schemas are not yet covered. It is OpenAPI 3.1.1, contract version 0.1.0-alpha.4. Nines does not yet provide Swagger UI.

## Authentication and tenant scope

Use your Nines server URL, an authorized API key and the tenant ID selected by your administrator. Membership and roles are enforced by the server; a tenant header alone does not grant access. Keep credentials in a secret manager and use HTTPS outside loopback.

```sh
# Set NINES_URL, NINES_TENANT and NINES_API_KEY in your environment.
curl --fail-with-body \
  -H "Authorization: Bearer $NINES_API_KEY" \
  -H "X-Nines-Tenant: $NINES_TENANT" \
  "$NINES_URL/api/slos"
```

See [identity and tenants](deployment/identity-and-tenants.md) for SSO, sessions and roles. Provider credentials and Mimir organization mappings are trusted server configuration, separate from the Nines tenant header.

## CLI authentication discovery and device login

These operations use the versioned Nines JSON protocol and shared Go DTOs. They are not upstream OAuth device-grant endpoints. Bootstrap requests do not send a resource API key; identity and tenant membership are enforced when the user approves and when the issued credential is used.

| Method and path                | Purpose                                                                                |
| ------------------------------ | -------------------------------------------------------------------------------------- |
| `GET /auth/cli/capabilities`   | Discover browser/device support, API-key fallback, refresh support and session limits. |
| `POST /auth/cli/device/start`  | Start a five-minute request with `tenant` and S256 `challenge`.                        |
| `POST /auth/cli/device/token`  | Poll with `deviceCode` and `codeVerifier`, respecting `intervalSeconds`.               |
| `POST /auth/cli/device/cancel` | Best-effort cancellation with the same secret and verifier.                            |

- Device polling returns HTTP 200 with a typed `status`: `authorization_pending`, `slow_down`, `authorized`, `access_denied`, `expired_token` or `cancelled`. Invalid input, verifier mismatch, throttling and service errors use the documented HTTP error responses.
- Only `authorized` includes a one-time Nines `token`, `identity`, `tenant` and `expiresAt`. Never log tokens, device codes, verifier values or request bodies. The browser URL contains a distinct approval ticket.
- The Go client offers `CLIAuthenticationCapabilities`, `StartCLIDeviceAuthorization`, `PollCLIDeviceAuthorization` and `CancelCLIDeviceAuthorization`; bootstrap calls omit its resource bearer credential. The application must validate the returned verification origin and expiry, protect secrets and implement the polling/cancellation policy. The shipped CLI performs those checks.
- See [CLI device login and storage](../cli.md#device-login-and-credential-storage) for platform support, expiry, OS credential storage and agent-mode behavior. Remote MCP OAuth remains a separate authentication boundary; agents cannot use a device-login tool to grant themselves access.

## SLOs and critical user journeys

| Method and path                   | Purpose                                                       |
| --------------------------------- | ------------------------------------------------------------- |
| `GET /api/slos`                   | List SLOs in the selected tenant.                             |
| `GET /api/slos/{id}`              | Read an SLO.                                                  |
| `GET /api/slos/{id}/manifest`     | Read its YAML manifest and revision ETag.                     |
| `GET /api/slos/{id}/status`       | Read calculated status. Missing data is not a healthy result. |
| `GET /api/slos/{id}/integration`  | Inspect collection, rule destination and runtime evidence.    |
| `GET /api/slos/{id}/revisions`    | List archived definition revisions.                           |
| `GET /api/journeys`               | List critical user journeys.                                  |
| `GET /api/journeys/{id}/manifest` | Read a journey manifest.                                      |
| `GET /api/slo-templates`          | List available SLO templates.                                 |

## Validate, plan and apply YAML

`POST /api/resources/validate` checks a versioned YAML resource. `POST /api/resources/plan` plans it against the selected tenant. Both use `Content-Type: application/yaml`. Validation does not prove telemetry exists or rules are active.

```sh
curl --fail-with-body -X POST \
  -H "Authorization: Bearer $NINES_API_KEY" \
  -H "X-Nines-Tenant: $NINES_TENANT" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/request-success.slo.yaml \
  "$NINES_URL/api/resources/plan"
```

`POST /api/migrations/convert` converts [Sloth or Pyrra](migration.md) definitions into Nines SLO YAML with notes and skipped items. It is stateless and saves nothing. With [Git delivery](git-delivery.md) configured, `POST /api/gitops/proposals` opens a pull or merge request for a reviewed SLO or journey instead of applying it. After review and authorization, `POST /api/resources/apply` accepts the YAML and requires a quoted `If-Match` revision; `"0"` creates a resource. Use the current revision for edits and review the new state after a conflict. Applying can trigger owner-configured automatic rule delivery; it does not prove rule evaluation or SLI health. See [SLO editing](slo-editing.md) and [automatic deployment](deployment/automatic-rules.md).

## Agent workflows and audit

| Method and path                                              | Purpose                                                                                             |
| ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------- |
| `GET /api/slos/{id}/customers`                               | Read per-customer reliability for the SLO's explicit allowlist, optionally with `expectedRevision`. |
| `GET /api/dependency-maps/{id}/composition`                  | Check availability targets against the ceiling set by critical dependencies.                        |
| `GET /api/intent-conversations/usage`                        | Read aggregate workflow progress: stages reached and median times.                                  |
| `GET /api/intent-conversations/{id}/status`                  | Read the current workflow phase.                                                                    |
| `GET /api/intent-conversations/{id}/activation`              | Explicitly inspect current backend activation for applied revisions.                                |
| `GET /api/intent-conversations/{id}/review-artifact`         | Export a bounded audit-verified review snapshot.                                                    |
| `POST /api/intent-conversations/{id}/evidence`               | Collect and save server-generated evidence for an exact, human-confirmed intent.                    |
| `POST /api/intent-conversations/{id}/change-set`             | Prepare an exact change for review.                                                                 |
| `POST /api/intent-conversations/{id}/change-set/approve`     | Approve an exact reviewed change when authorized.                                                   |
| `POST /api/intent-conversations/{id}/change-set/apply`       | Apply using a separately authorized identity.                                                       |
| `GET /api/intent-conversations/{id}/change-set/verification` | Read verification evidence.                                                                         |
| `GET /api/audit`                                             | Search the tenant audit journal, subject to authorization.                                          |

Each workflow operation has version, digest and authorization requirements. See [headless workflows](headless-workflows.md) and [approval and rollback](intent-governance.md). The [Go client](https://github.com/ishantanu/nines/tree/main/client) provides typed calls; [MCP](mcp.md) describes agent tools.

## Contract versioning and limits

The [contract source](https://github.com/ishantanu/nines/blob/main/api/openapi/v1alpha1.yaml) is versioned separately from the YAML resource API version `nines.dev/v1alpha1`. Existing HTTP routes are not URL-versioned. The specification documents the bearer-token integration profile; browser sessions have separate CSRF requirements. Read the spec's path list rather than assuming it covers every server endpoint. Some response and manifest objects deliberately remain open until their full schemas are specified; server validation remains authoritative.

Contract tests check documented routes, references and key request and response fields against the server's Go types. Expand the specification and those tests together when adding coverage or changing a documented contract. A future stable release needs a published compatibility and deprecation policy and complete schema/endpoint coverage.

## Native provider SLOs

These read-only operations use the configured Datadog or New Relic **native destination**, not client-selected credentials or routes. They are included in the versioned contract.

- `GET /api/slos/{id}/native-results?from=RFC3339&to=RFC3339`: read the exact published current revision. Past, whole-second ranges are limited to 31 days. New Relic clips inward to complete buckets; returned `from`/`to` show the effective range. The response is independent from Nines measurements, with nullable attainment/budget, history, provenance and limitations.
- `GET /api/connections/{connection}/native-slos`: bounded vendor catalog. New Relic is restricted to the configured associated entity/account; Datadog to the configured credential scope. Catalogs exceeding their bounds fail rather than claim completeness.
- `GET /api/connections/{connection}/native-slo-draft?nativeID=...&name=...&project=...&service=...`: prepare YAML for a new experimental Nines SLO. This does not save, adopt, synchronize or modify the vendor SLO. Review through the normal plan/apply workflow. Alerts, corrections, notifications and vendor ownership are not imported.

Datadog, New Relic, VictoriaMetrics, Dash0 and Google Cloud share the Nines resource API; there is no separate public API namespace per vendor. The [provider capability guide](provider-adapters.md) describes query languages, native publication and supported subsets. Connection management and rule plan/apply operations are not yet fully covered by the partial OpenAPI contract.

See [native results and reviewed import](provider-adapters.md#native-results-and-reviewed-import) for CLI examples and supported subsets.

## Remote agent transport

The optional [OAuth-protected remote MCP adapter](remote-mcp.md) is a separate process/endpoint, not `/mcp` on the Nines HTTP API server. It reuses these tenant-scoped resource endpoints with distinct server-held credentials per client/user. JSON-RPC framing follows MCP; the [versioned RemoteMCP configuration schema](../../api/mcp/v1alpha1.schema.json) is separate from this partial OpenAPI contract.

### Burn-window duration inputs

Versioned YAML resources accept `shortWindow: 5m` and `longWindow: 1h` in burn-rate policies. JSON resource input accepts the same strings. Legacy numeric `shortWindowSeconds` and `longWindowSeconds` remain accepted, with exactly one representation per window. Runtime JSON responses continue to return numeric seconds. See [measurement duration syntax](measurements.md#burn-alert-window-durations).

## Intent preparation reads

| Method and path                | Purpose                                                                                                                 |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `GET /api/intent-context`      | Read bounded saved service/source/project context, with an optional revision-pinned CUJ. No provider requests.          |
| `POST /api/intents/candidates` | Explicit bounded provider discovery for an exact scope and population. No writes, model calls or semantic confirmation. |

Candidate requests require JSON with canonical field names, no duplicate keys and no query parameters. See [request YAML, limits and result states](intents.md#discover-measurement-candidates). These operations are covered by the versioned OpenAPI contract and typed Go client.

### Measurement evidence preview

`POST /api/intents/evidence` is a tenant-authorized read-only POST, covered by the versioned OpenAPI contract. It accepts a complete inline intent, expected saved revision, optional provenance pins, explicit alternatives and an optional historical range. First reviews capture current digests; pinned requests reject mismatches. Responses retain quality/freshness gaps and require human confirmation. See [limits and interpretation](intents.md#inspect-evidence-and-compare-explicit-choices).

### Frozen intent review packet

Newly prepared change sets include `recommendation.reviewPacket`, described by the versioned OpenAPI schema. The [review-artifact operation](intents.md#review-the-frozen-proposal-packet) carries it unchanged through HTTP, the Go client, CLI and MCP. The packet is covered by the approval digest. Its logical destination is explicitly unverified; an exported proposal does not establish provider activation. Older proposals may omit this additive field.

## Clarification replies and frozen preparation context

`POST /api/intent-conversations/{id}/messages` is covered by the versioned contract. Its `AppendIntentConversation` body accepts optional `preserveGoal: true` to retain a saved typed goal while adding context. It rejects simultaneous replacement `goal`, invalidates earlier confirmation/proposals and preserves exact-request idempotency. Go client, CLI YAML and write-enabled MCP use the same input.

`IntentReviewPacket` includes optional typed `candidateDiscovery` and `exclusions`. They are server-generated, frozen review context covered by the existing approval digest, not request-uploadable evidence or authorization. See [states, bounds and examples](intents.md#review-the-frozen-proposal-packet).

## Intent activation inspection

`GET /api/intent-conversations/usage?days=N` returns an `IntentUsage` summary of conversations created in the last N days (1–365, default 90). It uses tenant reader permissions and only reads stored conversations. The response contains counts and medians, never an actor, goal text, measurement or telemetry value, and nothing is sent outside the server. It reads at most 500 conversations and sets `truncated` when it stops early. It describes workflow progress, not SLO health.

`GET /api/intent-conversations/{id}/activation` returns a versioned `IntentActivation` receipt for an applied change set or its applied rollback. It uses tenant reader permissions and explicitly performs bounded provider reads against configured destinations. It rejects query overrides and concurrent resource/source changes. It does not publish rules, collect samples, update outcomes or modify definitions.

The receipt separates exact saved revisions, publication, evaluation, recorded output and usable collected measurement evidence. Inspect its `state`, `code`, `nextAction` and optional `retryAt`. A successful HTTP response may report pending or unsupported activation. This is distinct from provider-free workflow status and definition verification. See [activation usage and limits](headless-workflows.md#inspect-current-activation).
