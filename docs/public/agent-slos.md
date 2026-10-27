# Agent, LLM and inference SLOs

Use the same event-weighted SLO engine for AI workloads as for conventional services.

- Start with the [SLI menu and templates](slo-templates.md), then connect the measurement to the relevant [critical user journey](critical-user-journeys.md).
- A fast inference call or successful tool attempt does not establish that a user's task succeeded.
- Try the [AI workload playground](deployment/ai-playground.md) to see real exported metrics, template-generated SLOs and automatic recording rules with controlled synthetic failures. No model account is required.

## Choose the observation point

| Objective              | Eligible population                               | Good events                                    | Important limit                                                 |
| ---------------------- | ------------------------------------------------- | ---------------------------------------------- | --------------------------------------------------------------- |
| Agent task completion  | One terminal outcome per logical task             | Declared user outcome achieved                 | Include exhausted retries, failures, cancellations and timeouts |
| Tool-call success      | Every eligible attempt                            | Tool's domain contract satisfied               | Attempts and tasks have different denominators                  |
| Evaluation pass rate   | Selected evaluation cohort with terminal verdicts | Versioned rubric passes                        | Missing evaluations need a separate completion-coverage signal  |
| Budget compliance      | One budget verdict per logical task               | Declared cost/token/step limits met            | Instrument accounting explicitly; unknown cost cannot pass      |
| LLM response success   | Logical generation requests                       | Usable response under the application contract | Specify refusals, truncation, retries and cancellation          |
| Inference availability | Serving requests at a declared boundary           | Successful result                              | Observe rejected admission and failed requests at that boundary |
| Inference latency      | Requests observed in the duration histogram       | Duration at most the threshold                 | Account for requests that never complete separately             |
| Time to first token    | Streams reaching their first token                | First-token delay at most the threshold        | Conditional latency; pair with request availability             |

- Use `type: availability` for any classified event-success ratio, including quality verdicts. The field is the engine's event-ratio measurement type; it does not assert that model quality equals infrastructure availability.

## Outcome evaluation example

The [complete agent evaluation example](../../examples/agent-evaluation.slo.yaml) uses an application-owned counter:

```yaml
measurement:
  type: availability
  metric: agent_evaluations_total
  labels:
    service: support-agent
    environment: production
    rubric: support-v1
    evaluator: rules-v1
  successLabels:
    verdict: pass
  countFrom: counter
  scrapeIntervalSeconds: 15
  observationPoint: Terminal verdict for every task in the selected evaluation cohort; unknown verdicts stay in the population. Evaluation completion coverage is measured separately.
```

- Instrument each selected task once with `verdict=pass`, `fail` or `unknown`.
  - For 97 passes, two failures and one unknown, attainment is **97 / 100 = 97%**.
  - The denominator is every matching verdict, not just passes.
  - This intentionally counts an emitted unknown as not passing the declared evaluation contract; it does not claim the underlying answer is incorrect.
  - If no verdict was emitted, the counter cannot see that task.
  - Track selected versus completed evaluations separately, and treat missing cohort coverage as insufficient evidence.
  - Zero observed traffic yields no attainment value.
- Pin the rubric, evaluator and sampling policy.
  - Prefer reproducible checks where they represent the intended outcome.
  - Model judges require calibration against reviewed examples and checks for bias and drift; a judge's score is not independent proof of correctness.
  - Do not let the proposing agent rewrite the rubric, exclude failures, select only favorable evaluations, approve itself or apply its own recommendation.
- Bound labels such as service, environment, model family and evaluator version.
  - Keep prompts, answers, user IDs, task IDs and unbounded tool arguments out of metric labels.
  - Retain sensitive evaluation artifacts in your separately controlled evidence system, not the SLO manifest.

## Latency and instrumentation

- [Inference time-to-first-token YAML](../../examples/inference-ttft.slo.yaml) illustrates a classic histogram family in seconds.
  - Nines compiles its exact threshold bucket as good events and its count series as total events.
  - The threshold must exist for every selected histogram population, alongside `+Inf` and `_count`; missing boundaries fail verification.
  - Percentile gauges, summaries and native histograms are not accepted by this measurement compiler. See [latency SLOs](latency-slos.md) and the [Prometheus histogram guidance](https://prometheus.io/docs/practices/histograms/).
- Templates deliberately use illustrative metric names. [OpenTelemetry GenAI conventions](https://github.com/open-telemetry/semantic-conventions-genai) describe GenAI signals, but an instrument name does not guarantee the metric names, units or labels exported into your Prometheus/Mimir dataset.
  - Inspect your actual telemetry connection and confirm its schema.
  - Nines does not automatically instrument your agent or invoke a model judge.

## Review and operate

- Verify the measurement, backtest available history, inspect freshness, traffic and cohort coverage, then review the exact SLO change.
  - A template's target is not a production recommendation.
  - For committed SLOs, add burn-rate policies with suitable event minimums and an owner [error-budget policy](error-budget-policy.md).
- Agent suggestions use the existing [approval, audit, observation and rollback workflow](intent-governance.md). Template rendering adds no write authority. SLO updates remain revision-fenced; rollback restores reviewed configuration, never consumed error budget.
- Two evaluation activities remain distinct: testing **Nines' drafting/preparation agent**, and measuring **your application's agent outcomes**. [The evaluation harness](intent-evaluations.md) reports deterministic measurement checks separately from adapter fixtures and live model runs. A green fixture suite does not establish live-model quality or production AI safety.
