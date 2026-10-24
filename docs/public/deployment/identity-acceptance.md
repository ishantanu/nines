# Identity acceptance

This guide separates controlled-provider evidence from verification of your own identity deployment. Fixtures use loopback OIDC issuers with generated signing keys and encrypted LDAP servers with disposable certificates. They do not contact Microsoft or a corporate directory.

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

Real Entra deployment and organization-specific directory policy require an administrator-run acceptance exercise. These fixtures do not certify conditional access, group-claim configuration, provider logout, native SAML/SCIM, every directory implementation or browser interoperability. Native SAML remains supported only through an OIDC broker. Issue #22 stays open until its remaining acceptance evidence is recorded.
