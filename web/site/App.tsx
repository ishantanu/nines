import { NinesFeatures } from "./features/NinesFeatures";
import { SLOTypes } from "./features/SLOTypes";
import { useState } from "react";
import { ArrowRight, Check, Copy, Terminal, Waypoints } from "lucide-react";
import { Theme } from "@astryxdesign/core/theme";
import { ninesTheme } from "../src/astryx-theme";
import { Button } from "../src/components/ui/button";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "../src/components/ui/tabs";
import slo from "../../examples/request-success.slo.yaml?raw";
import journey from "../../examples/checkout.journey.yaml?raw";
import latency from "../../examples/request-latency.slo.yaml?raw";
import intent from "../../examples/api-success.intent.yaml?raw";
import { examples } from "./examples";
import { documentationLink } from "./docs/navigation";
import { WorkflowIllustration } from "./workflow/WorkflowIllustration";

import { inlineWordmark as logo } from "./brand";
const repo = "https://github.com/ishantanu/nines";
const start =
  "git clone https://github.com/ishantanu/nines.git\ncd nines\ngo run ./cmd/server -demo";

function CopyCommand({
  text,
  label = "Copy demo commands",
}: {
  text: string;
  label?: string;
}) {
  const [status, setStatus] = useState("");
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setStatus("Copied");
    } catch {
      setStatus("Select the command to copy it.");
    }
  }
  return (
    <div className="copy-control">
      <span role="status">{status}</span>
      <Button variant="outline" size="sm" onClick={copy} aria-label={label}>
        {status === "Copied" ? (
          <Check aria-hidden="true" />
        ) : (
          <Copy aria-hidden="true" />
        )}{" "}
        Copy
      </Button>
    </div>
  );
}

// Decorative: an aurora, a dot grid and a reliability trace with dips and a
// spike. Nothing here is data, so it is hidden from assistive technology.
function HeroBackdrop() {
  const trace =
    "M0 430 L110 426 L190 434 L270 421 L330 427 L372 500 L410 412 L500 420 L590 416 L650 426 L694 360 L730 432 L820 422 L910 430 L956 540 L996 446 L1060 416 L1160 410 L1230 426 L1280 320 L1316 424 L1390 418 L1440 412";
  return (
    <div aria-hidden="true" className="hero-backdrop">
      <div className="hero-aurora" />
      <div className="hero-grid" />
      <svg viewBox="0 0 1440 640" preserveAspectRatio="xMidYMid slice">
        <defs>
          <linearGradient id="hero-wave-fill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0" stopColor="currentColor" stopOpacity="0.2" />
            <stop offset="1" stopColor="currentColor" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={`${trace} V640 H0 Z`} fill="url(#hero-wave-fill)" />
        <path
          className="hero-trace"
          d={trace}
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          strokeLinejoin="round"
          pathLength="1"
        />
        <path
          d="M0 500 H1440"
          stroke="currentColor"
          strokeOpacity="0.5"
          strokeWidth="1.5"
          strokeDasharray="3 9"
          fill="none"
        />
        <circle
          className="hero-dot"
          cx="1230"
          cy="426"
          r="6"
          fill="currentColor"
        />
      </svg>
    </div>
  );
}

