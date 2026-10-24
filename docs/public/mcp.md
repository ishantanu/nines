# MCP and deterministic skills

Nines exposes a stdio MCP adapter and a deterministic CLI. Both use the same tenant-scoped HTTP contracts as the portal. The CLI can also validate definitions and generate rules offline without a server, identity provider or model.

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

The adapter reads the Nines credential from `API_TOKEN`. Its server and tenant are fixed at startup; a prompt cannot supply an arbitrary provider endpoint or Mimir organization. Use the server's agent role or a narrower service account for automated clients.

## Mutation boundaries

Read-only tools are the default. `-allow-writes` enables the documented mutation surface; it does not grant the workflow model approval authority. Rule deployment additionally requires `-allow-rule-deployment` and the API owner role. Operator withdrawal is a further explicit opt-in through `-allow-rule-withdrawal`.

A client flag cannot bypass server-side roles, tenant boundaries, expected revisions or reviewed digests. The existing integration tool reads automatic-deployment evidence; it does not expose a new pause/resume mutation tool.

Remote HTTP/OAuth transport is not currently implemented. This guide describes the shipped stdio adapter.

## Use the CLI as a skill

For a deterministic authoring workflow:

```sh
nines validate --offline -f examples/request-success.slo.yaml
nines rules generate --offline -f examples/request-success.slo.yaml --tenant production
nines telemetry verify -f examples/request-success.slo.yaml --context production --agent
nines plan -f examples/request-success.slo.yaml --context production --agent
```

The first two commands require no server. Connected verification and planning use the selected tenant context. `--agent` selects structured output; it does not create an identity or elevate permissions. Provide `NINES_API_KEY` through the automation environment and retain the explicit review boundary before apply.

See [CLI reference](../cli.md), [intents](intents.md), [identity setup](deployment/identity-and-tenants.md) and [current experimental boundaries](experimental.md).
