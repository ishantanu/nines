# Experimental features

These features exist in source and demos, but are outside the initial supported public deployment path. Their API, output and operational behavior may change before a stable release.

| Feature | Present | Remaining release boundary |
| --- | --- | --- |
| Intent/model workflows | Provider-configurable drafting, bounded tools, explicit bindings, reviewed SLO/CUJ changes, apply receipts and a [drafting and workflow evaluation harness](intent-evaluations.md) | [Real-provider acceptance (#34)](https://github.com/ishantanu/nines/issues/34), model behavior tests, cost/scale and operator experience |
| Session replay | Consented masked app-tab capture, linked revisions, playback and portable HTML export | Browser/platform compatibility, masking/privacy review and offline-player acceptance |
| Kubernetes operator | Shared CRDs, deterministic rules, tenant/source bounds, ownership, leases and Mimir cleanup | Real-cluster admission, RBAC, garbage collection, lease failure and conformance tests |
| Observed dependency maps | Declared links and configured service-graph evidence | Instrumentation coverage and production-scale validation |

Replay masking is not anonymization. Layout, CSS/classes and retained metadata may be sensitive, and downloaded copies leave server retention/RBAC controls. Recordings are not videos of other browser tabs or the operating-system desktop.

Models are optional and provider-neutral. No model is trusted to approve its own changes, supply provider credentials, choose a Mimir organization or change deterministic arithmetic. Agent-mode CLI output is a machine contract, not blanket authorization to execute every discovered command.

The stdio MCP adapter is included in the MVP client surface. Remote MCP HTTP/OAuth is not implemented. It defaults to read-only tools; opt-in write/deployment flags still require server permissions. External clients are responsible for their own model runtime and consent controls.

The [operator quickstart](deployment/operator.md) distinguishes offline GitOps rendering from the experimental live controller and lists its provider prerequisites.
