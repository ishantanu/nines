import sdk from "../../examples/go-client/main.go?raw";

export const examples = [
  {
    id: "playground",
    label: "Playground",
    title: "Run the complete Prometheus flow",
    description:
      "Start Nines, PostgreSQL, Prometheus and a synthetic checkout service. Existing credentials and data are preserved on restart.",
    language: "Shell",
    code: `# Run from a Nines source checkout with Docker running.
([ -f .env ] || python3 scripts/setup-local.py --playground) && \\
  docker compose -p nines-playground -f compose.playground.yaml up --build -d

# Open http://127.0.0.1:18080 and sign in with admin / admin.
# Review and deploy the sample SLO's rules in its Rules tab.

# Optional: exercise collection, deployment, RBAC and alert recovery.
docker compose -p nines-playground -f compose.playground.yaml run --rm verify`,
    note: "The verifier deliberately injects synthetic failures and can take several minutes. Prometheus is at localhost:19090; Mimir is optional.",
    path: "docs/public/deployment/playground.md",
    link: "Playground guide",
  },
  {
    id: "offline",
    label: "Offline CLI",
    title: "Validate and generate rules without a server",
    description:
      "Keep the definition in Git. Validate its contract and render recording and severity-based alert rules before connecting to any provider.",
    language: "Shell",
    code: `# Build once from a source checkout, or use the alpha binary.
go build -o bin/nines ./cmd/nines

bin/nines validate --offline -f examples/api-availability.slo.yaml --agent

bin/nines rules generate --offline \\
  -f examples/api-availability.slo.yaml --tenant commerce

bin/nines capabilities --offline --agent`,
    note: "Offline validation does not check whether provider metrics exist. Generated rules are YAML output; this command does not deploy them.",
    path: "docs/cli.md",
    link: "CLI reference",
  },
  {
    id: "connected",
    label: "Tenant access",
    title: "Connect the CLI to one tenant",
    description:
      "Use a named context to make the server and tenant explicit. API keys come from the environment; configured SSO is also supported.",
    language: "Shell",
    code: `# Use a token issued for your deployment and tenant.
export NINES_API_KEY="YOUR_TENANT_API_KEY"

nines config set-context production \\
  --server https://nines.example.com --tenant commerce

nines get slos --context production --agent
nines plan --context production -f examples/api-availability.slo.yaml --agent

# Alternative: use the server's configured SSO provider.
nines login --context production --method sso`,
    note: "A plan previews a change. Applying it is a separate action that requires permission and the expected resource revision.",
    path: "docs/public/deployment/identity-and-tenants.md",
    link: "Identity and tenant guide",
  },
  {
    id: "metrics",
    label: "SLI and alerts",
    title: "Inspect the recorded SLI and budget burn",
    description:
      "After deploying reviewed rules, open the playground Prometheus query page. Compare observed reliability with the target across the configured windows.",
    language: "PromQL",
    code: `# SLO target
nines:slo_target:ratio{nines_tenant="prometheus",nines_slo="checkout-success"}

# Measured SLI for the short burn window
nines:sli:ratio{nines_tenant="prometheus",nines_slo="checkout-success",window="60s"}

# Error-budget consumption rate
nines:slo_burn_rate:ratio{nines_tenant="prometheus",nines_slo="checkout-success"}

# Active critical or warning burn alerts
ALERTS{alertname="NinesSLOBurnRate",alertstate="firing"}`,
    note: "Use localhost:19090/query and /rules. Prometheus's /metrics endpoint exposes its own instrumentation, not recording-rule results. Configured alerts can be inactive while traffic is healthy.",
    path: "docs/public/deployment/metrics-and-rules.md",
    link: "Metrics and rule destinations",
  },
  {
    id: "sdk",
    label: "Go client",
    title: "Preview a change with the typed Go client",
    description:
      "This complete example reads its connection from the environment, plans a typed SLO and requires an explicit opt-in before applying it.",
    language: "Go",
    code: sdk,
    note: "Set NINES_URL, NINES_TENANT and API_TOKEN. The example only applies when NINES_APPLY=true; the plan supplies the expected revision.",
    path: "examples/go-client/main.go",
    link: "Full Go example",
  },
  {
    id: "mcp",
    label: "MCP",
    title: "Give an agent tenant-scoped read access",
    description:
      "Add the stdio adapter to an MCP-compatible client. Use a viewer-role token and an explicit tenant; tool writes are disabled by default.",
    language: "JSON",
    code: `{
  "mcpServers": {
    "nines": {
      "command": "/path/to/nines-mcp",
      "args": [
        "--server", "https://nines.example.com",
        "--tenant", "commerce"
      ],
      "env": {
        "API_TOKEN": "YOUR_TENANT_VIEWER_TOKEN"
      }
    }
  }
}`,
    note: "The adapter needs a running Nines server. Store the real token in your client's private configuration or secret mechanism, outside source control.",
    path: "docs/cli.md#scripts-and-llm-skills",
    link: "Agent and skill integration",
  },
];
