# Identity and tenant setup

Nines separates identity from authorization. Signing in identifies a person; a tenant membership, team role or current verified SSO claim grants access to a workspace. Application tenant IDs are separate from projects, services and Kubernetes namespaces.

## API-key tenants

Start with a server-owned YAML file. Credentials are environment references, never literal keys in the file:

```yaml
tenants:
  - id: commerce
    name: Commerce
    access:
      - tokenEnv: COMMERCE_OWNER_TOKEN
        role: owner
      - tokenEnv: COMMERCE_VIEWER_TOKEN
        role: viewer
    source:
      kind: manual
  - id: analytics
    name: Analytics
    access:
      - tokenEnv: ANALYTICS_OWNER_TOKEN
        role: owner
    source:
      kind: manual
```

Set each referenced environment variable to an independently generated random value of at least 24 characters and set `TENANTS_FILE` to the file path. Nines uses independent tenant stores and source maps; PostgreSQL tenant schemas are selected by trusted startup configuration. A token grants access only to the tenants that explicitly name it. Sharing a credential across tenants intentionally expands that credential's access.

`owner` administers a tenant, `editor` edits resources, `viewer` reads, and `agent` works within the bounded proposal/tool surface. Use service-account keys or narrow tenant keys for automation. Projects and resource labels do not add another access-control boundary.

Native processes inherit the environment. Containers need the file mounted read-only and every referenced secret explicitly passed into their environment. YAML file changes require a server restart; they are not discovered from arbitrary request bodies.

## Local accounts

The persistent Compose deployment and Kubernetes reference enable a local provider. `admin` is seeded with `NINES_BOOTSTRAP_PASSWORD` on the first start; its hash and memberships persist in PostgreSQL. Owners use **Access control → Users** for profiles, password resets, roles and tenant access. A reset revokes existing sessions. Changing the environment variable does not overwrite an existing password.

For native execution, mount or copy [the local account configuration](../../../examples/auth-local.yaml), set `AUTH_CONFIG_FILE` to it, and supply `NINES_PUBLIC_URL` and `NINES_BOOTSTRAP_PASSWORD` through the process environment. Use HTTPS outside loopback. API-token credentials can coexist with account login.

## Microsoft Entra ID / OIDC

The [Entra example](../../../examples/auth-entra.yaml) configures OIDC SSO. Register a workforce web application in your Entra tenant, then set:

- A Web redirect URI such as `https://slo.example.com/auth/oidc/microsoft/callback`. The provider ID `microsoft` is part of the exact callback path.
- The issuer `https://login.microsoftonline.com/YOUR_DIRECTORY_ID/v2.0` and the application's client ID.
- A protected client secret in `NINES_ENTRA_CLIENT_SECRET`.
- `publicURL` to the actual external HTTPS origin and `AUTH_CONFIG_FILE` to the mounted auth YAML.

Configure exact app-role values or group Object IDs in `roleMappings`. For example, map a signed `roles` value `Reliability.Admin` to `owner` for the `commerce` tenant, and assign that app role to the intended administrator in Entra. The mapping's tenant must exist in the tenant configuration. Group claims must be present in the verified token; group-overage markers do not establish membership and are not expanded through Microsoft Graph.

If using a bootstrap entry, use the exact application-specific OIDC `sub`, not an email, UPN or directory Object ID. Obtain it through an approved identity-administration process and keep a recovery owner. Bootstrap is one-time provisioning per tenant/identity database, not ongoing directory synchronization. Do not leave placeholder bootstrap subjects in a shared deployment.

Keep provider IDs stable. Signed claims, audience, state, nonce and PKCE are checked. `claimsMaxAgeMinutes` bounds the age of SSO-derived grants; it is not continuous access evaluation. After changing mappings, test a fresh sign-in and verify the role in each intended tenant. Verify that an unrelated user cannot enter those tenants.

## LDAP and SAML upstreams

The [generic auth example](../../../examples/auth.yaml) includes an LDAP provider. Use LDAPS or StartTLS, a trusted CA, a restricted directory bind account and a stable subject attribute. Plain LDAP binds and certificate-verification bypasses are unsupported. Map the verified subject to tenant membership using bootstrap or tenant administration. LDAP sign-in alone does not grant a role.

Native SAML and SCIM are not implemented. Use an administrator-managed identity broker that presents OIDC to Nines if the upstream organization uses SAML. The same tenant authorization rules still apply.

## CLI login

```sh
nines config set-context commerce --server https://slo.example.com --tenant commerce
nines login --context commerce --method sso
nines get slos --context commerce --agent
```

The browser flow uses the server's configured providers and issues a short-lived, single-tenant Nines credential. It does not hand the identity-provider token to the CLI. Headless automation should inject `NINES_API_KEY` through its secret manager. See the [CLI guide](../../cli.md) for storage, precedence and logout.

Verify recovery access, viewer restrictions, cross-tenant denial and credential revocation before onboarding a team. Real Entra/LDAP configuration requires an environment-specific acceptance test; synthetic provider tests are not a substitute.
