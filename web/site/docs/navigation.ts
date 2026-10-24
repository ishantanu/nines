export interface DocPage {
  slug: string;
  title: string;
  source: string;
  description: string;
}
export interface DocGroup {
  title: string;
  pages: DocPage[];
}
const page = (
  slug: string,
  title: string,
  description: string,
  source = `docs/public/${slug || "index"}.md`,
): DocPage => ({ slug, title, description, source });
export const docGroups: DocGroup[] = [
  {
    title: "Getting started",
    pages: [
      page(
        "",
        "Introduction",
        "Choose a starting point and understand how Nines measures reliability.",
      ),
      page(
        "deployment/playground",
        "Local playground",
        "Run the complete Prometheus flow with Docker Compose.",
      ),
      page(
        "deployment",
        "Installation",
        "Choose a binary, Compose, Kubernetes or operator deployment.",
      ),
      page(
        "architecture",
        "Architecture",
        "Understand the server, CLI, tenant boundaries and telemetry flow.",
      ),
    ],
  },
  {
    title: "Define reliability",
    pages: [
      page(
        "measurements",
        "SLOs and measurements",
        "Define successful events, targets and rolling windows in one YAML resource.",
      ),
      page(
        "latency-slos",
        "Latency SLOs",
        "Measure requests within a duration threshold using histogram buckets.",
      ),
      page(
        "critical-user-journeys",
        "Critical user journeys",
        "Connect user outcomes to the SLOs on which each journey step depends.",
      ),
      page(
        "slo-editing",
        "Edit and version SLOs",
        "Change an SLO under the same identity and retain its previous evidence.",
      ),
      page(
        "promql-validation",
        "PromQL validation",
        "Understand syntax checks and the limits of static query validation.",
      ),
    ],
  },
  {
    title: "Work with agents",
    pages: [
      page(
        "intents",
        "SLO intents",
        "Move from a reliability goal to confirmed measurements and reviewed changes.",
      ),
      page(
        "intent-governance",
        "Approvals, audit and rollback",
        "Review recommendation rationale and restore configuration safely.",
      ),
      page(
        "intent-evaluations",
        "Model evaluation",
        "Configure providers and understand measured workflow limitations.",
      ),
      page(
        "cli",
        "CLI reference",
        "Use deterministic offline commands and tenant-scoped connected operations.",
        "docs/cli.md",
      ),
      page(
        "mcp",
        "MCP and skills",
        "Connect agent clients using scoped tools and deterministic CLI skills.",
      ),
      page(
        "experimental",
        "Experimental features",
        "Check the current support boundary before enabling a feature.",
      ),
    ],
  },
  {
    title: "Deploy and connect",
    pages: [
      page(
        "configuration",
        "Server configuration",
        "Configure tenants, sources, identity and credentials.",
      ),
      page(
        "deployment/docker-compose",
        "Docker Compose",
        "Run a persistent server with PostgreSQL.",
      ),
      page(
        "deployment/kubernetes",
        "Kubernetes server",
        "Deploy the portal and API with explicit configuration and secrets.",
      ),
      page(
        "deployment/operator",
        "Kubernetes operator",
        "Reconcile GitOps resources and publish portal-owned SLOs.",
      ),
      page(
        "deployment/linux-service",
        "Linux service",
        "Run the native server under a service manager.",
      ),
      page(
        "deployment/identity-and-tenants",
        "Identity and tenants",
        "Configure local accounts, Entra/OIDC, LDAP and tenant access.",
      ),
      page(
        "deployment/identity-acceptance",
        "Identity acceptance",
        "Test browser and CLI authorization using controlled and real providers.",
      ),
      page(
        "deployment/metrics-and-rules",
        "Prometheus and Mimir",
        "Choose the query dataset and the destination that evaluates generated rules.",
      ),
      page(
        "deployment/automatic-rules",
        "Automatic rule deployment",
        "Enable reconciliation and inspect runtime evidence or pause delivery.",
      ),
      page(
        "deployment/backup-and-restore",
        "Backup and restore",
        "Preserve definitions, identities and retained evidence.",
      ),
    ],
  },
  {
    title: "Operate and contribute",
    pages: [
      page(
        "operations",
        "Operational limits",
        "Understand retention, coverage and the supported operating envelope.",
      ),
      page(
        "testing",
        "Testing",
        "Run the relevant unit, database and provider acceptance checks.",
      ),
      page(
        "verification",
        "Verification record",
        "Read dated evidence and the limits of each verification pass.",
      ),
      page(
        "release",
        "Releases",
        "Build, verify and publish an attributable release candidate.",
      ),
      page(
        "alpha-release-notes",
        "Alpha release notes",
        "Review installation details and current alpha limitations.",
      ),
      page(
        "site",
        "Website and docs",
        "Preview and maintain this static documentation site.",
      ),
    ],
  },
];
export const docPages = docGroups.flatMap((group) => group.pages);
export const repository = "https://github.com/ishantanu/nines";
export const docPath = (page: Pick<DocPage, "slug">) =>
  `docs/${page.slug ? `${page.slug}/` : ""}`;
export function pageForSource(source: string) {
  return docPages.find((page) => page.source === source);
}
export function pageForPath(pathname: string) {
  const match = pathname.match(/(?:^|\/)docs(?:\/(.*))?$/);
  if (!match) return undefined;
  const slug = (match[1] ?? "")
    .replace(/\/?index\.html$/, "")
    .replace(/\/+$/, "");
  return docPages.find((page) => page.slug === slug);
}
export function rootForPage(page: DocPage) {
  return "../".repeat(1 + (page.slug ? page.slug.split("/").length : 0));
}
export function documentationLink(source: string, root = "./") {
  const suffixAt = source.search(/[?#]/);
  const file = suffixAt < 0 ? source : source.slice(0, suffixAt);
  const suffix = suffixAt < 0 ? "" : source.slice(suffixAt);
  const page = pageForSource(file);
  return page
    ? root + docPath(page) + suffix
    : `${repository}/blob/main/${source}`;
}
