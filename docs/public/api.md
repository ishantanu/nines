# HTTP API

Nines provides a tenant-scoped HTTP API for SLOs, critical user journeys and reviewed intent workflows. This guide covers common endpoints. The pre-release API can change. The [versioned OpenAPI contract](../../api/openapi/v1alpha1.yaml) describes the core SLO and reviewed intent workflow operations listed in that file; other endpoints and complete resource schemas are not yet covered. It is OpenAPI 3.1.1, contract version 0.1.0-alpha.2. Nines does not yet provide Swagger UI.

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

## SLOs and critical user journeys

| Method and path | Purpose |
| --- | --- |
| `GET /api/slos` | List SLOs in the selected tenant. |
| `GET /api/slos/{id}` | Read an SLO. |
| `GET /api/slos/{id}/manifest` | Read its YAML manifest and revision ETag. |
| `GET /api/slos/{id}/status` | Read calculated status. Missing data is not a healthy result. |
| `GET /api/slos/{id}/integration` | Inspect collection, rule destination and runtime evidence. |
| `GET /api/slos/{id}/revisions` | List archived definition revisions. |
| `GET /api/journeys` | List critical user journeys. |
| `GET /api/journeys/{id}/manifest` | Read a journey manifest. |
| `GET /api/slo-templates` | List available SLO templates. |

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

After review and authorization, `POST /api/resources/apply` accepts the YAML and requires a quoted `If-Match` revision; `"0"` creates a resource. Use the current revision for edits and review the new state after a conflict. Applying can trigger owner-configured automatic rule delivery; it does not prove rule evaluation or SLI health. See [SLO editing](slo-editing.md) and [automatic deployment](deployment/automatic-rules.md).

## Agent workflows and audit

| Method and path | Purpose |
| --- | --- |
| `GET /api/intent-conversations/{id}/status` | Read the current workflow phase. |
| `GET /api/intent-conversations/{id}/review-artifact` | Export a bounded audit-verified review snapshot. |
| `POST /api/intent-conversations/{id}/change-set` | Prepare an exact change for review. |
| `POST /api/intent-conversations/{id}/change-set/approve` | Approve an exact reviewed change when authorized. |
| `POST /api/intent-conversations/{id}/change-set/apply` | Apply using a separately authorized identity. |
| `GET /api/intent-conversations/{id}/change-set/verification` | Read verification evidence. |
| `GET /api/audit` | Search the tenant audit journal, subject to authorization. |

Each workflow operation has version, digest and authorization requirements. See [headless workflows](headless-workflows.md) and [approval and rollback](intent-governance.md). The [Go client](https://github.com/ishantanu/nines/tree/main/client) provides typed calls; [MCP](mcp.md) describes agent tools.

## Contract versioning and limits

The [contract source](https://github.com/ishantanu/nines/blob/main/api/openapi/v1alpha1.yaml) is versioned separately from the YAML resource API version `nines.dev/v1alpha1`. Existing HTTP routes are not URL-versioned. The specification documents the bearer-token integration profile; browser sessions have separate CSRF requirements. Read the spec's path list rather than assuming it covers every server endpoint. Some response and manifest objects deliberately remain open until their full schemas are specified; server validation remains authoritative.

Contract tests check documented routes, references and key request and response fields against the server's Go types. Expand the specification and those tests together when adding coverage or changing a documented contract. A future stable release needs a published compatibility and deprecation policy and complete schema/endpoint coverage.

## Native provider SLOs

These read-only operations use the configured Datadog or New Relic **native destination**, not client-selected credentials or routes. They are included in the versioned contract.

- `GET /api/slos/{id}/native-results?from=RFC3339&to=RFC3339`: read the exact published current revision. Past, whole-second ranges are limited to 31 days. New Relic clips inward to complete buckets; returned `from`/`to` show the effective range. The response is independent from Nines measurements, with nullable attainment/budget, history, provenance and limitations.
- `GET /api/connections/{connection}/native-slos`: bounded vendor catalog. New Relic is restricted to the configured associated entity/account; Datadog to the configured credential scope. Catalogs exceeding their bounds fail rather than claim completeness.
- `GET /api/connections/{connection}/native-slo-draft?nativeID=...&name=...&project=...&service=...`: prepare YAML for a new experimental Nines SLO. This does not save, adopt, synchronize or modify the vendor SLO. Review through the normal plan/apply workflow. Alerts, corrections, notifications and vendor ownership are not imported.

See [native results and reviewed import](provider-adapters.md#native-results-and-reviewed-import) for CLI examples and supported subsets.

## Remote agent transport

The optional [OAuth-protected remote MCP adapter](remote-mcp.md) is a separate process/endpoint, not `/mcp` on the Nines HTTP API server. It reuses these tenant-scoped resource endpoints with distinct server-held credentials per client/user. JSON-RPC framing follows MCP; the [versioned RemoteMCP configuration schema](../../api/mcp/v1alpha1.schema.json) is separate from this partial OpenAPI contract.

### Burn-window duration inputs

Versioned YAML resources accept `shortWindow: 5m` and `longWindow: 1h` in burn-rate policies. JSON resource input accepts the same strings. Legacy numeric `shortWindowSeconds` and `longWindowSeconds` remain accepted, with exactly one representation per window. Runtime JSON responses continue to return numeric seconds. See [measurement duration syntax](measurements.md#burn-alert-window-durations).
