# Security

Nines is pre-release. No version is currently declared production-supported. Use the current candidate in an isolated evaluation environment until the release checklist is complete.

Do not include credentials, tenant data, session replay events, raw provider responses or exploit details in a public issue. Use the repository's **Security → Report a vulnerability** channel when private reporting is enabled. If that channel is unavailable, open an issue asking the maintainer for a private contact without disclosing the vulnerability. A maintainer must enable private reporting and confirm the contact path before publishing the repository.

Tenant isolation, role bypasses, leaked credentials, unsafe provider routing and untrusted model/tool execution are security issues. State the affected version, deployment mode, prerequisites and a minimal synthetic reproduction. There is no response-time or bounty commitment at this stage.

Deploy evaluation servers behind HTTPS and an appropriately configured authentication boundary. Keep the synthetic demo on loopback. Treat recording exports as sensitive even when masking is enabled. Model calls and telemetry exporters are operator-configured outbound integrations; review their destinations before enabling them.
