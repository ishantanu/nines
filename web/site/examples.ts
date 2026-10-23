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
# Saving the sample SLO automatically deploys its rules.

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
      "After automatic rule deployment, open the playground Prometheus query page. Compare observed reliability with the target across the configured windows.",
    language: "PromQL",
    code: `# SLO target
nines:slo_target:ratio{nines_tenant="prometheus",nines_slo="checkout-success"}

# Measured SLI for the short burn window
nines:sli:ratio{nines_tenant="prometheus",nines_slo="checkout-success",window="60s"}

# Error-budget consumption rate
nines:slo_burn_rate:ratio{nines_tenant="prometheus",nines_slo="checkout-success"}

# Recorded configuration identity in this query dataset
nines:rule_definition:info{nines_tenant="prometheus",nines_slo="checkout-success"}

# Active critical or warning burn alerts
ALERTS{alertname="NinesSLOBurnRate",alertstate="firing"}`,
    note: "Use localhost:19090/query and /rules. Prometheus's /metrics endpoint exposes its own instrumentation, not recording-rule results. Use nines integration SLO_ID --agent for stage-specific diagnostics, query-dataset output verification and next steps. The playground automatically reconciles saved SLO rules and reloads Prometheus. Other data sources opt in with rules.autoDeploy: true. File written, activation and runtime evaluation are separate results. The configuration marker does not verify traffic coverage or alert delivery. Configured alerts can be inactive while traffic is healthy.",
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
  {
    id: "operator",
    label: "Kubernetes operator",
    title: "Manage SLOs with Kubernetes and Git",
    description:
      "Run the standalone controller with namespace RBAC, pinned provider routes and one lease-elected writer. No Nines server or database is required.",
    language: "Shell",
    code: `# Source checkout: inspect the installation bundle before applying.
make operator
bin/nines-operator -version
kubectl kustomize deploy/operator

# Set your namespace, tenant, sources, selectors and versioned image.
# Apply your reviewed installation, then the SLO/CUJ example.
kubectl apply -f examples/operator/compiled.yaml
kubectl -n team-commerce get slo,cuj,prometheusrules
kubectl -n team-commerce get slo checkout-availability -o yaml`,
    note: "The installation guide includes source/archive image builds, credentials, selectors, upgrades and rollback. Portal-owned SLOs can use rules.mode: operator with a trusted namespace/publisher and dedicated service account. Reviewed apply publishes a Kubernetes SLO; rules withdraw preserves the portal definition and waits for cleanup. Configuration readiness, rule evaluation and query output are separate evidence; a configured resource does not establish alert delivery.",
    path: "docs/public/deployment/operator.md",
    link: "Operator installation and operations",
  },
  {
    id: "intent-checks",
    label: "Intent checks",
    title: "Evaluate intent workflows before enabling a model",
    description:
      "Run synthetic drafting and reviewed-apply scenarios from a source checkout. Check provider contracts, missing telemetry, stale approvals and recovery after a lost response.",
    language: "Shell",
    code: `# No server, model credentials or external telemetry required.
make intent-eval                 # 75 drafting contract checks
make intent-workflow-eval        # 60 workflow contract checks

# Save a private JSON report to a new file.
go run ./integration/intent-eval --suite workflow \\
  --out intent-evaluation-workflow.json

# Run one scenario across all five provider adapter profiles.
go run ./integration/intent-eval --suite workflow \\
  --case availability-reviewed-apply`,
    note: "Fixtures use scripted model responses and disposable in-memory workspaces, including test-only approved writes. They do not prove real model quality. Live evaluation requires explicit --live and provider/model configuration.",
    path: "docs/public/intent-evaluations.md",
    link: "Evaluation guide and measured model limitations",
  },
];
