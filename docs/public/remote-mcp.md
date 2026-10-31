# Remote MCP with OAuth

Nines includes an opt-in OAuth-protected Streamable HTTP mode in `nines-mcp`. Stdio remains the default. Both transports use the same tool handlers, tenant-scoped Nines API, RBAC, revision checks and reviewed deployment digests.

This is an **experimental alpha transport**, tested with the official Go SDK and a local OAuth authorization server fixture. Hosted managed-agent clients and production identity-provider configurations still need deployment acceptance; do not assume interoperability with every client.

## Authorization model

1. Register your managed agent client at an external OAuth 2.1 authorization server. Configure authorization-code flow with S256 PKCE, exact redirect URIs and the MCP endpoint as its resource/audience. Client registration, login, consent and refresh belong to that authorization server.
2. Configure an introspection client on the authorization server. Nines supports RFC 7662 introspection using `client_secret_basic`; it does not act as an authorization server, mint OAuth tokens or implement dynamic client registration.
3. Create a **distinct, least-privilege Nines service-account/API credential for each client/user binding**, in the selected Nines tenant. Keep these in a secret manager or deployment Secret. Never use a shared platform-admin credential.
4. Bind the exact OAuth `sub` and `client_id` pair to its Nines credential through a private environment reference. One adapter instance serves one fixed tenant. Use separate instances/audiences for other tenants.
5. On every request the adapter introspects the access token, checks its issuer, resource audience, expiration, not-before time and required scope, and selects only the configured binding. The OAuth token is **never forwarded to the Nines API**. Nines rechecks its own credential and permissions.

The introspection response must include `active`, `sub`, `client_id`, `iss`, `aud`, `scope` and `exp`; `aud` may be a string or array. `nbf`, when present, is enforced. Remaining token validity must be at most one hour. Issuers that omit these fields or lack introspection are outside this initial profile. OAuth-to-Nines role provisioning is an explicit administrative binding, not automatic SSO role linking.

## Configure the adapter

Save a private `remote-mcp.yaml`:

```yaml
apiVersion: nines.dev/v1alpha1
kind: RemoteMCP
publicURL: https://agents.example.com/mcp
server: https://nines.example.com
tenant: production
issuer: https://identity.example.com/realms/nines
introspectionURL: https://identity.example.com/realms/nines/protocol/openid-connect/token/introspect
introspectionClientID: nines-mcp-resource
introspectionSecretEnv: MCP_INTROSPECTION_SECRET
allowWrites: false
allowRuleDeployment: false
allowRuleWithdrawal: false
bindings:
  - subject: USER_SUBJECT_FROM_YOUR_ISSUER
    clientID: REGISTERED_AGENT_CLIENT_ID
    apiTokenEnv: MCP_USER_NINES_API_KEY
```

- Supply the two referenced secrets privately. They are unrelated to Datadog/New Relic/other telemetry vendor keys.
- Blank credentials, duplicate client/user pairs and reuse of the same Nines credential across bindings are rejected.
- The [versioned configuration schema](../../api/mcp/v1alpha1.schema.json) describes the YAML shape; runtime validation adds URL, routing, credential and dependent-flag checks. `RemoteMCP` is process configuration, not an SLO/CUJ resource submitted to `nines apply`.

Run behind a TLS-terminating proxy on the same host/pod network namespace:

```sh
nines-mcp -transport http -remote-config /run/nines/remote-mcp.yaml \
  -listen 127.0.0.1:8081
```

Or serve TLS directly:

```sh
nines-mcp -transport http -remote-config /run/nines/remote-mcp.yaml \
  -listen 0.0.0.0:8443 -tls-cert /run/tls/tls.crt -tls-key /run/tls/tls.key
```

- Plain HTTP listeners require a literal loopback IP. Remote public/API/issuer URLs require HTTPS; HTTP is permitted only for loopback fixture development.
- Preserve the exact public `Host` header at the proxy. Untrusted Host headers, forwarded-host routing and request-selected endpoints are not accepted. Forward both `/mcp` and the protected-resource metadata paths.
- `NINES_URL`, `NINES_TENANT`, `NINES_API_KEY` and legacy `API_TOKEN` do not select remote identities. Stdio routing/write flags cannot be mixed with HTTP mode; use the YAML.
- The remote listener is disabled by default and is not exposed automatically by the normal server or playground Compose stack.

## Discovery and scopes

Clients connect to `publicURL`. Unauthenticated requests receive `401` with a `WWW-Authenticate` challenge containing `resource_metadata` and the required `nines:read` scope.

Protected-resource metadata is served at both:

- `/.well-known/oauth-protected-resource/mcp` for the example endpoint;
- `/.well-known/oauth-protected-resource` as a discovery fallback.

The metadata advertises the configured issuer and minimal read scope. Clients discover the external issuer's RFC 8414/OIDC metadata and obtain resource-bound tokens there.

