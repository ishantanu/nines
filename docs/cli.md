# Nines CLI

`nines` is a single Go executable with native [Cobra](https://github.com/spf13/cobra) commands, generated help and shell completion. `cobra-cli` is Cobra's optional contributor scaffolding generator; users need neither generator nor Go after the binary is built. Run `make cli`, `bin/nines --help`, or `bin/nines intent conversations approve --help`.

- Existing single-dash long flags remain accepted; new examples use normal double-dash flags.
- Flags may precede or follow positional arguments.
- Unknown or irrelevant leaf flags fail instead of being ignored.

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

- `-f -` reads stdin; file inputs retain their command-specific size limits.
  - Generated files use exclusive creation and do not overwrite an existing file.
  - Offline `validate` supports SLOs, critical user journeys, measurement bindings, dependency maps and intent YAML.
  - SLO override flags apply only to SLO definitions; intent validation does not select or confirm a measurement. Offline validation proves declared schema/numerical validity.
  - For query-backed SLOs it also parses PromQL, requires instant-vector results and checks the generated recording and alert expressions.
  - It cannot prove metric presence, result cardinality, provider evaluation, business semantics or a CUJ's end-to-end success.
  - Offline rule generation renders YAML without deploying it.
  - For `operator render --rules-only`, supply an explicit `--tenant` or `NINES_TENANT`; Kubernetes namespace and Nines tenant are separate identities.
- `nines integration SLO_ID --agent` reads separate definition, collection, rule deployment and live external evaluation states.
  - It does not deploy or reload anything.
  - Provider errors and unavailable runtime APIs remain unverified. `rules apply` returns configuration persistence plus separate `activation` and `evaluation` stages.
  - A successful HTTP response or `state: written` does not establish activation: inspect these fields, including failed reload, manual reload, pending and not-loaded states.
  - Standalone Prometheus can opt into `rules.reload: true` on its trusted connection; this changes the reviewed digest. See [automatic backend activation](public/deployment/metrics-and-rules.md#automatic-backend-activation).
- Connected planning, telemetry discovery, backtests, collection, intent conversations, rule deployment and live status require Nines. `--offline` rejects those commands before credential loading or requests. There is no implicit in-process server, local database, provider discovery or LLM invocation.

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

- `NINES_API_KEY` is preferred, with `API_TOKEN` retained as an alias. If both exist, `NINES_API_KEY` wins.
  - Keys are read at invocation time, never accepted as command-line values, printed in status, or written by login.
  - Existing server API grants and managed service-account keys both work.
  - CI and deterministic skills should normally use an appropriately scoped service-account key, including an agent/viewer role where sufficient.
  - Environment keys are explicitly usable with a different server/tenant selector; the server still enforces their grants.
  - Never point a credential-bearing invocation at an untrusted server.
- For server and tenant, explicit flags override `NINES_URL`/`NINES_TENANT`, then the selected context, then the existing loopback/default fallback. `--context` overrides `NINES_CONTEXT`, then `currentContext`.
  - Configuration path precedence is `--config`, `NINES_CONFIG`, then the OS user configuration directory's `nines/config.yaml` (on macOS, `~/Library/Application Support/nines/config.yaml`).
  - Config is YAML with `apiVersion: nines.dev/v1alpha1`, `kind: CLIConfig`, and named `{server, tenant}` contexts. `config get-contexts` reads no credentials.
- Saved browser credentials are considered only if no environment key is present, and only for an exact saved server and tenant match.
  - Changing either selector never forwards a saved browser credential to the new destination. Expired credentials require explicit login.
  - No command silently starts authentication or refreshes a session.

## Browser SSO and LDAP login

```sh
nines login --method sso --context production --timeout 5m
# Print a URL instead of opening the default browser:
nines login --method sso --context production --no-browser --timeout 5m
nines auth status --context production
nines auth logout --context production
```

- The server must have identity authentication enabled. Unset API-key environment variables before explicitly selecting SSO. `--method auto` (the default) selects an environment API key when present and browser login otherwise.
- The browser opens Nines' CLI authorization page, where the user signs in through the configured OIDC provider or LDAP directory, sees the exact tenant, current role, server and loopback return address, then explicitly authorizes the CLI.
  - This reuses the existing provider and role mappings. Microsoft Entra ID uses OIDC.
  - SAML-only upstream providers require an OIDC identity broker; **native SAML is not implemented**.
  - No provider client secret, IdP access token, ID token, password or browser cookie is copied to the CLI.
  - The credential follows its original browser session: logout, session expiry or a changed/removed authenticating provider invalidates it.
  - Changes to current tenant roles are applied on each request.
  - Secret rotation alone does not invalidate the session.
- The Nines authorization-code exchange uses a random loopback port, state and S256 PKCE.
  - Requests expire in five minutes and approved codes in one minute; codes are consumed atomically once.
  - A resulting opaque credential is restricted to the selected tenant and expires within one hour or the browser session's earlier deadline.
  - Every API request resolves current membership/team/SSO grants and the original session epoch/expiry.
  - Role reductions and browser sign-out revoke access.
  - Upstream IdP changes retain the existing configured claim-freshness limit; this adds no continuous-access or refresh-token promise.
- A separate `.credentials.json` file beside config stores only the Nines credential and its server/tenant/expiry binding.
  - The directory requires mode 0700, files mode 0600; writes use a lock, a temporary file and atomic rename.
  - Existing unsafe or symlink file paths are refused.
  - This is the default private-file fallback. Opt-in OS storage is described below.
  - Coordinate concurrent invocations through the supplied lock; a crash can leave a `.lock` file that must be inspected and removed after verifying no writer remains.
- `auth logout` revokes the saved credential and removes it locally, without signing out the browser or revoking service-account/environment keys.
  - An unavailable server leaves the credential in place so revocation can be retried. `auth logout --local` deliberately removes only the local copy; the server copy expires normally.
  - Environment API keys remain active until unset or revoked through the server's administrative flow.
- `--no-browser` still requires opening the URL on the computer running the CLI because the return address is loopback. Use device login below when the terminal and browser are on different computers. CI and deterministic agent skills should continue to use an API key.

## Device login and credential storage

```sh
# Discover support without starting login or unlocking an OS credential store.
nines auth diagnose --server https://nines.example.com --tenant production --agent

# Open the printed URL on another trusted computer, sign in, enter the displayed
# code and explicitly approve the exact tenant. No local callback listener opens.
nines login --method device --context production \
  --server https://nines.example.com --tenant production --timeout 5m

# Explicitly opt into the operating system's credential store.
nines login --method device --context production \
  --server https://nines.example.com --tenant production \
  --credential-store keychain --timeout 5m

nines auth status --context production
nines auth logout --context production
```

- Device login is mediated by **Nines**, using the server's existing OIDC, LDAP or local identity provider and current tenant role mappings. It does not require the upstream provider to implement OAuth device authorization, nor does it copy provider access/refresh tokens to the CLI. SAML-only identity still needs the existing OIDC-broker configuration.
- This is the versioned Nines JSON authentication protocol, inspired by device authorization semantics; it is not an RFC 8628 OAuth endpoint. Generic OAuth device clients must not point at these paths.
- The device request expires after five minutes. The terminal retains a secret device code and random verifier; the printed browser URL contains a separate approval ticket, never that secret. Enter the human code shown in your own terminal. Never approve a device flow initiated by someone else.
- Polling begins at the advertised interval, normally five seconds. Early polling increases the interval, up to 30 seconds. Concurrent exchanges consume the approved grant once. Cancellation makes a best-effort request to remove the pending grant; an unreachable server still expires it after five minutes.
- The resulting credential is limited to the selected tenant and approving identity, with at most one hour of validity or the parent session's earlier expiry. Browser sign-out, account/session revocation, current role changes and provider binding checks apply just as they do to loopback login.
- **Automatic refresh is not supported.** Discovery reports `refresh: false`. Expired sessions fail with exit code 3; explicitly run login again. Long-running CI and skills should use independently scoped, managed service-account keys in `NINES_API_KEY`.
- Keychain storage is opt-in for human-operated CLI commands: macOS Keychain, Windows Credential Manager, or Linux Secret Service with a working session D-Bus and keyring. Native OS access can require an OS unlock/permission dialog; headless installations may not have an available credential service.
- `--credential-store keychain` stores the Nines credential in the OS service `nines.cli.v1`. The private metadata file retains its server, tenant, identity, expiry and opaque lookup reference, with **no token**. Lookup is bound to the configuration location, server, tenant and identity. Changing server or tenant never sends a saved credential to the new route.
- If OS storage is unavailable, login fails explicitly; it does not silently write the token to a file. Choose `--credential-store file` explicitly for the existing 0700-directory/0600-file fallback, or use `NINES_API_KEY`. Keep user configuration outside source repositories and shared directories.
- `--agent` never initiates device/browser login and never reads, writes or unlocks OS credentials. An environment key takes precedence without touching the OS store. Pre-authenticated file contexts continue to work until expiry. `auth diagnose --agent` discovers capabilities without loading a session or printing a credential.
- Login compares the selected context and credential snapshot before saving. A competing login or context change fails with conflict exit code 5; newly issued but unsaved credentials are revoked on a best-effort basis. Explicit logout revokes the selected saved session and removes its local entry; `--local` only removes the local copy. If replacing a keychain entry cannot clean up the previous OS entry, login reports a warning; the old server credential still has its original bounded expiry.
- Servers without identity report device/browser login disabled. Older servers may not expose discovery at all; diagnostics explain the API-key fallback. Existing `NINES_API_KEY`, `API_TOKEN`, loopback SSO, exit codes and structured errors remain supported.

## Scripts and LLM skills

```sh
nines capabilities --agent > capabilities.json
nines skills show nines-cli --offline > SKILL.md
nines search --context production --kind slo --query checkout --agent
nines plan --context production -f reviewed.yaml --agent > plan.json
nines apply --context production -f reviewed.yaml --expected-revision 0 --agent
```

- `--agent` selects JSON for data and errors, prohibits browser login, and never prompts.
  - It does not infer permission to mutate resources or disable model calls on commands explicitly marked as such.
  - The offline capability catalog exposes modes, possible effects, flags, defaults and artifact formats; it grants no authorization.
  - It does not evaluate a skill or invoke an LLM.
  - Exact revisions, intent versions, measurement confirmation and reviewed rule digests remain mandatory. Server role restrictions remain authoritative.
- Successful data goes to stdout; login instructions and errors go to stderr.
  - Main buffers output until successful completion. `--output json|yaml` gives structured data with canonical API field names.
  - List commands default to tables for humans; operational results default to JSON.
  - Manifest, Kubernetes and rule exports always emit YAML, including in agent mode; explicit incompatible output flags fail.
  - With `--out`, the artifact goes to the exclusively created file; human mode prints its path and agent mode emits a JSON `CLIArtifact` receipt with path, size and SHA-256. `skills show` defaults to Markdown (JSON with `--agent` or explicit structured output). `history -o csv` preserves its existing time-series CSV contract. No ANSI styling is emitted.
  - Unknown values in human tables read `unknown`.

| Exit | Meaning                                    |
| ---- | ------------------------------------------ |
| 0    | Success                                    |
| 1    | Local execution or validation failure      |
| 2    | Invalid command/flag/argument usage        |
| 3    | Missing, invalid or expired authentication |
| 4    | Tenant or operation forbidden              |
| 5    | Revision/digest/configuration conflict     |
| 6    | API unavailable, timeout or cancellation   |

- Agent errors use `{apiVersion, kind: CLIError, error: {code, message, exitCode}}`.
  - API response bodies and credentials are excluded from errors.
  - The CLI blocks redirects and requires HTTPS, except loopback HTTP for development. `--timeout` bounds the command; provider/SDK operations may impose shorter limits. No mutation is automatically retried.
  - After an ambiguous timeout, read back current revisions or the intent apply receipt before deciding whether to retry the exact reviewed request.
- Deterministic here means typed inputs, the shared compiler, stable ordering, explicit side-effect classes and revision/digest checks.
  - It does not mean byte-reproducible live telemetry, timestamps or optional model output.
  - Native resource types retain the shared `nines.dev/v1alpha1` YAML contract.
- Generate shell completion with `nines completion bash|zsh|fish|powershell`. Install the generated script in the shell's completion directory according to that shell's instructions; running generation does not modify shell configuration.
- See [testing and verification limits](public/testing.md) for required clean-checkout and external-provider checks.
- For the controller image, CRD installation, provider prerequisites and ownership boundaries, see the [operator guide](public/deployment/operator.md). Offline rendering does not install a controller or synchronize resources into the portal.
- See [PromQL validation](public/promql-validation.md) for examples, shared API/operator behavior and the limits of offline checks.
- Operator-backed connections use the same `rules plan` and `rules apply` commands.
  - Their plans include Kubernetes resource YAML and ownership/generation preconditions. `rules withdraw --id SLO_ID --expected-digest REVIEWED_DIGEST` removes only an owned operator publication while preserving the portal definition and data.
  - Pending cleanup retains the route pin; refresh/review and repeat withdrawal after cleanup to release it. See [portal publishing](public/deployment/operator.md#publish-portal-slos-through-the-operator).

## Intent decision audit and rollback

- `intent conversations audit ID` reads the tenant-scoped decision log and frozen recommendation evidence. `reject ID -f request.yaml` rejects a proposal or withdraws approval with an exact digest, current conversation version, `confirm: true` and a reason.
- `rollback-plan`, `rollback-approve`, `rollback-apply` and `rollback-reject` use explicit operational YAML through the same context/authentication contract.
  - Rollback appends configuration revisions, rejects intervening edits and does not deploy external rules.
  - Newly created resources have no previous revision and produce a blocked rollback. `rollback-kubernetes ID --namespace NAME --digest SHA256` exports exact restore definitions for a separate GitOps review.
- See [recommendation review, audit and rollback](public/intent-governance.md) for request sequencing, approval notes, permission boundaries and production limitations. Machine output mode does not reduce the privileges of the configured credential; assistants should use the server's agent role.

### Archived SLO revisions

- Measurement selectors, service, source, sampling interval and binding references can be edited under the same SLO name. `plan` reports `measurementChanged` and reset/deployment warnings.
  - Apply with the current expected revision; old measurement evidence is archived atomically and is not reused by the new measurement.
  - Binding provenance must still match the declared query contract.
- `nines history SLO --revision 1 --range 1h --resolution 1m --agent` reads a replaced revision's original definition and retained observations. Omit `--revision` for current history. Inspect definition history through `GET /api/slos/SLO/revisions` and `GET /api/slos/SLO/revisions/1`. See [SLO editing](public/slo-editing.md).

## Verify an SLO-owned measurement

Prefer one self-contained [SLO YAML](../examples/request-success.slo.yaml) with `spec.measurement`. Offline validation and rule generation derive selectors deterministically. Connected telemetry verification is read-only:

```sh
nines telemetry verify -f examples/request-success.slo.yaml --context production --agent
```

- It returns the SLO manifest and digests with recent source metadata/series evidence; it creates no binding or SLO and deploys no rules.
  - Use `plan` and the existing revision-fenced `apply` afterward.
  - MCP `verify_slo_measurement` and Go `CompileMeasurement` use the same endpoint.
  - For an inline intent conversation, confirmation YAML uses `measurementDigest`, `intentDigest`, `expectedVersion` and `confirm: true`, with no `binding`. Only owner/editor roles can confirm. See [measurements](public/measurements.md).

## Automatic rule deployment controls

- For an owner-configured automatic source, saved SLO revisions reconcile without `rules apply`.
  - The playground enables this by default; other deployments opt in with `rules.autoDeploy: true`. `integration` includes durable automation progress and failure evidence.
  - Owners can pause/resume one SLO using an exact revision:

```sh
nines rules pause --id checkout-success --expected-revision 2 --agent
nines rules resume --id checkout-success --expected-revision 2 --agent
```

- Pause retains loaded rules and survives PostgreSQL-backed restarts and SLO edits. Resume queues the current saved definition. These owner controls neither edit nor approve an SLO. See [automatic rule delivery](public/deployment/automatic-rules.md).

### Read recent SLO measurements

- `nines collect SLO_ID --recent --context CONTEXT --agent` reads a bounded recent range from the SLO's configured source using the server clock.
  - It commits measurement data without changing the SLO definition or deploying rules.
  - Editors and owners may collect; this is a write operation in the command catalog.
  - Explicit `--from` and `--to` remain available for backfills and cannot be combined with `--recent`.
  - A returned `collected: 0` means no measurement rows matched, not a successful SLI calculation.
  - Inspect the source's metric names and labels in the portal before changing a selector.

### Decision audit journal

- Owners/editors can search tenant decisions with `nines audit list --agent`, inspect a frozen proposal with `nines audit show RECORD_ID --agent`, or export matching full records with `nines audit export --agent` (NDJSON).
  - All use the selected server/tenant context; offline mode rejects them.
  - Filters include actor subject, resource, conversation, action, outcome and RFC3339 from/to dates.
  - Check export exit status before retaining the output.
  - Approvals require a nonempty review reason through every client, and a different owner/editor subject must apply that approval or rollback.
  - The bundled CLI skill describes these boundaries.
  - See [governance](public/intent-governance.md) for pagination, ambiguous apply outcomes and archive limitations.

## SLO templates

- Use `nines --offline templates list --category agent`, `nines templates show agent-task-completion` and `nines templates generate agent-task-completion --out task.slo.yaml`.
  - The shared catalog covers request serving, pipelines, storage, agents, LLMs and inference.
  - Generated manifests start as experimental SLOs with illustrative metrics and targets. `--values` accepts strict YAML overrides; `--agent` returns a machine-readable draft receipt.
  - Rendering never verifies telemetry, applies resources or deploys rules. See [SLI menus and templates](public/slo-templates.md) for the complete workflow.

## Declarative extensions

- Use `nines --offline extensions test -f bundle.yaml`, `extensions install --file bundle.yaml --sha256 EXPECTED_DIGEST --lock extensions.lock.yaml`, and `templates generate EXTENSION--RECIPE --extension-lock extensions.lock.yaml --values values.yaml`. Explicit HTTPS installation supports `--url`.
  - Git installation uses `--git REPOSITORY --commit FULL_COMMIT_ID --git-path bundles/extension.yaml` with the same trusted `--sha256`.
  - Public HTTPS repositories and existing local clones are supported; `--offline` permits local clones and forbids remote downloads.
  - The operator renderer accepts the same `--template`, `--extension-lock` and `--values` inputs. See [extension authoring, distribution and tenant configuration](public/slo-extensions.md).

## Kubernetes dependency discovery

- `dependencies import-kubernetes --offline --namespace commerce -f resources.yaml --out map.yaml` projects a local Kubernetes export into a reviewable dependency map without loading credentials or contacting a server.
  - Omit `--out` for a JSON receipt with candidate evidence and warnings. `dependencies discover --namespace commerce --context production --agent` requests an explicitly enabled namespace through the Nines server's in-cluster reader.
  - Both commands only preview; review the map and use the normal revision-checked `plan`/`apply` flow to save it.
  - See [service dependencies](public/service-dependencies.md) for permissions, annotations, limitations and the visual editor.

## Retire objectives

- Use `nines retire plan ID --action archive --agent` to inspect references and the exact revision/digest, then `nines retire apply ID --action archive --expected-revision REVISION --digest DIGEST --agent`. `retire status` reads the durable cleanup/audit receipt; `retire retry` retries the original owned cleanup when due.
  - Permanent deletion needs a separate `--action delete` plan and `--confirm-name ID`.
  - Mutations require a tenant owner; offline mode is refused.
  - See [SLO retirement](public/slo-retirement.md) for retained history, reference resolution and recovery.

## Migrate from Sloth or Pyrra

[Conversion](public/migration.md) is offline: it reads a file, prints or writes Nines definitions and a report, and contacts no server.

```sh
nines import sloth -f slo.yaml --out-dir slos/ --project payments --window 30d
nines import pyrra -f pyrra-slos.yaml --out-dir slos/ --lifecycle experimental
nines import sloth -f slo.yaml --agent     # definitions, notes and skipped items as JSON
```

Imported SLOs start `experimental`, so Nines does not page while your current rules run. Review the notes and skipped items, then `nines plan` and `nines apply` each file. `--force` overwrites existing files in `--out-dir`.

## Git delivery

Optional [Git delivery](public/git-delivery.md) opens a pull or merge request for a reviewed definition and never applies it.

```sh
nines git info
nines git propose -f reviewed.yaml --expected-revision 0 --digest PLAN_DIGEST
nines git proposals SLO_ID            # add --journey for a journey
nines git drift SLO_ID                # compares the repository file with the live digest
```

`propose` needs the revision and digest from the plan you reviewed. `proposals` marks a merged request as applied only when its digest equals the live definition.

## Visual evidence and recording deletion

- `nines visual assess JOURNEY` requires an exact `--revision`, fixed request `--id`, `--route-digest` from reviewed capabilities, `--image`, `--criterion` and explicit `--consent`. See [visual journey evidence](public/visual-evaluations.md) for privacy and idempotency semantics.
- `nines visual list JOURNEY` reads bounded receipts. `nines visual review JOURNEY EVALUATION --digest ... --outcome ... --reason ...` saves a separate immutable owner/editor review. Neither command publishes SLI data.
- `nines replays ingest` requires an owner or editor to attest both capture and tenant-sharing consent. Agent-role credentials cannot grant those attestations.
- `nines replays delete ID --expected-version N --confirm-id ID` permanently removes the native stored recording with owner permission. It does not delete exports or backups.
- `--agent` does not authorize consent. Agents must obtain explicit permission from the responsible human before sending images or deleting recordings.


## Resumable headless workflows

- `nines quality ID --customers --context production --agent` reads per-customer reliability for the SLO's `spec.customers` allowlist: SLI, target, burn, events and whether the service-wide average hides the customer. It is an explicit, bounded provider read and never changes a definition.
- `nines dependencies composition MAP_ID --context production --agent` checks each committed availability SLO in a dependency map against the product of its critical dependencies' committed availability targets. It reads stored definitions only and reports exceeded ceilings, headroom and unmeasured dependencies.
- `nines intent conversations usage --days 30 --context production --agent` returns aggregate workflow progress for the tenant: conversations started, proposed, approved and applied, how many applied on the first proposal, rejections, applied rollbacks, backtest attachment, guided versus model runs, and median times to proposal and to applied. It reads stored conversations only and contains no goal text or actor.
- `nines intent conversations status ID --context production --agent` returns the saved phase, next action, exact version/digests, human review gate and recorded observation outcome.
- `nines intent conversations report ID --context production --agent` emits a verified JSON snapshot. Add `--markdown` for a portable Markdown attachment; share its potentially sensitive definitions and review notes only with authorized recipients.
- Status is read-only. It does not call a model, query telemetry or authorize mutation. Unknown states require inspection; a committed configuration is not marked healthy.
- Read the [headless workflow guide](public/headless-workflows.md) for the full prepare/review/apply/verify/rollback sequence and executable CLI acceptance test.

## GitHub review evidence

- `nines intent conversations github-plan ID` prepares a stable review plan for an explicit repository, PR number and full head commit without contacting GitHub.
- `github-publish ID` requires the exact reviewed conversation version, change-set digest, report digest and `--confirm-sharing`. Set `NINES_GITHUB_TOKEN` privately; it is distinct from Nines authentication.
- Comments are immutable evidence snapshots, not approval or application. See [GitHub reviews](public/github-reviews.md) for consent, retries, concurrency limits and the opt-in Actions example.

## Native provider evidence

- `nines native results SLO_ID --from RFC3339 --to RFC3339`: on-demand current native Datadog/New Relic evidence, separate from Nines calculations.
- `nines native list CONNECTION`: bounded native catalog on the configured destination.
- `nines native draft CONNECTION --native-id ID --name NAME --project PROJECT --service SERVICE`: experimental YAML for normal plan/apply review. No vendor mutation or ownership adoption. `--agent` retains the draft envelope and limitations; ordinary YAML output sends limitations to stderr.
- These are connected, read-only operations. Offline mode prohibits them. [Provider limits](public/provider-adapters.md#native-results-and-reviewed-import) apply.

### History retention

`nines history SLO --range 1h --agent` reads retained Nines observations. The retention horizon is 24 hours or twice the SLO window, whichever is longer, shared by the CLI, HTTP history endpoint and MCP history tool. Older gaps are not filled automatically and increasing retention does not change SLO calculations.

## Intent measurement candidates

- Run `nines --context production --agent intent candidates -f examples/intent-candidates.yaml` for bounded, read-only provider discovery. The YAML contains exact service/source/project, explicit population filters and optional literal prefix, metrics, category, outcome and unit-bearing latency threshold.
- HTTP/API, Go client and MCP share the same receipt. Inspect `state`, `truncated`, candidate questions and digests; exit zero only means a report was returned. Unsupported discovery is not missing telemetry.
- The command does not draft, confirm, apply or deploy an SLO and does not call a model. See [candidate discovery](public/intents.md#discover-measurement-candidates) and `make test-intent-candidates`.

## Intent evidence and trade-offs

- `nines --context production --agent intent evidence -f examples/api-success.intent.yaml` reads a complete inline intent without a model, writes or deployment. Add `--expected-revision N` for an existing SLO identity.
- Use `examples/intent-evidence.yaml` for explicit target/window/policy alternatives and optional provenance pins. Historical sampling requires an explicit `history` range. First reviews capture current digests without confirming semantics.
- Inspect current quality, freshness, comparison/history states and gaps. Exit zero does not establish sufficient evidence or approval. See [the complete contract](public/intents.md#inspect-evidence-and-compare-explicit-choices) and `make test-intent-evidence`.
