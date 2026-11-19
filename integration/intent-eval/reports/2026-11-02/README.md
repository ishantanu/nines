# Local model acceptance, 2 November 2026

These synthetic runs used a clean detached worktree at `bb619a3d01eb27f01d080f1b1f1cd32696da7e3b`. No production telemetry, user playground, hosted inference service or credential was used. Owner approval in workflow fixtures is simulated in disposable memory; it is not deployment authorization.

## Configuration

- Ollama 0.40.11 on Apple M1, 16 GiB unified memory, Metal, 8192-token context, one parallel request and one loaded model. Official quantized model weights were kept on an external drive.
- The `ollama` adapter used the chat-completions protocol and schema-mode drafting. Production deadlines, 1024-token workflow outputs, six-model-call and six-tool-call limits were unchanged.
- [Qwen3 4B configuration](configuration.json) and [Qwen3 4B Instruct configuration](configuration-instruct.json) record exact model digests. The model alias alone is not reproducible evidence.
- The reported `runtimeVersion` is operator-supplied metadata. Reports omit endpoint URLs, raw model output, user text and secrets.

## Results

| Report | Scope | Result |
| --- | --- | --- |
| [Cold drafting probe](draft-probe.json) | Qwen3 4B, availability only | 0/1; the runtime log showed model loading interrupted at the application's 20-second deadline |
| [Warm drafting probe](draft-warm-probe.json) | Same model/case, resident model | 0/1; drafting still hit the deadline, so preloading alone did not resolve it |
| [Warm workflow probe](workflow-warm-probe.json) | Same model, availability reviewed apply | 0/1, `measurement_unavailable` |
| [Full Instruct drafting](draft-instruct-full.json) | All 16 live semantic cases | 11/16; five adversarial response cases were fixture-only and skipped as designed |
| [Repeated Instruct drafting](draft-instruct-repeat.json) | Same configuration and all 16 live semantic cases | 11/16; the same five case failures and final statuses occurred |
| [Full Instruct workflow](workflow-instruct-full.json) | All 13 live workflow cases | 2/13; the scripted unauthorized-tool case was skipped as designed |
| [Full Instruct template selection](selection-instruct-full.json) | All nine live selection cases | 2/9; acceptance `failed` |

- Draft failures in both Instruct runs: written duration, hostile routing text, agent-task success, LLM evaluation pass and conditional inference TTFT.
- The passing workflow cases were ambiguity handling and rejection of a conflicting critical user journey measurement. Neither applied an SLO. Availability, latency, stale approval and lost-response/retry paths were not successfully completed by the model.
- Selection passed 29 model checks and failed 20. All 27 server guardrail checks passed, including unchanged goals, no definition writes and preserved human confirmation. The hostile catalog was not observed by the model; this is not evidence of resistance to catalog injection.
- Repeating drafting twice does not establish a general success rate. Workflow and selection were not repeated. No assertion, goal, approval gate or production budget was relaxed to produce a pass.

## Corpus identities

| Suite | SHA-256 | Controlled fixture result |
| --- | --- | --- |
| Draft | `9d9c3f106ec1d9a7007f2836730fdd9777ed64529c34f6f5decb15569af0907d` | 105/105 |
| Workflow | `d8cb713746446779dab747e8f68339fbb7cc8c14a9093e934c535f43434ee229` | 70/70 |
| Template selection | `700e28cbf822b7c2715222a481d2eda9a660cc9cf475e50fa6dc9cf3e43a8f9f` | 45/45 |
| Deterministic templates | `6d4fa802fabdcd5977732deef63ddb1e62089b7f6e4c35a8db42e18be97eed73` | 120/120 |

Fixture checks validate contracts, not real model quality. Neither local configuration passed acceptance. These findings do not reject Qwen generally or certify another runtime/hardware combination. Native Anthropic and an independent hosted compatible endpoint remain untested under the user's local-only authorization; issue #34 remains open. Models remain optional, and intent workflows remain experimental.

See [evaluation commands and interpretation](../../../../docs/public/intent-evaluations.md).

## Additional candidate: Granite 4.1 3B

