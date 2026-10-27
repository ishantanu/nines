# System One advice with Jev

Nines can use TypeSafe’s Jev model for two bounded judgments during intent preparation:

- Suggest a template category: request serving, pipeline, storage, agent, LLM application, inference serving or no match.
- Assess whether the description defines the eligible population, good events and observation point. These are three independent probabilities of the definitions being present.
- Both judgments share one native TypeSafe HTTP request.
  - Jev returns typed answers and probabilities, not a generated explanation or a tool call.
  - Nines retains its deterministic measurement compiler, telemetry checks, error-budget policy and approval flow.
  - Jev is optional and does not replace your configured drafting or review model.

## Configure a tenant

- Set a key in the **server process** environment or inject it from your secret manager. The adapter recognizes `JEV_API_KEY` and `TYPESAFE_API_KEY`; conflicting values require an explicit `apiKeyEnv`. Keys alone do not enable calls.

Add `systemOne` to the tenant in your administrator-owned `TENANTS_FILE`:

```yaml
tenants:
  - id: production
    name: Production
    access:
      - tokenEnv: NINES_PRODUCTION_OWNER_TOKEN
        role: owner
    source:
      kind: prometheus
      url: http://prometheus:9090
    systemOne:
      provider: typesafe
      model: jev-1.14.0
      apiKeyEnv: JEV_API_KEY
```

- Preserve your existing storage, access, connection and rule-destination configuration.
  - This example shows a complete tenant entry, not a replacement for your deployment setup.
  - Native processes read their inherited environment; Compose requires explicit environment or secret injection into the Nines container.
- The adapter uses `https://api.typesafe.ai/v1/systemone` with bearer authentication, normal TLS verification and no redirects.
  - A pinned `jev-x.y.z` version is required; the current default is `jev-1.14.0`. There is no OpenAI dependency.
  - Tenant configuration is explicit: an opted-in tenant does not enable another tenant. `-demo` disables external models, including Jev.

## Local Compose playground

- Use the optional `compose.playground.jev.yaml` override with either the base Prometheus playground or its Mimir override/profile.
  - The override requires `JEV_API_KEY` and selects a separate opt-in tenant file; default playground starts remain key-free. See [playground commands and UI walkthrough](deployment/playground.md#optional-jev-advice).
  - Adding a key alone does not enable a tenant route or consent to a request.
  - An existing paused admin policy must also be enabled deliberately.

## Use the workflow

- In **SLO intents → a saved conversation → Run options**, opt in to sending this run’s goal description and observation point to TypeSafe.
  - This consent is separate from consent for the model-directed agent.
  - You can use Jev with a typed goal and **Guided preparation**, without a generative model.
- Each opted-in run sends only its goal description (or latest user message when a typed goal is absent) and the inline measurement’s observation point if present.
  - Each field is bounded to 2,000 bytes.
  - No metric samples, provider URLs, selectors, labels, credentials, targets, windows, extension text or earlier conversation history are added to this request.
  - Free text can still contain confidential information: review it before consenting.
- The run shows category confidence, the three clarity probabilities and an expandable receipt. These values are **not SLI attainment or measurement-quality scores**.
  - The full category distribution is preserved; `none` is a valid answer.
  - Nines applies no automatic probability threshold: a category is a navigation hint for the workflow model, with the full template catalog still available.
  - The guided path displays the advice for the engineer.

API, CLI and MCP callers opt in explicitly on the existing run request:

```yaml
id: jev-definition-review
expectedVersion: 1 # Replace with the current conversation version.
mode: guided
modelConsent: false
systemOneConsent: true
```

For example, save this as `prepare.yaml` and run:

```sh
nines intent conversations run CONVERSATION_ID -f prepare.yaml
```

- The connected CLI uses your configured server, tenant and Nines credential. `JEV_API_KEY` belongs on the server, not the CLI host unless you also run the server there. `POST /api/intent-conversations/CONVERSATION_ID/runs` accepts the same fields as JSON. MCP’s `start_intent_preparation` uses them under `input`; caller authorization, enabled writes and tenant scope still apply.

## Administrative enablement and limits

- With a configured identity service, platform administrators can pause Jev and set per-tenant daily request limits in the [admin console](platform-administration.md#jev-controls).
  - These persisted UTC-day request reservations are separate from per-run consent and the generative model budget.
  - Provider keys, pinned model routes and tenant provisioning remain server-owned.

## Failure, audit and approval

- Nines reserves one external call before sending it.
  - Timeout, malformed output, provider failure, configuration drift or an interrupted call produces an unavailable receipt, never invented judgments. The normal preparation flow continues.
  - There are no adapter retries or automatic provider fallbacks; retry through a new, explicitly consented run.
  - The HTTP request has a 15-second timeout and inherits workflow cancellation.
- Each run records the configured provider, pinned model, question-contract digest, input and request digests, resolved model, token counts and typed probabilities.
  - Audit events record the request and result with an evidence digest.
  - A proposal freezes the latest run’s assessment when present; older assessments remain attached to their original run and must not be interpreted as assessments of a subsequently edited goal.
- Jev cannot choose metrics, modify targets or alert windows, confirm measurements, approve or apply proposals, deploy recording rules, or change rollback policy. A high probability is not evidence that a measurement exists, that an LLM outcome is correct, or that an SLO is healthy.

## Evaluate the judgments

- Normal tests use transport fixtures and exercise consent, tenant boundaries, budgets, unavailable results, cancellation, durable receipts and existing review gates. They do not prove model quality.

Run the opt-in synthetic live check with a server-side key already exported:

```sh
NINES_SYSTEM_ONE_LIVE_TEST=1 go test -count=1 -v ./internal/systemone -run '^TestLiveAcceptance$'
```

- This makes twelve bounded requests with synthetic definitions across the six categories, an ambiguous request, an instruction-injection request and four descriptions with undefined eligibility. It incurs provider usage.
  - The test compares the category with a labeled expectation and checks whether clarity probabilities fall on the expected side of 0.5; that is an evaluation convention, **not an application decision threshold**.
  - It logs safe assessment receipts, not keys or input descriptions.
  - The small suite is a smoke test, not production calibration or acceptance across all workloads and languages.
- TypeSafe documents the [native System One API](https://docs.typesafe.ai/api/system-one), [Choice](https://docs.typesafe.ai/primitives/choice), [Noul](https://docs.typesafe.ai/primitives/noul) and [confidence semantics](https://docs.typesafe.ai/confidence).

### Recorded smoke test: 25 October 2026

- Native `jev-1.14.0` matched all **12/12 category expectations** and all **12/12 population-definition expectations**.
  - Ten of twelve complete cases passed; two good-event-definition probabilities fell below the test’s 0.5 convention despite explicit HTTP success criteria. The corresponding observation-point checks passed.
  - This is a measured false-negative limitation, not a reason to block a definition or to relax deterministic validation.
- An initial eight-case run exposed descriptions that said “eligible” without specifying eligibility.
  - The revised suite retained those four descriptions as negative population cases and added fully specified positive populations. Across the final suite, **46/48 individual checks passed**.
  - Fixture transport tests and deterministic workflow guards are reported separately from these real model judgments. No automatic action threshold is enabled.