| OAuth scope | Additional operator opt-in | Effect |
| --- | --- | --- |
| `nines:read` | None | Read-only tool surface, including native provider results, discovery/import drafts and integration evidence |
| `nines:write` | `allowWrites: true` | Existing direct writes and proposal/preparation tools; server RBAC and revision checks still apply |
| `nines:deploy` | Both write and deployment flags | Reviewed rule/native publication tools; Nines owner permission and exact digest required |
| `nines:withdraw` | All three flags | Reviewed withdrawal, preserving provider-specific safeguards |

Additional scopes must be explicitly requested at your authorization server/client; they are not granted by a tool call or advertised as the default. Human approval/rollback review gates remain in the human CLI/API. Enabling writes does not expose agent approval tools.

## Lifecycle, cancellation and limits

- Supported and tested profile: MCP **2026-07-28** and **2025-11-25** through official Go SDK **v1.8.0**, stateless Streamable HTTP with JSON responses.
- Each POST is independently authenticated and gets its own MCP server/identity. No `Mcp-Session-Id` is issued. Session IDs and `Last-Event-ID` replay requests are rejected. GET/DELETE return `405`; OPTIONS supports approved browser origins.
- There are no persistent SSE subscriptions, resumable event stores, server-initiated requests or durable MCP tasks in this profile. Reconnect using a fresh HTTP request; older clients may repeat initialization. Interruption cancels upstream Nines API calls, including the legacy profile.
- A lost response does not prove a mutation failed. **Never blindly retry writes.** Read the current revision/receipt and review the same or a new plan before acting. OAuth authorization-code replay protection belongs to the authorization server; valid bearer tokens can be reused until expiration/revocation and must be protected from theft.
- Bounds: 1 MiB MCP body, 64 KiB introspection response, 16 concurrent HTTP operations, four per client/user binding, a global 20 requests/second burst 40, and five per binding burst 10. Request deadlines are at most 30 seconds and never exceed token expiration. The CLI listener additionally limits headers and connection timeouts.
- Token introspection happens on every request without caching. Revocation blocks subsequent requests; in-flight calls are bounded by cancellation/deadlines and Nines' own authorization checks. This is not instantaneous distributed cancellation of already committed writes.
- An unavailable/invalid introspection service returns `503`, without forcing a new login. Expired/inactive/wrong-audience grants return `401`; missing scope or unbound identity returns `403`. Saturation returns `429`.
- Browser Origins are denied by default except the canonical origin. Add exact HTTPS origins through `allowedOrigins` only when needed. No cookie authentication, wildcard CORS, query tokens or browser credential sharing is enabled.

## Threat model and operations

- Trust only your configured authorization server, introspection endpoint, administrative bindings and Nines server. Do not derive routes, credentials or tenant IDs from models, user arguments or forwarded headers.
- The adapter logs tenant, public client identifier, subject digest and HTTP outcome. It does not log bearer tokens, introspection bodies, tool arguments or Nines/provider credentials. Upstream errors remain sanitized.
- Nines mutation auditing records the distinct bound Nines service-account identity. Correlate it with the adapter's client/subject digest and your authorization-server audit trail. This is explicit delegation, not human impersonation.
- Use separate service-account credentials per binding; rotation/binding changes require an adapter restart. Revoke the OAuth grant at the issuer and the Nines credential independently when offboarding.
- Keep rate/concurrency limits at the proxy too, protect deployment secrets, and monitor authorization availability and denials. Introspection increases issuer traffic; there is deliberately no revocation cache.
- This alpha has local fixture acceptance, not a penetration test, HA/load certification or blanket compatibility claim for managed clients.

## Client conformance and reproducible checks

| Client/profile | Evidence |
| --- | --- |
| Official Go SDK v1.8.0, MCP 2026-07-28 | Credential-free OAuth discovery, authorization-code S256 PKCE/resource binding, token exchange, tools/list/call, distinct identities, revocation, reconnect and interruption tests |
| Legacy MCP 2025-11-25 HTTP profile | Raw protocol initialization, stateless tool calls, metadata, chunked body bounds and interrupted upstream cancellation tests |
| Existing stdio SDK client | Preserved executable stdio initialize/list/call regression |
| Hosted managed-agent products | Not exercised in this pass; validate the client's OAuth registration and HTTP profile before enabling |
| Other issuers/opaque tokens/JWTs | Accepted only through the documented introspection fields/profile; no universal JWT or IdP compatibility claim |

Run without any paid-platform credentials:

```sh
make test-providers
go test -race ./internal/mcpserver ./cmd/mcp
```

Tests also reject unauthenticated/expired/wrong-issuer/wrong-resource grants, unknown client/user bindings, invalid origins/hosts, stolen session/event identifiers, unbounded requests, replayed authorization codes, credential serialization/leaks and OAuth attempts to bypass actual Nines API RBAC. They use a synthetic local issuer, not your paid-provider accounts.

Standards: [MCP transport](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports), [MCP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization) and [RFC 7662 introspection](https://www.rfc-editor.org/rfc/rfc7662).
