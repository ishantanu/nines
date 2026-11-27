# Platform administration

The admin console is at `/admin/tenants`, where you [create and manage tenants](#tenants), with separate pages for [users](#users-and-tenant-access), [Jev policies](#jev-controls) and [audit history](#audit-history).

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
- Administrator assignments remain in server configuration. Tenants can be [created in the console](#tenants); tenants defined in `TENANTS_FILE` stay managed there.
  - The console cannot silently promote an ordinary identity to platform administrator, change the trust configuration or delete its own recovery administrator.
  - Keep a separately configured recovery identity and retain access to the authentication configuration.
- Sign in as a person: in the browser, or with `nines auth login` for the [command line](#from-the-command-line).
  - A CLI token works only while the browser session that approved it is valid, and only for a configured platform administrator.
  - API keys and service-account keys cannot access platform endpoints, even if their tenant role is `owner`.
  - Browser changes require the CSRF token; every change requires a nonempty reason. The audit history records whether a change came from the browser or the CLI.
  - Platform requests do not accept `X-Nines-Tenant`; their scope is explicit in the operation. The [API reference](api.md) documents every platform operation.

## Tenants

Create a tenant for a new team without editing the tenants file or restarting the server.

1. Open **Tenants** and select **Create tenant**.
2. Enter an ID (lowercase letters, digits and hyphens; it cannot change later) and a display name.
3. Choose the first owner: a new local account with an initial password, or someone who can already sign in through a configured provider.
4. Give a reason and create it.

- The tenant is ready at once: its owner can sign in, add telemetry connections, invite people and create SLOs. Every server process serves it within 10 seconds, including separate collector processes.
- Each tenant has its own database schema, rules, collection and background work. Nothing is shared with other tenants.
- **Rename** changes the display name. The ID stays the same.
- **Retire** stops collection and alerting and makes the tenant read-only. People keep read access and its data is kept.
- **Delete** is available once a tenant is retired. Type its ID to confirm. It permanently deletes the tenant's SLOs, measurements, history, members, teams and access keys, and its sessions and keys stop working at once. If deletion is interrupted, the tenant shows **Deletion incomplete**; select **Finish deleting** to resume.
- Tenants from server configuration are marked **Configured** and are changed there. The ID `default` is reserved for the server's base tenant.
- Every change needs a reason, is refused if someone changed the tenant first, and is recorded in [audit history](#audit-history).
- Creating tenants here needs the identity service and a PostgreSQL database. Without them, as in `-demo` or `-memory` mode, the console lists the configured tenants only.

### From the command line

Sign in once with `nines auth login`, then:

```sh
nines admin tenants list
printf '%s\n' "$INITIAL_PASSWORD" | nines admin tenants create payments --name Payments \
  --owner-username pat --owner-name "Pat Example" --owner-password-stdin --reason "Onboard the payments team"
nines admin tenants rename payments --name "Payments platform" --reason "Team renamed"
nines admin tenants retire payments --reason "Merged into billing"
nines admin tenants delete payments --confirm-id payments --reason "Merged into billing"
```

- To make someone who can already sign in the first owner, use `--owner-provider` and `--owner-subject` instead of the local account flags.
- A new owner's password is read from stdin, never from a flag, so it stays out of shell history and process lists.
- Changes use the tenant's current version unless you pass `--expected-version`; then they are refused if someone changed the tenant since that version.

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
