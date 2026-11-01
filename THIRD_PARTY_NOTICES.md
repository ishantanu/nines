# Third-party software

Nines' own code is licensed under Apache-2.0. Dependencies and incorporated components retain their original licenses and notices.

The React UI includes shadcn/ui-derived components. Their MIT notice is retained at `web/public/shadcn-license.txt` and in the embedded web build. Other UI dependencies include React, Radix UI, Tailwind, rrweb and OpenTelemetry. Go dependencies include Cobra, pgx, the MCP Go SDK, OIDC/LDAP clients, the Prometheus PromQL parser, the VictoriaMetrics MetricsQL parser and OpenTelemetry. Google Cloud authentication uses the Go OAuth2 library and Compute metadata client; supplied notices are collected with the other Go modules.

The release builder collects the installed Go modules' and npm packages' supplied root license/notice files into `LICENSES/` and records the package names, versions and source paths in `DEPENDENCIES.json`. Missing runtime notices require an explicit hash-bound upstream notice in `third_party/license-sources.json`; unrecognized gaps fail the builder. Version/provenance gaps in those records still require maintainer review. Missing notices from unbundled development-only packages are recorded separately rather than represented as runtime attribution. This inventory includes build dependencies; it is not a legal opinion, a complete recursive source audit or a vulnerability scan.

Before a public release, review the generated inventory, dependency notices and vulnerability results. Preserve upstream notices when modifying or redistributing their code. The standalone source export includes the lockfiles and shadcn/ui notice, and does not vendor the dependency source trees.

## Community acknowledgements

[Sloth](https://github.com/slok/sloth) and [Pyrra](https://github.com/pyrra-dev/pyrra) helped shape the approach to SLOs as code, rule generation and accessible reliability workflows. We credit their maintainers and contributors for that work. The dependency inventory above records the software actually bundled with Nines.

## Provider symbols

The portal bundles Simple Icons SVG symbols under CC0-1.0 and the separate official Grafana Mimir product mark. See `web/src/assets/providers/NOTICE.md` for the sources and attribution. Provider names and logos are trademarks of their respective owners; integration support implies no endorsement.