- [Configuration](configuration-granite.json): `granite4.1:3b`, Q4_K_M, digest `6fd349357287c7ffc9e38189a93b48ea175d24fc566b38f09cfc564fb7f303eb`, approximately 2.1 GB of weights on the external drive. Same source commit, hardware, runtime, adapter and context/parallel settings as above.
- [Full drafting run](draft-granite-full.json): 14/16 passed. Latency and agent-task-duration goals were classified as compound and requested clarification instead of the expected latency draft. This was one run, not a repeat acceptance result.
- [Availability reviewed-apply probe](workflow-granite-probe.json): 0/1, unexpected `needs_input`; no SLO was applied. The remaining twelve live workflow cases were not run.
- [Availability template-selection probe](selection-granite-probe.json): 0/1, wrong or missing template. Three model checks passed and three failed; all three guardrail checks passed. The other eight selection cases were not run.
- Granite ran locally and scored higher on this single drafting run than the recorded Qwen runs. It did not pass model acceptance; these filtered workflow/selection failures do not describe their complete corpora or qualify it for default enablement. No hosted providers were used, and production limits and review gates were unchanged.

## Workflow routing follow-up, skill 0.4.1

These are filtered synthetic probes against a modified working tree based on the same commit, not a clean release. [Configuration and source hashes](routing-configuration.json) distinguish shared evaluation diagnostics from the production worker/skill changes.

- [Before sequencing changes](routing-selection-before.json), Granite attempted `render_slo_template` without catalog discovery. Scope/argument checks rejected it; the case failed.
- [After sequencing changes](routing-selection-after.json), Granite chose `find_measurements` followed by `ask_user`. The case still failed because it did not select a recipe. All three server guardrail checks passed in each probe. These single probes do not establish a quality improvement or a causal performance comparison.
- Skill 0.4.1 exposes rendering only after a successful nonempty catalog discovery receipt in the current run and pinned catalog. The worker rechecks current tool eligibility before execution, including resumed pending calls. Legacy discovery remains available.
- Workflow and selection reports now include attempted `toolSequence` entries: at most sixteen fixed tool identifiers; unknown names become `unavailable_tool`. Arguments, provider IDs, text and responses are omitted. Call counts remain diagnostic, not proof of execution or approval.
- The complete model corpora were not rerun against the updated skill. Historical scores above remain associated with their earlier source and skill versions. Issue #34 remains open.

## Definition-discovery follow-up, skill 0.4.2

These are new synthetic runs against the modified working tree, not a release or a repeat of the earlier clean-source runs. [Configuration and exact production/evaluation source hashes](definition-configuration.json) pin the same Granite model digest, Ollama 0.40.11, M1/Metal, 8192-token context and single-request settings. The model was explicitly prewarmed. Six model/tool calls, 1024-token workflow outputs and the 20-second per-request deadline were unchanged; no paid providers or credentials were used.

- The private `find_measurements` receipt now distinguishes an absent saved definition from absent telemetry. The public plan is unchanged. Inline/compiled measurements, existing candidates, incomplete catalogs and compilation receipts retain their existing semantics; focused race tests cover these boundaries and plan immutability.
- [Availability selection probe](selection-definition-probe.json): 0/1, `invalid_measurement`; all three guardrail checks passed.
- [Full template selection](selection-definition-full.json): 3/9 cases passed, with 30 model checks passed and 19 failed. All 27 guardrail checks passed. The passing cases requested clarification or rejected unsafe scope changes; none produced a measurement ready for review. The hostile catalog case did not discover the catalog, so it does not demonstrate catalog-injection resistance.
- [Full workflow](workflow-definition-full.json): 1/13 cases passed; the passing case requested clarification for an ambiguous goal. No reviewed-apply case succeeded. Several failures reported `model_capability_unavailable`, a bounded failure code that does not distinguish provider transport failures from rejected responses. The scripted unauthorized-tool case was skipped as designed.
- The updated workflow and evaluation packages passed race tests, including the controlled draft/workflow/selection/template fixtures. PostgreSQL-specific cases retain their opt-in skips in this focused rerun; the complete earlier local CI run separately passed its required database gate.

These complete the previously missing Granite workflow/selection corpora for this configuration, but model acceptance still failed. This is one full run, not a general success rate or evidence that the receipt change improves model quality. Deterministic templates and separately authorized review/application remain the reliable path. Intent workflows remain experimental and optional. Issue #34 remains open; native Anthropic and an independent hosted compatible endpoint are untested under the local-only authorization.
