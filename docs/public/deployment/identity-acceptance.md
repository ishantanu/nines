# Identity acceptance

This guide separates controlled-provider evidence from verification of your own identity deployment. The full runner uses a loopback OIDC issuer with generated signing keys, real OpenLDAP with disposable certificates, a dedicated PostgreSQL database and the native Nines CLI. Go-only tests also use smaller protocol fixtures. They do not contact Microsoft or a corporate directory.

## Run the complete acceptance fixture

```sh
make test-identity
```

Requires Go 1.27.3, Docker, OpenSSL and Python 3. The command builds a disposable OpenLDAP image from a pinned official Debian image and Debian-maintained packages, starts it alongside PostgreSQL 17, and generates new credentials and a short-lived local CA. Ports bind only to random loopback addresses. No existing playground, directory, Nines context or API key is used. Containers, their storage, generated image, certificates, temporary CLI profiles and binaries are removed on completion or interruption.

The runner executes the existing rejection/RBAC tests with its dedicated database, then the upstream acceptance suite. It writes an ignored `identity-test-results.json` report and rejects failed, missing or skipped required cases. The CI **identity** job runs the same command.

| Scenario | Evidence |
| --- | --- |
| Encrypted directory | Real OpenLDAP LDAPS and StartTLS; untrusted CA rejected after a successful trusted control |
| Stable directory identity | Renaming a user's DN/login retains its `entryUUID`, Nines user ID and explicit tenant grant |
| Directory failures | Invalid passwords, escaped search input and reader-bind failure cannot create a new session |
| Native CLI | Actual `nines login --no-browser`, private 0600 credentials, tenant-bound `auth status`, cancellation without saved credentials |
| Durable identity | Browser and CLI access survive closing and reopening the PostgreSQL identity repository |
| Live permissions | Owner/editor/viewer restrictions, exact claims, group-overage denial, team removal, direct membership removal and identity-provider client changes |
| Revocation | Fresh verified claims update old sessions; claim expiry and explicit logout-all deny existing browser/CLI access |
| Consent | Exact server, tenant, identity, current role and return address appear before approval; CSRF, callback changes and consumed requests are rejected |

The browser steps use HTTP clients with cookie jars through the production authentication and tenant handlers. This exercises redirects, signed token validation, cookies, CSRF and consent; it does not certify desktop browser rendering or Microsoft conditional access. The OIDC issuer supplies Entra-shaped signed claims. OpenLDAP checks run against the Debian-packaged upstream server. Microsoft tenant verification uses the procedure below. No provider token is handed to the CLI.

An upstream outage or directory credential change blocks new authentication. Nines does not poll the directory on every existing-session request. The fixture verifies this distinction and then proves that Nines logout-all revokes the existing CLI credential even while directory authentication is failing. Use application-side revocation for immediate removal of existing access.

## Run the controlled-provider checks

```sh
go test -race ./internal/identity ./internal/httpapi ./cmd/nines
```

For persistent session and membership coverage, supply a dedicated disposable PostgreSQL database through `TEST_DATABASE_URL`. Never point the suite at a live identity database. Tests use isolated schemas and remove their own data.

| Boundary | Controlled-provider coverage |
| --- | --- |
| OIDC authentication | Signed tokens, issuer, audience, state, nonce, expiry, callback replay and PKCE |
| Entra-shaped authorization claims | Exact app roles/groups, highest allowed role, malformed claims and group-overage denial |
| Tenant RBAC | Owner/editor/viewer behavior, two-tenant isolation, CSRF, independent direct/team membership |
| Browser and CLI | Explicit approval page with server, tenant, current role and callback; one-time PKCE exchange; no grant on cancellation |
| Live authorization | New verified claims replace old claims for existing browser and CLI sessions; role loss cannot reuse an old CLI grant |
| Provider changes | Removed providers and changed issuer/client identity reject old sessions, including directly granted owners |
| Directory authentication | LDAPS/StartTLS, CA rejection, escaped filters, stable subjects and binary AD GUIDs |
| Persistence | Session/provider binding and tenant grants survive repository reopen; a changed OIDC client rejects persisted sessions |

A session is authentication evidence, not a permanently cached role. Role mappings are evaluated from the current server policy and the last verified claims snapshot. Direct grants and team roles are separate from SSO-derived grants. Removing an Entra role does not remove an independently assigned direct membership.

## Optional real-Entra acceptance

Use a test Entra application and disposable Nines tenants. Configure the [Entra example](../../../examples/auth-entra.yaml) using private environment variables. Keep client secrets, tokens, directory identifiers and user data out of reports.

1. Configure the exact issuer, client ID and Web callback. Assign separate test identities owner, editor, viewer and no assignment. Prefer app-role values for a small repeatable fixture; use exact group Object IDs when testing group mappings.
2. Sign in to each tenant and verify the resulting role through the portal. Check that viewers cannot write, editors cannot administer users or deploy rules manually, and an unrelated identity cannot read tenant resources.
3. Run `nines login --context TEST_CONTEXT --method sso`. Verify the displayed server, tenant, role and loopback callback before approving. Test cancellation with a fresh request and confirm that no CLI credential is created.
4. Remove or downgrade an assignment. Complete a fresh sign-in and verify both existing browser and CLI sessions use the new claims. Separately allow `claimsMaxAgeMinutes` to expire and confirm a new sign-in is required. No background Graph lookup or continuous access evaluation is implied.
5. Remove direct/team grants separately where present. Test logout, logout-all, session expiry and provider configuration changes. Restart the collecting server against the same test database and repeat the permission checks.
6. Record the Nines source commit, sanitized provider configuration shape, test case/result, relevant expiry bounds and whether a browser or CLI was used. Do not include token payloads, session cookies, passwords or private directory attributes.

Disabling an upstream account is enforced at the next provider authentication; Nines does not poll Entra or LDAP for account state on every request. Existing access remains bounded by the Nines session and, for mapped SSO grants, verified-claim expiry. Use Nines session revocation and remove local grants when immediate application-side denial is required.

## Remaining acceptance

Real Entra deployment and organization-specific directory policy require an administrator-run acceptance exercise. These fixtures do not certify conditional access, group-claim configuration, provider logout, native SAML/SCIM, every directory implementation or browser interoperability. Native SAML remains supported only through an OIDC broker. The controlled suite and environment-specific acceptance are separate evidence. The latter requires your administrator and should record the deployed source commit; fixture success does not certify a production Entra tenant or directory policy.