function WorkspacePreview() {
  return (
    <figure className="workspace-preview" aria-labelledby="preview-caption">
      <div className="preview-top">
        <span>Commerce / Checkout</span>
        <span className="preview-label">Synthetic example</span>
      </div>
      <div className="preview-body">
        <div className="objective-title">
          <div>
            <h2>Checkout success</h2>
          </div>
          <span className="objective-status">
            <span />
            Within objective
          </span>
        </div>
        <div className="metrics-row">
          <div>
            <span>Measured SLI</span>
            <strong>
              99.96<small>%</small>
            </strong>
          </div>
          <div>
            <span>SLO target</span>
            <strong>
              99.90<small>%</small>
            </strong>
          </div>
          <div>
            <span>Rolling window</span>
            <strong>
              28<small>days</small>
            </strong>
          </div>
          <div>
            <span>Error budget remaining</span>
            <strong>
              60.0<small>%</small>
            </strong>
          </div>
        </div>
        <div className="chart-heading">
          <span>
            <i className="legend-sli" />
            SLI
          </span>
          <span>
            <i className="legend-target" />
            SLO target
          </span>
          <span>Last 24 hours</span>
        </div>
        <svg
          className="sli-chart"
          viewBox="0 0 520 164"
          role="img"
          aria-labelledby="chart-title chart-desc"
        >
          <title id="chart-title">
            Illustrative SLI compared with its SLO target
          </title>
          <desc id="chart-desc">
            Synthetic availability fluctuates around 99.96 percent, with a brief
            dip below the 99.90 percent target. This is an illustration, not
            live telemetry.
          </desc>
          <g stroke="var(--folio-rule-soft)" strokeWidth="1">
            <path d="M0 26H520M0 65H520M0 104H520M0 143H520" />
          </g>
          <path
            d="M0 104H520"
            stroke="var(--folio-revision)"
            strokeWidth="1.5"
            strokeDasharray="5 5"
          />
          <path
            d="M0 45L20 39L40 43L60 32L80 37L100 34L120 42L140 35L160 43L180 39L200 51L220 40L240 46L260 39L280 61L300 125L320 88L340 43L360 38L380 42L400 28L420 36L440 32L460 38L480 27L500 37L520 32"
            fill="none"
            stroke="var(--folio-reference)"
            strokeWidth="2.7"
            strokeLinejoin="round"
          />
          <g
            fill="var(--folio-muted)"
            fontSize="10"
            fontFamily="ui-monospace,monospace"
          >
            <text x="0" y="160">
              00:00
            </text>
            <text x="244" y="160">
              12:00
            </text>
            <text x="487" y="160">
              24:00
            </text>
          </g>
        </svg>
        <div className="journey-link">
          <Waypoints size={17} aria-hidden="true" />
          <span>
            <span>Critical user journey</span>
            <strong>Complete checkout · Authorize payment</strong>
          </span>
          <ArrowRight size={16} aria-hidden="true" />
          <span>
            <span>Linked SLO</span>
            <strong>Checkout success</strong>
          </span>
          <span className="journey-count">3 linked · 2 required</span>
        </div>
        <div className="contract-stamp">
          <span>Journey definition</span>
          <code>checkout.journey.yaml</code>
          <a href="#as-code">
            Review YAML <ArrowRight size={14} aria-hidden="true" />
          </a>
        </div>
      </div>
      <figcaption id="preview-caption">
        Illustrative workspace with synthetic values.
      </figcaption>
    </figure>
  );
}

const journeySteps = [
  {
    name: "Submit cart",
    actor: "User",
    slo: "api-availability",
    dimension: "Availability",
    required: true,
  },
  {
    name: "Authorize payment",
    actor: "Service",
    slo: "checkout-success",
    dimension: "Correctness",
    required: true,
  },
  {
    name: "Send confirmation",
    actor: "Service",
    slo: "background-jobs",
    dimension: "Latency",
    required: false,
  },
];
function JourneyExample() {
  const [selected, setSelected] = useState(1);
  const step = journeySteps[selected];
  return (
    <figure className="journey-example">
      <div
        className="journey-example-path"
        role="group"
        aria-label="Example critical user journey steps"
      >
        {journeySteps.map((item, index) => (
          <div className="journey-example-step" key={item.slo}>
            <button
              type="button"
              aria-pressed={selected === index}
              aria-controls="journey-step-detail"
              onClick={() => setSelected(index)}
            >
              <span>{item.actor}</span>
              <strong>{item.name}</strong>
              <span className="journey-example-slo">{item.slo}</span>
            </button>
            {index < journeySteps.length - 1 ? (
              <ArrowRight aria-hidden="true" />
            ) : null}
          </div>
        ))}
      </div>
      <div
        className="journey-example-detail"
        id="journey-step-detail"
        aria-live="polite"
        aria-atomic="true"
      >
        <p>
          <strong>{step.name}</strong> depends on <code>{step.slo}</code>.
        </p>
        <span>
          {step.dimension} ·{" "}
          {step.required ? "Required for journey health" : "Optional objective"}
        </span>
        <a className="text-link" href="#as-code">
          Read the definition <ArrowRight size={15} aria-hidden="true" />
        </a>
      </div>
      <figcaption>
        Illustrative journey from <code>checkout.journey.yaml</code>. Select a
        step to inspect its SLO link.
      </figcaption>
    </figure>
  );
}

