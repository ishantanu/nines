# MCP and deterministic skills

Nines exposes a stdio MCP adapter, an opt-in [remote OAuth HTTP transport](remote-mcp.md), and a deterministic CLI.

- Both use the same tenant-scoped HTTP contracts as the portal.
- The CLI can also validate definitions and generate rules offline without a server, identity provider or model.

## Connect a read-only MCP client

Configure your MCP-capable client to launch the released `nines-mcp` executable. Supply credentials through its private process environment or your client's secret manager.

```json
{
  "mcpServers": {
    "nines": {
      "command": "nines-mcp",
      "args": ["-server", "https://nines.example.com", "-tenant", "production"]
    }
  }
}
```

- The adapter reads `NINES_API_KEY`, with `API_TOKEN` as a compatibility fallback, matching CLI precedence.
  - Its server and tenant are fixed at startup; a prompt cannot supply an arbitrary provider endpoint or Mimir organization.
  - Use the server's agent role or a narrower service account for automated clients.

## Mutation boundaries

- Read-only tools are the default. `-allow-writes` enables the documented mutation surface; it does not grant the workflow model approval authority.
  - Rule deployment additionally requires `-allow-rule-deployment` and the API owner role.
  - Operator withdrawal is a further explicit opt-in through `-allow-rule-withdrawal`.
- A client flag cannot bypass server-side roles, tenant boundaries, expected revisions or reviewed digests. `get_slo_integration` reads automatic-deployment evidence; it does not expose a new pause/resume mutation tool.
- Remote HTTP/OAuth is available as an opt-in experimental resource-server profile. See [configuration, scopes, client conformance and limits](remote-mcp.md). This page describes the default stdio adapter.

## Use the CLI as a skill

For a deterministic authoring workflow:

```sh
nines validate --offline -f examples/request-success.slo.yaml
nines rules generate --offline -f examples/request-success.slo.yaml --tenant production
nines telemetry verify -f examples/request-success.slo.yaml --context production --agent
nines plan -f examples/request-success.slo.yaml --context production --agent
```

- The first two commands require no server.
  - Connected verification and planning use the selected tenant context. `--agent` selects structured output; it does not create an identity or elevate permissions.
  - Provide `NINES_API_KEY` through the automation environment and retain the explicit review boundary before apply.
- See [CLI reference](../cli.md), [intents](intents.md), [identity setup](deployment/identity-and-tenants.md) and [current experimental boundaries](experimental.md).

## Measurement quality and budget decisions

- Use `slo_quality` to inspect saved freshness, count, coverage and source-lineage checks.
  - Set `cohorts: true` only for an explicit bounded provider read. `slo_budget_decision` reads the current configured response and its evidence; it changes no policy or deployment.
  - Both tools are tenant-bound and accept `expectedRevision`. `slo_context` includes the same quality and budget decision.
  - Neither an observed percentage nor a frozen proposal-time decision is current release permission.
- The read-only `verify_intent_changes` tool also returns the last saved post-change observation, including rule activation/output, quality, measured SLI and review/rollback guidance. It makes no provider call. Approval, corrective-work exceptions and rollback remain human-controlled. See [agent change safety](intent-governance.md#action-time-safety-checks).


## Resume an intent workflow

- Call read-only `get_intent_workflow_status` with the saved conversation `id` for the next operation, exact version/digests, human-review gate and recorded runtime outcome.
- Use `get_intent_review_artifact` to retrieve an audit-verified review snapshot, or `get_intent_conversation` for the full saved conversation. Artifacts can contain sensitive definitions and reviewer notes. A suggested action is not authorization, and MCP does not grant approval or application capabilities.
- See [headless workflows](headless-workflows.md) for CLI/API human review and the complete browser-free lifecycle.

## Native SLO reads

- `native_slo_results`: exact current native revision, explicit `id`, `from` and `to`; independent vendor results, not Nines measurement or alert-delivery proof.
- `list_native_slos`: pass the configured connection name as `id`; bounded Datadog/New Relic catalog, without account/route overrides.
- `draft_native_slo_import`: `source`, `nativeID`, new Nines `name`, `project` and `service`; returns experimental YAML and limitations. Does not save, adopt or modify vendor resources. Review through `plan_slo` before separately authorized application.

All three tools are read-only and available without enabling write tools. See [supported profiles and bounds](provider-adapters.md#native-results-and-reviewed-import).
