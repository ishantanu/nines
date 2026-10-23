# Intent workflow evaluations

The development harness exercises production provider adapters and deterministic intent validation with synthetic text, telemetry and disposable in-memory workspaces. It does not connect to your Nines server or deploy recording rules. The workflow suite explicitly tests reviewed writes inside its disposable workspace.

## Contract checks without a model

From the repository root:

```sh
make intent-eval                 # 75 drafting checks
make intent-workflow-eval        # 60 full workflow checks

# Save private reports in new files:
go run ./integration/intent-eval --out intent-evaluation-draft.json
go run ./integration/intent-eval --suite workflow \
  --out intent-evaluation-workflow.json

# Run one workflow scenario through all five adapter profiles:
go run ./integration/intent-eval --suite workflow \
  --case availability-reviewed-apply
```

Both suites ignore configured model endpoints unless `--live` is supplied. They start loopback HTTP fixtures with scripted responses and run five adapter profiles: native Anthropic, native Gemini, compatible chat completions, Ollama with schema output and vLLM with JSON output. `go test ./...` also exercises both suites in CI.

The drafting corpus contains 15 cases: availability, latency, written durations, missing target/window, calendar windows, conflicting targets, compound outcomes, percentiles and hostile routing instructions. Adversarial responses cover invented quantities, duplicate fields, discarded clarifications, null clarification items and unauthorized query fields. Drafting must preserve the selected scope and leave the SLO store unchanged.

The workflow corpus contains 12 cases: availability and latency through reviewed apply; ambiguity; missing populations and metrics; sparse and zero traffic; conflicting critical user journey references; stale binding approvals; changed source routing; a lost apply response followed by readback/retry; and an unauthorized provider-routing tool call. The harness runs the actual bounded worker, simulates an explicit owner selecting a binding and approving a digest, and checks that the agent role cannot approve or apply. It checks source, binding, intent, manifest and time-window provenance, no preparation writes, atomic apply receipts and retry behavior. The source-routing fixture simulates a changed Mimir dataset fingerprint; it does not contact a real Mimir deployment.

Scripted responses establish adapter and safety contracts. They do not establish real model quality or prove how a model responds to hostile instructions.

## Evaluate a configured model

Live evaluation requires `--live` and explicit `INTENT_MODEL_PROVIDER` and `INTENT_MODEL_MODEL`, even when the application could otherwise infer a provider from an API key. Set credentials using the provider's existing environment variable or your secret manager. For example, after setting `ANTHROPIC_API_KEY`:

```sh
INTENT_MODEL_PROVIDER=anthropic \
INTENT_MODEL_MODEL=your-anthropic-model-id \
go run ./integration/intent-eval --live --out intent-evaluation-anthropic.json

INTENT_MODEL_PROVIDER=anthropic \
INTENT_MODEL_MODEL=your-anthropic-model-id \
go run ./integration/intent-eval --suite workflow --live \
  --timeout 10m --out intent-evaluation-anthropic-workflow.json
```

Anthropic and Gemini use their native APIs. Gemini uses `INTENT_MODEL_PROVIDER=gemini` and `GEMINI_API_KEY`; other supported providers retain their credential variables. There is no mandatory provider SDK or hosted OpenAI service.

For Qwen already served by Ollama:

```sh
INTENT_MODEL_PROVIDER=ollama \
INTENT_MODEL_MODEL=your-installed-qwen-model \
go run ./integration/intent-eval --live \
  --runtime-version=your-ollama-version \
  --out intent-evaluation-ollama.json

# Probe native tool calling before running the complete workflow corpus:
INTENT_MODEL_PROVIDER=ollama \
INTENT_MODEL_MODEL=your-installed-qwen-model \
go run ./integration/intent-eval --suite workflow \
  --case availability-reviewed-apply --live \
  --runtime-version=your-ollama-version \
  --out intent-evaluation-ollama-workflow-probe.json
```

Ollama defaults to `http://127.0.0.1:11434/v1/chat/completions`. For vLLM:

```sh
INTENT_MODEL_PROVIDER=vllm \
INTENT_MODEL_MODEL=your-served-model-id \
INTENT_MODEL_OUTPUT_MODE=json \
go run ./integration/intent-eval --suite workflow --live \
  --runtime-version=your-vllm-version \
  --out intent-evaluation-vllm.json
```

vLLM defaults to `http://127.0.0.1:8000/v1/chat/completions`. Use `schema` output if supported by your runtime. Full workflows require native tool calling in addition to structured drafting. A model that can emit draft JSON may still be unsuitable for agentic workflows. The harness does not download models, start runtimes or discover versions.

For another compatible endpoint, use `INTENT_MODEL_PROVIDER=custom`, an explicit model, `INTENT_MODEL_ENDPOINT` ending in `/chat/completions`, and optionally `INTENT_MODEL_API_KEY_ENV` naming your existing key variable. `INTENT_MODEL_API_STYLE` selects the supported protocol and `INTENT_MODEL_OUTPUT_MODE` selects `schema` or `json`. Non-loopback endpoints require HTTPS. Provider selection does not trigger fallback to another service.

The live drafting suite runs ten semantic cases and excludes five scripted-response cases. The live workflow suite runs eleven cases and excludes the scripted unauthorized-tool response. `--case` selects one workflow case and is rejected for the drafting suite. Requests for separate cases start at least five seconds apart; production request timeouts and tool/token budgets still apply. The default total timeout is five minutes, configurable up to fifteen minutes. Hosted calls may incur charges. Cancellation stops subsequent calls; requests already accepted by a provider may still be billed.

## Evidence, progress and recovery

