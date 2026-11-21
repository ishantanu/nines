# Experimental features

Experimental features are opt-in and may change or be removed in any release without deprecation. The [feature maturity](maturity.md#experimental) page lists every experimental feature, what each level promises, and what remains before each feature can become beta.

The notes below apply when you turn experimental features on.

- Replay masking is not anonymization.
  - Layout, CSS/classes and retained metadata may be sensitive, and downloaded copies leave server retention/RBAC controls.
  - Recordings are not videos of other browser tabs or the operating-system desktop.
- Models are optional and provider-neutral.
  - No model is trusted to approve its own changes, supply provider credentials, choose a Mimir organization or change deterministic arithmetic.
  - Agent-mode CLI output is a machine contract, not blanket authorization to execute every discovered command.
- The stdio MCP adapter is included in the MVP client surface. [Remote MCP HTTP/OAuth](remote-mcp.md) is opt-in and experimental, with explicit per-user/client credential bindings and local conformance tests. Hosted-client and production issuer acceptance remain deployment-specific.
  - It defaults to read-only tools; opt-in write/deployment flags still require server permissions.
  - External clients are responsible for their own model runtime and consent controls.
- The [operator quickstart](deployment/operator.md) covers the supported alpha controller and offline GitOps rendering, including provider prerequisites and acceptance boundaries.