export function App() {
  const [exampleId, setExampleId] = useState("playground");
  return (
    <Theme theme={ninesTheme} mode="light">
      <a href="#main" className="skip-link">
        Skip to content
      </a>
      <header className="site-header">
        <div className="container nav-row">
          <a className="brand" href="#" aria-label="Nines home">
            <img src={logo} width="124" height="35" alt="Nines" />
          </a>
          <nav aria-label="Main navigation">
            <a href="#product">Product</a>
            <a href="#slo-types">SLO types</a>
            <a href="#features">Features</a>
            <a href="#architecture">Architecture</a>
            <a href="#as-code">As code</a>
            <a href="#examples">Examples</a>
            <a href="#agents">For agents</a>
            <a href={"./docs/"}>Docs</a>
          </nav>
          <Button variant="outline" size="sm" href={repo}>
            GitHub
          </Button>
        </div>
      </header>
      <main id="main">
        <div className="hero-stage">
          <HeroBackdrop />
          <section
            className="hero container"
            id="product"
            aria-labelledby="hero-title"
          >
            <div className="hero-copy">
              <h1 id="hero-title">
                Reliability, tied to the <em>user journey.</em>
              </h1>
              <p className="hero-description">
                Define what good looks like. Connect service level objectives to
                critical user journeys, then track the signals and error budgets
                that matter.
              </p>
              <div className="hero-actions">
                <Button size="lg" href="#quickstart">
                  Try Nines locally <ArrowRight aria-hidden="true" />
                </Button>
                <a className="text-link" href={repo}>
                  Explore the source <ArrowRight size={15} aria-hidden="true" />
                </a>
              </div>
              <p className="release-note">
                Pre-release MVP <span>·</span> Apache-2.0 <span>·</span>{" "}
                Self-hosted
              </p>
            </div>
            <WorkspacePreview />
          </section>
        </div>
        <div className="stack-bar" id="providers">
          <div className="container">
            <span>
              Connect your observability stack
              <br />
              <small>Five additional adapters, currently experimental</small>
            </span>
            <ul aria-label="Telemetry provider integrations">
              <li>
                <a href="./docs/deployment/metrics-and-rules/">Prometheus</a>
              </li>
              <li>
                <a href="./docs/deployment/metrics-and-rules/">Grafana Mimir</a>
              </li>
              <li>
                <a href="./docs/provider-adapters/">
                  Datadog <span className="sr-only">(experimental)</span>
                </a>
              </li>
              <li>
                <a href="./docs/new-relic/">
                  New Relic <span className="sr-only">(experimental)</span>
                </a>
              </li>
              <li>
                <a href="./docs/dash0/">
                  Dash0 <span className="sr-only">(experimental)</span>
                </a>
              </li>
              <li>
                <a href="./docs/google-cloud/">
                  Google Cloud Monitoring{" "}
                  <span className="sr-only">(experimental)</span>
                </a>
              </li>
              <li>
                <a href="./docs/provider-adapters/">
                  VictoriaMetrics{" "}
                  <span className="sr-only">(experimental)</span>
                </a>
              </li>
            </ul>
          </div>
        </div>
        <section
          className="container product-section"
          aria-labelledby="product-title"
        >
          <div className="section-heading">
            <h2 id="product-title">Give every objective a reason.</h2>
            <p>
              Service metrics describe a system. Critical user journeys explain
              why its reliability matters. Nines connects the two explicitly.
            </p>
          </div>
          <JourneyExample />
        </section>
        <SLOTypes />
        <NinesFeatures />
        <section
          className="container product-section portal-gallery"
          id="portal"
          aria-labelledby="portal-title"
        >
          <div className="section-heading">
            <h2 id="portal-title">Inside the portal.</h2>
            <p>
              SLI trends, budget burn and measurement coverage. Captured from
              the synthetic demo workspace.
            </p>
          </div>
          <div className="portal-gallery-grid">
            <figure>
              <a
                href="./docs/assets/nines-overview.jpg"
                aria-label="Open the full-size portal overview screenshot"
              >
                <img
                  src="./docs/assets/nines-overview.jpg"
                  width="1280"
                  height="720"
                  loading="lazy"
                  decoding="async"
                  alt="Nines overview with SLO trends and linked critical user journeys"
                />
              </a>
              <figcaption>
                Objectives and critical user journeys, with measured SLI trends.
              </figcaption>
            </figure>
            <figure>
              <a
                href="./docs/assets/nines-measurements.jpg"
                aria-label="Open the full-size measurement summary screenshot"
              >
                <img
                  src="./docs/assets/nines-measurements.jpg"
                  width="1280"
                  height="720"
                  loading="lazy"
                  decoding="async"
                  alt="Per-service budget burn and a donut showing complete and partial measurement windows"
                />
              </a>
              <figcaption>
                Budget burn by service and measurement-window coverage.
              </figcaption>
            </figure>
          </div>
        </section>
        <section
          className="container architecture-section"
          id="architecture"
          aria-labelledby="architecture-title"
        >
          <div className="section-heading">
            <h2 id="architecture-title">
              Your telemetry stack. One SLO workflow.
            </h2>
            <p>
              Connect Prometheus, Mimir, Datadog, New Relic, Dash0, Google Cloud
              Monitoring or VictoriaMetrics. Keep telemetry with your provider;
              define SLOs and critical user journeys in Nines. Additional vendor
              adapters are experimental, with provider-specific query and
              publication capabilities.
            </p>
          </div>
          <details className="architecture-details">
            <summary>
              Explore the deployment architecture{" "}
              <span>
                Provider APIs, reviewed publication, CLI, MCP and operator
              </span>
            </summary>
            <figure className="architecture-figure">
              <div
                className="architecture-viewport"
                role="region"
                aria-label="Architecture diagram, scroll horizontally on smaller screens"
                tabIndex={0}
              >
                <img
                  src="./nines-architecture.svg"
                  width="1200"
                  height="870"
                  loading="lazy"
                  decoding="async"
                  alt="Nines reads telemetry from Prometheus, Mimir, Datadog, New Relic, Dash0, Google Cloud Monitoring and VictoriaMetrics through provider APIs. Portal, CLI and MCP share tenant-scoped SLO review. Prometheus and Mimir receive recording and burn-rate rules; Datadog, New Relic, Dash0 and Google Cloud support reviewed native SLO publication within supported subsets. The standalone Kubernetes operator targets Prometheus and Mimir. Additional vendor adapters are experimental."
                />
              </div>
              <figcaption>
                <span>
                  Prometheus/Mimir rules reconcile automatically when enabled.
                  Native vendor SLO publication has a separate review and apply
                  flow. Configuration, evaluation and attainment remain separate
                  evidence.
                </span>
                <a
                  className="text-link"
                  href="./nines-architecture.svg"
                  download
                >
                  Download SVG <ArrowRight size={15} aria-hidden="true" />
                </a>
              </figcaption>
            </figure>
          </details>
          <div className="architecture-notes">
            <p>
              <strong>Provider-specific publication.</strong> Deliver recording
              and burn-rate rules to Prometheus or Mimir. Publish reviewed
              native SLOs to Datadog, New Relic, Dash0 or Google Cloud within
              supported subsets. VictoriaMetrics currently supports collection
              only. Native vendor alert creation is not included.{" "}
              <a className="text-link" href="./docs/provider-adapters/">
                Compare provider capabilities{" "}
                <ArrowRight size={15} aria-hidden="true" />
              </a>
            </p>
            <p>
              <strong>Independent operator.</strong> Apply GitOps resources
              directly without the Nines server, database or a model. Standalone
              operator resources are not imported into the portal.{" "}
              <a className="text-link" href={"./docs/deployment/operator/"}>
                Read the operator guide{" "}
                <ArrowRight size={15} aria-hidden="true" />
              </a>
            </p>
          </div>
        </section>
        <section
          className="code-section"
          id="as-code"
          aria-labelledby="code-title"
        >
          <div className="container code-layout">
            <div className="code-copy">
              <h2 id="code-title">
                Reliability belongs
                <br />
                in version control.
              </h2>
              <p>
                SLOs and critical user journeys share a YAML API. Review changes
                in Git, validate offline, and apply against an explicit resource
                revision.
              </p>
              <dl className="contract-list">
                <div>
                  <dt>Versioned YAML</dt>
                  <dd>
                    A shared <code>nines.dev/v1beta1</code> contract.
                  </dd>
                </div>
                <div>
                  <dt>Explicit links</dt>
                  <dd>Journey steps refer to the SLOs they depend on.</dd>
                </div>
                <div>
                  <dt>Reviewed changes</dt>
                  <dd>Plan before applying. Check the expected revision.</dd>
                </div>
              </dl>
              <a className="text-link" href={"./docs/measurements/"}>
                Explore SLO definitions{" "}
                <ArrowRight size={15} aria-hidden="true" />
              </a>
            </div>
            <Tabs defaultValue="slo" className="definition-panel">
              <div className="definition-toolbar">
                <TabsList aria-label="Definition examples">
                  <TabsTrigger value="slo">SLO</TabsTrigger>
                  <TabsTrigger value="journey">
                    Critical user journey
                  </TabsTrigger>
                  <TabsTrigger value="latency">Latency SLO</TabsTrigger>
                  <TabsTrigger value="intent">
                    Goal (intent)
                  </TabsTrigger>
                </TabsList>
                <span>YAML</span>
              </div>
              <TabsContent value="slo">
                <div className="file-name">request-success.slo.yaml</div>
                <pre tabIndex={0} aria-label="Complete SLO YAML example">
                  <code>{slo}</code>
                </pre>
              </TabsContent>
              <TabsContent value="journey">
                <div className="file-name">checkout.journey.yaml</div>
                <pre
                  tabIndex={0}
                  aria-label="Complete critical user journey YAML example"
                >
                  <code>{journey}</code>
                </pre>
              </TabsContent>
              <TabsContent value="latency">
                <div className="file-name">request-latency.slo.yaml</div>
                <pre
                  tabIndex={0}
                  aria-label="Self-contained latency SLO YAML example"
                >
                  <code>{latency}</code>
                </pre>
              </TabsContent>
              <TabsContent value="intent">
                <div className="file-name">
                  api-success.intent.yaml
                </div>
                <pre
                  tabIndex={0}
                  aria-label="SLO goal (intent) YAML example"
                >
                  <code>{intent}</code>
                </pre>
              </TabsContent>
            </Tabs>
          </div>
        </section>
        <section
          className="container examples-section"
          id="examples"
          aria-labelledby="examples-title"
        >
          <div className="section-heading">
            <h2 id="examples-title">From a definition to a running SLO.</h2>
            <p>
              Use these examples from a source checkout or with the alpha
              binaries. Each workflow makes its prerequisites and changes
              explicit.
            </p>
          </div>
          <div className="examples-panel">
            <label className="example-picker" htmlFor="workflow-example">
              Choose a workflow
              <select
                id="workflow-example"
                value={exampleId}
                onChange={(e) => setExampleId(e.target.value)}
              >
                {[
                  {
                    name: "Try locally",
                    ids: ["playground", "ai-playground", "offline"],
                  },
                  {
                    name: "Connect and operate",
                    ids: ["connected", "metrics", "operator"],
                  },
                  {
                    name: "Build and automate",
                    ids: ["slo-templates", "sdk", "mcp", "intent-checks"],
                  },
                ].map((group) => (
                  <optgroup key={group.name} label={group.name}>
                    {group.ids
                      .map((id) => examples.find((item) => item.id === id))
                      .filter((item) => item !== undefined)
                      .map((item) => (
                        <option key={item.id} value={item.id}>
                          {item.label}
                        </option>
                      ))}
                  </optgroup>
                ))}
              </select>
            </label>
            {examples.map((example) => (
              <section
                key={example.id}
                hidden={exampleId !== example.id}
                aria-label={example.label}
                className="example-content"
              >
                <div className="example-description">
                  <h3>{example.title}</h3>
                  <p>{example.description}</p>
                  <p className="example-note">{example.note}</p>
                  <a
                    className="text-link"
                    href={documentationLink(example.path)}
                  >
                    {example.link} <ArrowRight size={15} aria-hidden="true" />
                  </a>
                </div>
                <div className="example-code">
                  <div className="example-toolbar">
                    <span>{example.language}</span>
                    <CopyCommand
                      text={example.code}
                      label={`Copy ${example.label} example`}
                    />
                  </div>
                  <pre tabIndex={0} aria-label={`${example.label} example`}>
                    <code>{example.code}</code>
                  </pre>
                </div>
              </section>
            ))}
          </div>
        </section>
        <section
          className="container agent-section"
          id="agents"
          aria-labelledby="agent-title"
        >
          <div className="section-heading">
            <h2 id="agent-title">
              Give agents context.
              <br />
              Keep changes reviewable.
            </h2>
            <p>
              The same resource contracts sit behind the UI, CLI, Go client and
              MCP adapter. Model assistance is optional; measurement and
              validation remain deterministic.
            </p>
          </div>
          <div className="agent-layout">
            <div className="agent-interfaces">
              <article>
                <Terminal size={21} aria-hidden="true" />
                <div>
                  <h3>A CLI that works offline</h3>
                  <p>
                    Validate definitions and generate rules without a server.
                    Use structured JSON output in scripts and deterministic
                    skills.
                  </p>
                </div>
              </article>
              <article>
                <Waypoints size={21} aria-hidden="true" />
                <div>
                  <h3>Tenant-scoped MCP tools</h3>
                  <p>
                    Expose tenant-scoped tools over stdio or opt-in HTTP with
                    OAuth. Keep server contexts and write permissions explicit.
                  </p>
                </div>
              </article>
              <article>
                <Check size={21} aria-hidden="true" />
                <div>
                  <h3>Reviewed intent proposals</h3>
                  <p>
                    Inspect the source, eligible population, success criteria
                    and historical evidence. Applying an intent change set
                    requires owner or editor approval of its exact digest; the
                    agent role cannot approve or apply it.
                  </p>
                </div>
              </article>
              <a className="text-link" href={"./docs/intent-evaluations/"}>
                Evaluate intent workflows and model support{" "}
                <ArrowRight size={15} aria-hidden="true" />
              </a>
              <a className="text-link" href={"./docs/intent-governance/"}>
                Recommendation review, audit and rollback{" "}
                <ArrowRight size={15} aria-hidden="true" />
              </a>
              <a
                className="text-link"
                href={"./docs/cli/#scripts-and-llm-skills"}
              >
                Read the CLI and skills guide{" "}
                <ArrowRight size={15} aria-hidden="true" />
              </a>
            </div>
            <WorkflowIllustration />
          </div>
        </section>
        <section
          className="quickstart-section"
          id="quickstart"
          aria-labelledby="start-title"
        >
          <div className="container quickstart-layout">
            <div>
              <h2 id="start-title">
                Start with a
                <br />
                working workspace.
              </h2>
              <p>
                Run Nines with synthetic SLOs and critical user journeys. No
                provider credentials or model account needed.
              </p>
              <p className="requirements">Requires Go 1.27.3 or later.</p>
            </div>
            <div className="quickstart-panel">
              <div className="quickstart-toolbar">
                <span>Terminal</span>
                <CopyCommand text={start} />
              </div>
              <pre tabIndex={0}>
                <code>{start}</code>
              </pre>
              <p>
                Open <code>http://127.0.0.1:8080</code> and sign in with{" "}
                <code>admin</code> / <code>admin</code>. Demo data resets on
                restart. Keep the demo on loopback.
              </p>
              <a
                className="text-link"
                href={`${repo}#run-a-persistent-workspace`}
              >
                Set up a persistent workspace{" "}
                <ArrowRight size={15} aria-hidden="true" />
              </a>
            </div>
          </div>
        </section>
        <section
          className="container release-section"
          aria-labelledby="release-title"
        >
          <div>
            <h2 id="release-title">Release status and scope.</h2>
            <p>
              Nines is a pre-release MVP under active development. Expect
              regular features and fixes. Core SLO and critical user journey
              management and the standalone Kubernetes operator are supported
              alpha paths. Intent workflows and session replay are experimental.
            </p>
          </div>
          <div className="release-links">
            <a href={`${repo}/releases`}>
              Alpha downloads <ArrowRight size={16} aria-hidden="true" />
            </a>
            <a href={"./docs/release/"}>
              Release checklist <ArrowRight size={16} aria-hidden="true" />
            </a>
            <a href={"./docs/operations/"}>
              Operational limits <ArrowRight size={16} aria-hidden="true" />
            </a>
            <a href={"./docs/experimental/"}>
              Experimental features <ArrowRight size={16} aria-hidden="true" />
            </a>
          </div>
        </section>
      </main>
      <footer className="site-footer">
        <div className="container footer-row">
          <a className="brand" href="#" aria-label="Back to top">
            <img src={logo} width="106" height="30" alt="Nines" />
          </a>
          <p>
            Inspired by Sloth and Pyrra. Built around critical user journeys.
          </p>
          <nav aria-label="Footer navigation">
            <a href={repo}>Source</a>
            <a href={`${repo}/blob/main/LICENSE`}>Apache-2.0</a>
            <a href={`${repo}/blob/main/SECURITY.md`}>Security</a>
            <a href="https://github.com/slok/sloth">Sloth</a>
            <a href="https://github.com/pyrra-dev/pyrra">Pyrra</a>
            <a href="./third-party-notices.txt">Notices</a>
          </nav>
        </div>
      </footer>
    </Theme>
  );
}