The authoring UI shows the selected source, metric, SLO-owned measurement digest (or explicit legacy binding revision), observation point, measurement population, success condition, discovery interval and assumptions. Expand the fingerprint details to inspect source, discovery and binding identities. Backtest evidence includes its sampled interval, resolution and definition provenance. Source changes during compilation or backtesting invalidate the result. A changed source fingerprint also invalidates cached measurement health until it is rechecked.

Provider stream progress is persisted as a phase and bounded byte count under the worker lease. The browser reads it through the existing conversation polling endpoint. Raw model tokens, reasoning and private tool correlations are not exposed. Progress updates do not change the conversation revision used for reviewed actions. Cancellation fences late progress, and persisted progress survives a repository restart.

| Acceptance boundary | Automated coverage |
| --- | --- |
| Five provider transport profiles | `internal/intenteval`, `internal/intentmodel/review_test.go` |
| Tenant scope, role enforcement and strict requests | `internal/httpapi/intent_conversations_test.go` |
| Ambiguity, binding selection, cancellation and stale workers | `internal/intentflow/flow_test.go` |
| Persistent progress and late callback fencing | `internal/intentflow/progress_test.go` |
| Reviewed atomic apply, stale approval rejection and lost-response recovery | `internal/intentflow/changeset_test.go`, workflow evaluation suite |
| Backtest provenance, sparse/no traffic and source drift | `internal/service/intent_followups_test.go`, workflow evaluation suite |
| Credential echo rejection and safe provider failures | `internal/intentmodel/credentials_test.go`, `internal/intentflow/credentials_test.go` |
| Visible measurement evidence and progress | `web/src/components/intent-evidence.test.tsx` |

Run persistent acceptance tests with a disposable PostgreSQL database:

```sh
# Set TEST_DATABASE_URL privately before running these commands.
make test-integration
```

This target rejects skipped required database tests. Memory-backed contract results alone do not prove restart recovery. Existing tests exercise repository reopen, lease recovery, cancellation, and recovery when resources commit but the conversation receipt update fails.

Known configured model keys are rejected if echoed into drafting responses, tool calls or conversation inputs. This is a narrow guard, not general secret detection or anonymization of arbitrary user text. Provider error codes are allowlisted; raw provider errors do not enter the public transcript.

Atomic transaction failure rolls back that transaction. Existing definitions can be restored through an explicit, separately approved configuration rollback, with current revision/digest checks and an immutable transaction receipt. Newly created resources have no previous definition and show a blocked rollback; resource retirement is separate and not implemented. External rules follow the owner-configured automatic or manual source policy after the restored definition commits. There is no automatic error-budget-triggered rollback, and restoring configuration cannot undo consumed error budget. See [recommendation review, audit and rollback](intent-governance.md).

## Reports and interpretation

Reports use `apiVersion: nines.dev/v1alpha1` and `kind: IntentEvaluationReport`. They contain suite name and SHA-256, mode, timestamp, completion state, case/provider/model identifiers, observed statuses, fixed failure codes and elapsed times. Workflow failures also include a safe `workflowError` code when available. `--runtime-version` is optional operator-supplied metadata. Keep credentials and private text out of identifiers, runtime versions and paths.

Raw responses, clarification text, endpoint URLs and credential values are omitted. Root-level `intent-evaluation*.json` reports are Git-ignored. Output files are created with mode `0600` before requests begin; existing files and symlinks are refused. Invalid configuration or cancellation can leave an incomplete report. Exit status is nonzero for incomplete runs, failed checks or report-write errors. Inspect `complete`, `failed`, `selectedCase` and `skipped` together. A single-case pass is not full-suite acceptance. A selection containing only skipped scripted cases returns a nonzero exit status.

Record `git rev-parse HEAD`, whether the tree is modified, exact model/version, runtime, protocol/output settings and suite digest alongside each report. A moving model alias is not a reproducible model version. Repeat live runs to assess variability. The harness does not yet measure token usage, cost or performance distributions.

## Local model findings, 22 October 2026

An isolated Ollama 0.40.8 runtime on Apple M1 was tested with `qwen3:0.6b`, model digest `7df6b6e09427a769808717c0a93cadc4ae99ed4eb8bf5ca557c90846becea435`. These were synthetic runs against a modified working tree based on `e9ed9efcafdbb7c71842e484496f151eaea619e3`; they are not a release certification.

| Run | Observed result |
| --- | --- |
| Drafting before adding intent examples | 6/10 passed |
| Drafting with intent examples | 7/10 passed; latency, written-duration and hostile-routing descriptions requested clarification rather than producing the expected draft |
| Compatible-endpoint drafting against the same Qwen backend | 7/10 passed; this is route coverage, not a separate model/provider evaluation |
| Full native workflow corpus | 0/11 passed; the model failed before reviewed apply |
| Availability workflow capability probe after preserving safe error codes | 0/1 passed, `model_capability_unavailable` |

The small Qwen model did not return usable native tool calls under this contract. These failures remain failures; assertions were not relaxed to count clarification or missing tools as successful authoring. No model-created SLO was applied by these failed workflow runs. Results do not establish that larger Qwen models, another serving configuration or another provider behave the same way.

Real Anthropic evaluation remains unverified because no credentials were available. Fixture coverage for Anthropic is not a substitute for that run. The [intent hardening issue](https://github.com/ishantanu/nines/issues/20) is closed for the current alpha scope with this caveat. [Real-provider acceptance is tracked separately in #34](https://github.com/ishantanu/nines/issues/34); closing the hardening issue does not certify model quality or complete its original provider acceptance criterion. Intent workflows remain experimental.
