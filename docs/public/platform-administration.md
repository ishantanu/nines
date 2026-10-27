# Platform administration

The admin console is at `/admin/tenants`, with separate pages for [users](#users-and-tenant-access), [Jev policies](#jev-controls) and [audit history](#audit-history).

- Platform administrators manage deployment-wide identity and configuration controls. Tenant owners administer their own workspaces. Neither role implies the other: a platform administrator without a tenant membership cannot read or change that tenant’s SLOs through tenant APIs.

## Open the console

- Sign in through the existing `/login` page using an explicitly configured platform administrator account.
  - Select **Administration** in the workspace header or **Platform administration** in the sidebar.
  - Administrators with no tenant memberships land directly in the console.

The Compose playground explicitly grants platform administration to its local bootstrap identity. After rebuilding the server:

```sh
docker compose -p nines-playground -f compose.playground.yaml up --build -d
```

- Open `http://localhost:18080/admin/tenants`.
  - A fresh playground uses `admin` / `admin`, unless `NINES_PLAYGROUND_PASSWORD` was set before the first start.
  - Existing passwords persist; rebuilding does not reset them.
- The synthetic `-demo` mode has no real identity service and does not grant platform administration. Use the Compose playground or configure the local, OIDC or LDAP identity service.

## Configure platform administrators

In your administrator-owned authentication YAML, add exact provider/subject identities:

```yaml
platformAdmins:
  - provider: local
    subject: admin
```

- This supplements the provider configuration and optional tenant `bootstrap` memberships.
  - The [local account example](../../examples/auth-local.yaml) and Kubernetes reference include it explicitly.
  - Platform administration is not inferred from the username `admin`, an email, tenant ownership, a team, or an SSO tenant-role mapping.
- For Microsoft Entra ID/OIDC, use the configured provider ID and the application-specific verified OIDC `sub`.
  - For LDAP, use the configured stable subject attribute.
  - Platform privileges are checked against the current server-owned allowlist on each request.
  - Changing a provider’s identity binding invalidates the old session.
- Administrator assignments and tenant provisioning remain in server configuration.
  - The console cannot silently promote an ordinary identity to platform administrator, change the trust configuration or delete its own recovery administrator.
  - Keep a separately configured recovery identity and retain access to the authentication configuration.
- Browser sessions are required.
  - Tenant API keys, service-account keys and tenant-scoped CLI credentials cannot access platform endpoints, even if their tenant role is `owner`.
  - Mutations require the existing browser CSRF token and a nonempty reason.
  - Platform requests do not accept `X-Nines-Tenant`; their scope is explicit in the operation.

## Users and tenant access

- The **Users** page lists identities across configured tenants, their effective grants and whether they are explicitly configured platform administrators. The directory is paginated.
- Administrators can:

* Create a local account with an initial password and an explicit tenant role.
* Edit a local account’s display name and email, or reset its password.
* Assign or change a direct tenant membership using a revision check.
* Remove a direct tenant membership, retaining last-owner protection.
* Revoke all current browser and derived CLI user sessions through the account’s session epoch.
* Passwords are hashed using the existing local-account policy. Password changes revoke existing user sessions.
  - Passwords, password hashes and session tokens are excluded from responses and audit records.
  - Password fields are cleared from component state when submitted or dismissed, and the admin console is excluded from replay capture.
* SSO credentials remain with the identity provider.
  - Removing a direct membership does not remove an upstream SSO entitlement; change the corresponding provider assignment or configured mapping.
  - Direct membership removal also removes the user from that tenant’s Nines teams.
  - Service-account credentials have a separate lifecycle and are not revoked by user-session revocation.
* The console preserves optimistic concurrency and the rule that an existing last tenant owner cannot be removed or demoted.
  - A configured platform administrator can assign the initial owner to an otherwise unowned configured tenant.
  - The platform role itself grants no implicit tenant data access.

## Jev controls

- On **Tenants**, select **Jev policy** for the required tenant. The console shows the server-configured pinned route and current UTC-day request reservations.
- The policy supports enabling or pausing future Jev requests and setting a daily request limit of 1–100,000.
  - With a configured identity service, an unchanged policy defaults to 1,000 requests per tenant per UTC day.
  - Policy and counter records live in the identity database; native `-memory` runs lose them at restart.
- A request reserves quota transactionally before reaching TypeSafe. Parallel workers share the same counter.
  - Failed provider attempts consume their reserved slot; quota resets on the next UTC date.
  - Paused, exhausted or unavailable policy storage prevents the provider call.
  - Pausing does not cancel requests already reserved or in flight; cancel their workflow runs when necessary.
  - The intent workflow records the assessment as unavailable and continues its deterministic preparation path.
- A policy does not configure a provider credential or select an arbitrary endpoint/model.
  - Those remain server-owned references and pinned routes in the tenant configuration.
  - A route must be configured before it can be enabled.
  - See [System One advice](system-one.md) for `JEV_API_KEY`/`TYPESAFE_API_KEY`, tenant opt-in, separate per-run consent and measured model limitations.
- These are request limits for Jev assessments.
  - They are not token or currency budgets and do not manage generative drafting/review-model usage.
  - Per-run model budgets, measurement confirmation, SLO approval and recording-rule permissions remain independent.

## Audit history

- The **Audit history** page records platform sign-in/sign-out, account changes, membership changes, user-session revocation and Jev-policy changes. Each administrative mutation requires a reason. Successful changes and their audit entry commit in the same identity transaction; an audit-storage failure rolls back the change.
- Records include a random UUID, time, actor ID/name/provider, operation, affected identity/tenant, result and safe change details.
  - Jev-policy records include the previous and resulting policy.
  - Validated operations that fail, including revision conflicts and last-owner protection, are recorded separately.
  - Invalid authentication and CSRF requests are rejected before administrative operations; use server request/security logs for those rejections.
- Platform audit records are retained for 90 days and paginated by a server-generated cursor.
  - They are separate from the tenant SLO recommendation audit journal.
  - Optional S3 Object Lock archival currently applies to that tenant journal, not this platform audit store.
  - Back up the identity database for platform audit recovery.

## API surface

- These endpoints use authenticated platform browser sessions and same-origin CSRF protection:

| Method | Endpoint                              | Purpose                                                            |
| ------ | ------------------------------------- | ------------------------------------------------------------------ |
| GET    | `/api/platform/overview`              | Tenant inventory, identity providers and configured administrators |
| GET    | `/api/platform/users?before=CURSOR`   | Paginated global identity directory                                |
| GET    | `/api/platform/audit?before=CURSOR`   | Paginated platform audit records                                   |
| GET    | `/api/platform/tenants/ID/system-one` | Pinned Jev route, policy and UTC-day usage                         |
| POST   | `/api/platform/users/create`          | Create a local account and tenant grant                            |
| POST   | `/api/platform/users/update`          | Edit a local profile or reset its password                         |
| POST   | `/api/platform/users/revoke-sessions` | Revoke current user sessions                                       |
| POST   | `/api/platform/memberships/save`      | Create or update a direct tenant grant                             |
| POST   | `/api/platform/memberships/remove`    | Remove a revision-matched direct grant                             |
| POST   | `/api/platform/system-one/save`       | Save a revision-matched tenant Jev policy                          |

- Platform administration is intentionally unavailable through autonomous MCP mutation tools or ordinary tenant API credentials.
