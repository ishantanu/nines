# AI workload playground

Run synthetic agent, LLM and inference measurements through Prometheus, Nines collection and automatic recording rules.

- This is an optional addition to the [local playground](playground.md).
- It requires Docker Compose, uses a small Go workload and needs no model account, API key, GPU or telemetry provider account.
- It does not execute an agent or evaluate real model outputs.

## Start and sign in

The launcher creates private local credentials on first use, or uses your existing `.env` / `POSTGRES_PASSWORD`:

```sh
./playground up --ai

./playground logs
```

- Open [Nines on localhost:18080](http://127.0.0.1:18080) and sign in with `admin` / `admin`, or the password configured when your playground database was initialized.
  - Select **Playground Prometheus**, then filter SLOs by project **ai-playground**.
  - Open **Critical user journeys → Resolve a support request with an agent** to follow its SLO links.
- The `ai-setup` service discovers only the authorized playground workspaces, renders nine built-in templates, adds one explicit evaluation-coverage SLO, verifies the actual exported telemetry and creates missing definitions. It also creates `ai-support-resolution`.
  - Repeated setup preserves existing definitions and user edits.
  - To rerun setup after a failure, run `./playground up` again; saved settings retain the AI profile.
- Every AI objective has a **60-second rolling window**, **10-second step**, **5-second scrape interval**, illustrative **99% target** and **experimental lifecycle**.
  - Recording rules and SLI values work; paging is suppressed.
  - These accelerated settings are for synthetic testing.
  - The existing checkout example retains its own window and burn alerts.
  - Startup does not overwrite an existing objective with these settings.

## What to inspect

| SLO                         | Measurement                                             | Degraded scenario                                                    |
| --------------------------- | ------------------------------------------------------- | -------------------------------------------------------------------- |
| `ai-agent-tasks`            | Terminal logical task outcomes after retries            | 70% successful; failures, cancellations and timeouts remain eligible |
| `ai-agent-tools`            | Every tool attempt                                      | 100% successful, despite failed logical tasks                        |
| `ai-agent-budget`           | One classified budget verdict per task                  | 90% pass, 10% unknown; unknown accounting does not pass              |
| `ai-agent-quality`          | Terminal evaluation verdicts                            | 90% pass among completed evaluations                                 |
| `ai-evaluation-coverage`    | Every selected evaluation after its deadline            | 60% completed, 40% missing                                           |
| `ai-llm-responses`          | Logical generation outcomes                             | 80% usable responses                                                 |
| `ai-llm-quality`            | Terminal LLM evaluation verdicts                        | 90% pass, with fail and unknown in the denominator                   |
| `ai-inference-availability` | All requests, including rejected and timed-out requests | 80% successful                                                       |
| `ai-inference-latency`      | All terminal durations at most two seconds              | 80% within threshold                                                 |
| `ai-inference-ttft`         | First-token observations at most 0.5 seconds            | 50% within threshold among streams reaching a first token            |

- Quality counters carry bounded `rubric="fixture-v1"` and `evaluator="deterministic-v1"` labels.
  - In the degraded scenario, 100 evaluations are selected per tick: 60 complete with 54 pass, three fail and three unknown verdicts; 40 emit no terminal verdict.
  - Thus quality is **54/60 = 90%**, while completion coverage is **60/100 = 60%**.
  - The coverage counter classifies every selected evaluation once after its deadline; missing work is not silently dropped from that denominator.
  - The matching `llm_evaluation_selections_total` metric is also exported with the LLM cohort labels; create a separate LLM coverage SLO from it when exploring that cohort.
  - These deterministic classifications demonstrate accounting, not a calibrated quality evaluator.
- Task success and tool success use different denominators.
  - Failed inference requests never enter TTFT; compare conditional TTFT with request availability.
  - Classic histogram boundaries include the exact 0.5- and two-second thresholds, `+Inf` and `_count`.
  - Prompts, output content, user IDs and task IDs are absent from labels.
- In an SLO, inspect **Performance** for observed SLI versus target and coverage, **Source measurements** for the metric and cohort, and **Rules** for recording output and deployment evidence.
  - Allow initial scrapes and collection to complete.
  - The window needs one minute of history; Mimir's ruler adds its configured evaluation delay.
  - A saved definition or written rule file alone does not establish measured attainment.

## Change the synthetic scenario

- The sample publishes metrics at [localhost:19093/metrics](http://127.0.0.1:19093/metrics).
  - Its unauthenticated controls are deliberately bound to loopback by Compose; do not publish this service on a shared network.
  - Change the host port with `NINES_PLAYGROUND_AI_PORT` if necessary.

```sh
curl -fsS -X POST 'http://127.0.0.1:19093/scenario?name=degraded'
# Wait for the rolling window and backend evaluation delay, then inspect SLOs.
curl -fsS -X POST 'http://127.0.0.1:19093/scenario?name=healthy'
```

- Supported scenarios are `healthy`, `degraded`, `task-failure`, `evaluation-gap`, `budget-unknown`, `inference-failure`, `slow-first-token` and `no-traffic`.
  - The named scenarios change only their stated signals; `degraded` combines the failure conditions in the table.
  - Counters stay monotonic across changes. `no-traffic` stops new observations while scrapes continue; earlier traffic remains in rolling windows until it ages out.
  - Once empty, attainment should be absent rather than 100%.

Query [Prometheus](http://127.0.0.1:19090/query):

```promql
nines:sli:ratio{nines_tenant="prometheus",nines_slo=~"ai-.*",window="60s"}
nines:slo_target:ratio{nines_tenant="prometheus",nines_slo=~"ai-.*"}
nines:slo_coverage:ratio{nines_tenant="prometheus",nines_slo=~"ai-.*",window="60s"}
```

- These recorded values appear in the query API and Rules page, not Prometheus's own `/metrics` endpoint.

## Verify degradation and recovery

```sh
./playground verify
```

- Allow several minutes.
  - The Go verifier checks actual telemetry compilation, background collection, owner-enabled automatic deployment, evaluator health and current-definition output markers.
  - It compares all ten recorded ratios with Nines' collected SLI values through healthy, degraded and recovered phases, and checks journey links.
  - It does not call manual rule apply.
  - Incomplete, stale, ambiguous or mismatched evidence exits nonzero.
  - An edited fixture definition may fail acceptance; use a separate fresh Compose project for reproducible tests rather than overwriting your edits.
- Verification temporarily controls the AI scenario and restores its previous value on success, error or normal cancellation.
  - A hard kill cannot run cleanup; reset to `healthy` explicitly if needed.
  - Do not run two verifiers or change scenarios concurrently.
  - AI SLOs remain experimental, so this test does not establish burn-alert firing or notification delivery; use the checkout verifier for its configured burn alerts.

## Add Mimir

Select Mimir and the AI workload together; the launcher remembers both:

```sh
./playground up --mimir --ai

./playground verify
```

- Setup automatically recognizes both authorized workspaces.
  - The verifier checks **Playground Prometheus** and **Playground Mimir** independently.
  - Prometheus remote-writes source metrics only; Mimir evaluates its own recording groups in organization and namespace `playground`. The launcher configures AI setup/verification for the selected workspaces.
- Use `./playground logs` and `./playground down`; saved settings retain the correct profiles. Shutdown preserves history.
  - Returning to the base playground mounts an empty AI target list; retained AI SLOs then lose fresh data as their old windows age out.
  - Remove unwanted definitions through the normal reviewed workflow.

## Inspect the generated YAML

```sh
# Offline; writes no resources and needs no running server.
go run ./integration/aiplayground render
```

- The command prints ten `nines.dev/v1alpha1` SLO documents and the linked critical user journey.
  - Nine SLOs use the same deterministic renderer as the CLI, portal and operator.
  - The separate completion-coverage definition names its own measurement explicitly.
  - No extension executable or provider SDK is loaded.
- This workload tests measuring **an application's AI outcomes**. Acceptance of **Nines' own model-assisted authoring agent** is a separate [evaluation suite](../intent-evaluations.md#template-selection-acceptance); healthy synthetic metrics do not certify a model or its recommendations.
