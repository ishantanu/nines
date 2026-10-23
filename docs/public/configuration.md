# Configuration

See the [deployment guides](deployment.md) for container, Linux, Kubernetes, identity and recovery setup.

## Demo and persistent server

`go run ./cmd/server -demo` binds to loopback by default. It uses synthetic in-memory data and disables external integrations. Its `admin` / `admin` UI sign-in does not protect an API deployment. Keep it local.

For a persistent single tenant, set `DATABASE_URL` and a random `API_TOKEN` of at least 24 characters. Compose reads `.env`; a native process reads only its inherited environment. Use a URL-safe PostgreSQL password in the Compose example because it is interpolated into a connection URL. Back up the volume before changing or upgrading data handling. The public MVP does not promise zero-downtime upgrades.

Set `PROMETHEUS_URL` and optionally `PROMETHEUS_BEARER_TOKEN`, or use `MIMIR_URL`, `MIMIR_TENANT_ID` and optional Mimir authentication. Do not configure both legacy primary provider modes. Sources and rule destinations are administrator-owned. Configure actual metrics and selectors explicitly; the platform does not invent instrumentation.

## Tenant boundaries

Use `TENANTS_FILE` or `-tenants-file` with `examples/tenants.yaml` for multiple workspaces. Give every tenant independent storage/source configuration. Clients select an authorized tenant with `X-Nines-Tenant`. The server fixes provider credentials and Mimir organization headers; resources and model output cannot override them. Up to 20 named connections plus a primary source are supported per tenant.

Owners administer tenant access and deployment. Editors can change definitions; viewers read; agent roles prepare bounded proposals within their allowed surface. Do not issue an owner token to an agent to bypass approval restrictions. Project/service labels do not grant access to another tenant.

## Identity and CLI login

Set `AUTH_CONFIG_FILE` or `-auth-file` using `examples/auth.yaml`. Use HTTPS for `publicURL` outside loopback. OIDC providers use discovery, signed ID tokens, state, nonce and PKCE. LDAP supports configured TLS/StartTLS trust. `examples/auth-entra.yaml` shows Microsoft Entra ID OIDC and explicit group/app-role mapping. Native SAML, SCIM and directory synchronization are not implemented; a SAML upstream needs an OIDC broker.

Authentication alone grants no tenant access. Direct membership, teams and current verified SSO evidence determine the role on each request. OIDC claim freshness limits bound stale upstream grants; this is not continuous access evaluation. Configure a recovery owner and keep provider IDs stable.

The CLI reads `NINES_API_KEY`, with `API_TOKEN` as an alias. Browser login issues a short-lived single-tenant Nines credential, not an identity-provider token. It reuses the configured provider and current roles. See [CLI authentication and storage](../cli.md).

## Optional outbound integrations

`ALERT_WEBHOOK_URL` enables at-least-once delivery of alert transitions. Deduplicate the stable event ID at the receiver. Configure standard `OTEL_*` variables to export server telemetry. Browser OTLP/PostHog settings are build-time `VITE_*` configuration and are empty by default. Do not put a private server credential in browser configuration.

Models are optional. Intent/review model calls require explicit workflow consent and trusted provider configuration. The first-release core does not require OpenAI, Anthropic, Gemini or a local model. Review destinations and data sharing before enabling any of them. The [intent evaluation guide](intent-evaluations.md) documents provider/key variables, local Ollama and vLLM endpoints, repeatable checks and measured model limitations.

## Local users

A configured `local` provider enables server-authenticated username/password sign-in alongside OIDC/LDAP and API credentials. See `examples/auth-local.yaml`. Bootstrap passwords come from environment variables, are hashed with bcrypt cost 12, and are seeded only when absent. Passwords require 12–72 bytes and are never returned by APIs. The loopback playground explicitly enables `allowDemoCredentials` for its `admin` / `admin` bootstrap account only; remote public URLs and other short passwords are rejected. Changing a password increments the session epoch, revoking existing browser and CLI sessions. Configure HTTPS outside loopback and retain a recovery owner.

Tenant owners use **Access control → Users** to create local identities and tenant membership, edit profiles/passwords, change roles and remove tenant access. External-provider passwords remain with that provider. Shared local profile/password edits require ownership of every tenant the user can access; last-owner protections and revision checks apply. Usernames are unique per local provider across the server. The API does not delete a shared global identity when removing one tenant membership.

## Managed data sources

Set `NINES_SOURCE_HOSTS` to a comma-separated list of exact approved hostnames to enable **Data sources → Add data source** for tenant owners. Set a private base64 32-byte `NINES_SOURCE_ENCRYPTION_KEY` to permit bearer credentials; keep it outside source control and back it up separately from the database. The encrypted tokens are never included in GET responses. Leaving a token blank on edit preserves it; clearing it is explicit. Managed sources are stored in the tenant's PostgreSQL schema and restored on startup.

Mimir organization IDs must already be authorized by the tenant's trusted YAML source configuration. Clients cannot select an arbitrary upstream organization. Managed Prometheus rule files use only `NINES_MANAGED_RULE_DIRECTORY`; callers cannot choose filesystem paths. Rule namespaces are server-assigned. Configure a shared mount, `rule_files` and a reload mechanism as described in the metrics guide. The playground includes that mechanism.

YAML-configured sources remain read-only through HTTP. Managed identity edits/deletions are refused while referenced by SLOs or measurement bindings. Credential rotation is allowed without changing dataset identity. Run one collecting server; this registry does not implement cross-replica configuration invalidation. Keep the host/org policy and encryption key available across restarts; changing them can prevent persisted connections from loading.

Changing or deleting a connection does not retire previously deployed external rule groups. Before moving an SLO to another connection, retire its old rules using the configured backend and verify they are no longer evaluating. Connection deletion is blocked while an SLO or measurement binding still references it.

## Automatic rule delivery

`rules.autoDeploy: true` is an owner opt-in per static or managed source. Direct Prometheus sources additionally require `rules.reload: true`, a writable/shared `rules.directory`, a matching Prometheus `rule_files` mount and lifecycle enablement. Mimir uses its pinned ruler URL/namespace/organization. Portal operator mode can publish automatically too. The managed source API accepts `autoDeploy`; the portal exposes the same setting. Existing non-playground sources default to manual. Read [automatic rules](deployment/automatic-rules.md) for retries, pause/resume, status and deployment limits.
