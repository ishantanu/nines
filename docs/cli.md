# Nines CLI

`nines` is a single Go executable with native [Cobra](https://github.com/spf13/cobra) commands, generated help and shell completion. `cobra-cli` is Cobra's optional contributor scaffolding generator; users need neither generator nor Go after the binary is built. Run `make cli`, `bin/nines --help`, or `bin/nines intent conversations approve --help`. Existing single-dash long flags remain accepted; new examples use normal double-dash flags. Flags may precede or follow positional arguments. Unknown or irrelevant leaf flags fail instead of being ignored.

## Without a Nines server

These commands use the same validation and rule compiler as the server. They do not contact a server or load a credential store:

```sh
nines init slo --name checkout --window 28d --step 1m --out checkout.yaml
nines validate --offline -f checkout.yaml --agent
nines rules generate --offline -f checkout.yaml --tenant commerce --out rules.yaml
nines operator crds --offline > crds.yaml
nines operator render --offline -f checkout.yaml --namespace team-commerce > slo.kubernetes.yaml
nines capabilities --offline --agent
nines skills show nines-cli --offline > SKILL.md
```

`-f -` reads stdin; file inputs retain their command-specific size limits. Generated files use exclusive creation and do not overwrite an existing file. Offline `validate` supports SLOs, critical user journeys, measurement bindings, dependency maps and intent YAML. SLO override flags apply only to SLO definitions; intent validation does not select or confirm a measurement. Offline validation proves declared schema/numerical validity. It cannot prove metric presence, provider evaluation, business semantics or a CUJ's end-to-end success. Offline rule generation renders YAML without deploying it. For `operator render --rules-only`, supply an explicit `--tenant` or `NINES_TENANT`; Kubernetes namespace and Nines tenant are separate identities.

`nines integration SLO_ID --agent` reads separate definition, collection, rule deployment and live external evaluation states. It does not deploy or reload anything. Provider errors and unavailable runtime APIs remain unverified.

Connected planning, telemetry discovery, backtests, collection, intent conversations, rule deployment and live status require Nines. `--offline` rejects those commands before credential loading or requests. There is no implicit in-process server, local database, provider discovery or LLM invocation.

## Tenant contexts and API keys

```sh
nines config set-context production \
  --server https://nines.example.com --tenant commerce
nines config use-context production
# Inject NINES_API_KEY through your shell or CI secret manager.
nines login --method api-key --context production
nines auth status --context production --agent
nines get slos --context production -o table
nines get slos --context production --agent
```

`NINES_API_KEY` is preferred, with `API_TOKEN` retained as an alias. If both exist, `NINES_API_KEY` wins. Keys are read at invocation time, never accepted as command-line values, printed in status, or written by login. Existing server API grants and managed service-account keys both work. CI and deterministic skills should normally use an appropriately scoped service-account key, including an agent/viewer role where sufficient. Environment keys are explicitly usable with a different server/tenant selector; the server still enforces their grants. Never point a credential-bearing invocation at an untrusted server.

For server and tenant, explicit flags override `NINES_URL`/`NINES_TENANT`, then the selected context, then the existing loopback/default fallback. `--context` overrides `NINES_CONTEXT`, then `currentContext`. Configuration path precedence is `--config`, `NINES_CONFIG`, then the OS user configuration directory's `nines/config.yaml` (on macOS, `~/Library/Application Support/nines/config.yaml`). Config is YAML with `apiVersion: nines.dev/v1alpha1`, `kind: CLIConfig`, and named `{server, tenant}` contexts. `config get-contexts` reads no credentials.

Saved browser credentials are considered only if no environment key is present, and only for an exact saved server and tenant match. Changing either selector never forwards a saved browser credential to the new destination. Expired credentials require explicit login. No command silently starts authentication or refreshes a session.

## Browser SSO and LDAP login

```sh
nines login --method sso --context production --timeout 5m
# Print a URL instead of opening the default browser:
nines login --method sso --context production --no-browser --timeout 5m
nines auth status --context production
nines auth logout --context production
```

The server must have identity authentication enabled. Unset API-key environment variables before explicitly selecting SSO. `--method auto` (the default) selects an environment API key when present and browser login otherwise.

The browser opens Nines' CLI authorization page, where the user signs in through the configured OIDC provider or LDAP directory, sees the exact tenant, current role, server and loopback return address, then explicitly authorizes the CLI. This reuses the existing provider and role mappings. Microsoft Entra ID uses OIDC. SAML-only upstream providers require an OIDC identity broker; **native SAML is not implemented**. No provider client secret, IdP access token, ID token, password or browser cookie is copied to the CLI.

The Nines authorization-code exchange uses a random loopback port, state and S256 PKCE. Requests expire in five minutes and approved codes in one minute; codes are consumed atomically once. A resulting opaque credential is restricted to the selected tenant and expires within one hour or the browser session's earlier deadline. Every API request resolves current membership/team/SSO grants and the original session epoch/expiry. Role reductions and browser sign-out revoke access. Upstream IdP changes retain the existing configured claim-freshness limit; this adds no continuous-access or refresh-token promise.

A separate `.credentials.json` file beside config stores only the Nines credential and its server/tenant/expiry binding. The directory requires mode 0700, files mode 0600; writes use a lock, a temporary file and atomic rename. Existing unsafe or symlink file paths are refused. This is private-file storage, not OS-keychain encryption. Coordinate concurrent invocations through the supplied lock; a crash can leave a `.lock` file that must be inspected and removed after verifying no writer remains.

`auth logout` revokes the saved credential and removes it locally, without signing out the browser or revoking service-account/environment keys. An unavailable server leaves the credential in place so revocation can be retried. `auth logout --local` deliberately removes only the local copy; the server copy expires normally. Environment API keys remain active until unset or revoked through the server's administrative flow.

`--no-browser` still requires opening the URL on the computer running the CLI because the return address is loopback. For remote headless/CI sessions, use an API key. Device-code login, automatic refresh and OS-keychain storage are not implemented.

## Scripts and LLM skills

```sh
nines capabilities --agent > capabilities.json
nines skills show nines-cli --offline > SKILL.md
nines search --context production --kind slo --query checkout --agent
nines plan --context production -f reviewed.yaml --agent > plan.json
nines apply --context production -f reviewed.yaml --expected-revision 0 --agent
```

`--agent` selects JSON for data and errors, prohibits browser login, and never prompts. It does not infer permission to mutate resources or disable model calls on commands explicitly marked as such. The offline capability catalog exposes modes, possible effects, flags, defaults and artifact formats; it grants no authorization. It does not evaluate a skill or invoke an LLM. Exact revisions, intent versions, measurement confirmation and reviewed rule digests remain mandatory. Server role restrictions remain authoritative.

Successful data goes to stdout; login instructions and errors go to stderr. Main buffers output until successful completion. `--output json|yaml` gives structured data with canonical API field names. List commands default to tables for humans; operational results default to JSON. Manifest, Kubernetes and rule exports always emit YAML, including in agent mode; explicit incompatible output flags fail. With `--out`, the artifact goes to the exclusively created file; human mode prints its path and agent mode emits a JSON `CLIArtifact` receipt with path, size and SHA-256. `skills show` defaults to Markdown (JSON with `--agent` or explicit structured output). `history -o csv` preserves its existing time-series CSV contract. No ANSI styling is emitted. Unknown values in human tables read `unknown`.

| Exit | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Local execution or validation failure |
| 2 | Invalid command/flag/argument usage |
| 3 | Missing, invalid or expired authentication |
| 4 | Tenant or operation forbidden |
| 5 | Revision/digest/configuration conflict |
| 6 | API unavailable, timeout or cancellation |

Agent errors use `{apiVersion, kind: CLIError, error: {code, message, exitCode}}`. API response bodies and credentials are excluded from errors. The CLI blocks redirects and requires HTTPS, except loopback HTTP for development. `--timeout` bounds the command; provider/SDK operations may impose shorter limits. No mutation is automatically retried. After an ambiguous timeout, read back current revisions or the intent apply receipt before deciding whether to retry the exact reviewed request.

Deterministic here means typed inputs, the shared compiler, stable ordering, explicit side-effect classes and revision/digest checks. It does not mean byte-reproducible live telemetry, timestamps or optional model output. Native resource types retain the shared `nines.dev/v1alpha1` YAML contract.

Generate shell completion with `nines completion bash|zsh|fish|powershell`. Install the generated script in the shell's completion directory according to that shell's instructions; running generation does not modify shell configuration.

See [testing and verification limits](public/testing.md) for required clean-checkout and external-provider checks.

For the controller image, CRD installation, provider prerequisites and ownership boundaries, see the [operator guide](public/deployment/operator.md). Offline rendering does not install a controller or synchronize resources into the portal.
