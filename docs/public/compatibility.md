# Compatibility and deprecation policy

This page states what you can rely on when you upgrade Nines, and how much notice you get before anything you depend on changes.

## What the policy covers

The policy covers every capability at the stable or beta [maturity level](maturity.md). Experimental capabilities are excluded: they are opt-in, labelled in the portal and in `nines capabilities --agent`, and may change or be removed in any release.

It applies to these surfaces:

| Surface                                | What is covered                                                                                            |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| YAML definitions                       | `nines.dev/v1beta1` SLOs, critical user journeys, measurement bindings and dependency maps, and their meaning |
| HTTP API                               | Documented routes, request and response fields, status codes and error codes                                |
| Go client                              | Exported identifiers in `github.com/ishantanu/nines/client` and `api/...` packages                          |
| CLI                                    | Commands, flags, exit codes and `--agent` JSON output                                                       |
| MCP tools                              | Tool names, inputs and outputs                                                                              |
| Kubernetes operator                    | CRD versions and fields, and the shipped RBAC                                                               |
| Configuration and environment variables | Server, operator and CLI configuration keys and environment variables                                      |
| Database                               | Upgrading the PostgreSQL schema in place                                                                    |

## Versions

- Releases use [semantic versioning](https://semver.org) from `v1.0.0`.
  - A **major** release (`v2.0.0`) may remove something deprecated.
  - A **minor** release (`v1.4.0`) only adds: new fields, flags, routes, commands or capabilities.
  - A **patch** release (`v1.4.1`) only fixes defects.
- Before `v1.0.0`, beta releases already follow the deprecation process below. Removals before `v1.0.0` are announced at least one release ahead and come with a migration path, as the [operator upgrade to v1beta1](deployment/operator.md#upgrade-to-v1beta1) does.

## Breaking and non-breaking changes

A change is **breaking** if a correct use that worked before stops working or changes meaning. For example:

- removing or renaming a field, flag, route, command, tool, CRD version, configuration key or environment variable;
- making an optional input required, or narrowing what an input accepts;
- changing a default, an exit code, an error code or the meaning of a value;
- changing how an SLI, error budget or burn rate is calculated for the same definition and telemetry.

These are **not breaking**, so integrations must tolerate them:

- adding a response field, `--agent` output field, enum value, route, flag, command or tool (ignore fields you do not recognise);
- adding an optional input with a default that preserves the old behaviour;
- changing human-readable text: messages, table output and portal wording (parse `--agent` JSON, not tables);
- adding a warning.

## Deprecation process

1. **Announce.** The release that deprecates something lists it under **Deprecations** in its release notes, with what to use instead, and it is added to the table below.
2. **Warn at the point of use.** Using it produces a warning everywhere it can be used:
   - the server adds an HTTP `Warning: 299` header, the form Kubernetes uses;
   - the CLI prints `Warning: …` on standard error, once per message, never on standard output;
   - the Go client passes each warning to `client.Config.OnWarning`;
   - a deprecated CRD version is marked deprecated, so the Kubernetes API server warns `kubectl` and other clients;
   - validation and planning include it in `notes`.
3. **Keep it working.** A deprecated stable feature keeps working for at least **12 months** after the release that announced it, and is removed only in a **major** release.
4. **Remove.** The removal is listed in the release notes with the migration path.

## Exceptions

- **Security.** A vulnerability may be fixed in a way that breaks a use that depended on it. The release notes say so.
- **Incorrect calculations.** If Nines computed an SLI, error budget or alert condition wrongly, the fix ships in a patch release and is called out in the release notes. Results can change for the same telemetry; the definition you wrote keeps its documented meaning.
- **Provider behaviour.** Changes in a telemetry provider's own API are outside Nines' control. Adapters follow them as compatibly as possible.

## Upgrades

- Database migrations run automatically when the server starts and only move forward.
- A server refuses to start against a database written by a newer release, rather than reading data it does not understand. To roll back across a schema change, restore the database backup taken before the upgrade.
- You can upgrade directly from any earlier release in the same major version, and from the last minor release of the previous major version.
- Each release is tested by upgrading a database written by earlier releases (`make test-upgrade`).

## Current deprecations

<!-- deprecations:start -->
| Deprecated | Surface | Use instead | Since | Removed no earlier than |
| --- | --- | --- | --- | --- |
| nines.dev/v1alpha1 ServiceLevelObjective, CriticalUserJourney, MeasurementBinding and ServiceDependencyMap documents | YAML definitions | write nines.dev/v1beta1; nines export prints any stored definition in it | v0.1.0-beta.1 | v2.0.0 |
<!-- deprecations:end -->

The table is generated from the same list that produces the warnings, and a test keeps them in step.
