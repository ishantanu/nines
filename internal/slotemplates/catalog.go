// Package slotemplates supplies deterministic, provider-neutral SLO starting points.
// It does not query telemetry, call a model, save resources or deploy rules.
package slotemplates

import (
	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func categories() []v1.SLOTemplateCategory {
	return []v1.SLOTemplateCategory{{ID: "request-serving", Title: "Request serving"}, {ID: "pipeline", Title: "Pipelines"}, {ID: "storage", Title: "Storage"}, {ID: "agent", Title: "Agents"}, {ID: "llm", Title: "LLM applications"}, {ID: "inference", Title: "Inference serving"}}
}
func catalog() []v1.SLOTemplate {
	return []v1.SLOTemplate{
		event("request-availability", "request-serving", "availability", "Request availability", "Completed eligible server requests", "Requests classified as successful", "http_requests_total", "status", "200", "api", "Includes server completions only; instrument edge failures separately. Define success codes and retry treatment."),
		latency("request-latency", "request-serving", "latency", "Request latency", "http_request_duration_seconds", "api", .3, "Completed eligible server requests, including error completions when instrumented. Pair with availability."),
		event("pipeline-correctness", "pipeline", "correctness", "Pipeline correctness", "All eligible records receiving a validation verdict", "Records passing the versioned validation contract", "pipeline_records_total", "verdict", "pass", "pipeline", "Count invalid and unevaluable records in the population; do not emit only successful records."),
		latency("pipeline-freshness", "pipeline", "freshness", "Pipeline freshness", "pipeline_record_age_seconds", "pipeline", 300, "Record age at delivery relative to event time, in seconds. Missing deliveries are absent from this histogram; pair with delivery coverage."),
		event("storage-operation-success", "storage", "availability", "Storage operation success", "All eligible storage operation completions", "Operations acknowledged successfully", "storage_operations_total", "outcome", "success", "storage", "An acknowledgement measures operation success, not long-term durability. Verify durability through separate integrity checks."),
		event("storage-integrity", "storage", "correctness", "Storage integrity checks", "All scheduled object integrity checks reaching a terminal verdict", "Checks confirming the declared integrity contract", "storage_integrity_checks_total", "verdict", "pass", "storage", "Emit failures and unknown verdicts. Check completion coverage separately; sampled checks do not establish universal durability."),
		event("agent-task-completion", "agent", "task-success", "Agent task completion", "One terminal outcome per eligible logical task, after internal retries", "Tasks satisfying the declared user outcome", "agent_tasks_total", "outcome", "success", "agent", "Emit failed, cancelled and timed-out task outcomes. Tool success alone does not establish task completion."),
		event("agent-tool-success", "agent", "tool-success", "Agent tool-call success", "Every eligible tool attempt, including retries", "Attempts satisfying the tool result contract", "agent_tool_calls_total", "outcome", "success", "agent", "A transport 200 is insufficient when the tool result contains a domain error. This measures attempts, not logical tasks."),
		event("agent-budget-compliance", "agent", "budget-compliance", "Agent task budget compliance", "One terminal budget verdict per eligible logical task", "Tasks within the declared cost, token and step limits", "agent_task_budget_total", "verdict", "pass", "agent", "Define limits and accounting version in instrumentation; emit unknown costs as unknown verdicts, never pass. No universal cost conversion is assumed."),
		event("agent-evaluation-pass", "agent", "evaluation-quality", "Agent outcome evaluation", "All tasks selected for the declared evaluation cohort with terminal verdicts", "Tasks passing the versioned outcome rubric", "agent_evaluations_total", "verdict", "pass", "agent", "Pin rubric, evaluator and sampling policy using bounded labels. Failed and unevaluable verdicts stay in the denominator. Track evaluation completion coverage separately."),
		event("llm-response-success", "llm", "availability", "LLM response success", "All eligible logical generation requests with a terminal outcome", "Requests returning a usable response under the application contract", "llm_requests_total", "outcome", "success", "llm", "Define refusal, truncation, cancellation and retry semantics explicitly. A provider HTTP success is not necessarily usable output."),
		event("llm-evaluation-pass", "llm", "evaluation-quality", "LLM output evaluation", "All outputs selected for the declared evaluation cohort with terminal verdicts", "Outputs passing the versioned domain rubric", "llm_evaluations_total", "verdict", "pass", "llm", "Evaluator scores are evidence, not ground truth. Pin evaluator and rubric versions; unknown verdicts are not passes. Track selected-but-missing evaluations separately."),
		event("inference-availability", "inference", "availability", "Inference availability", "All eligible serving requests with a terminal outcome", "Requests returning a successful inference result", "inference_requests_total", "outcome", "success", "inference", "Count admission failures and timeouts at the chosen observation point; do not use successful-stream counts as the request denominator."),
		latency("inference-latency", "inference", "latency", "Inference request latency", "inference_request_duration_seconds", "inference", 2, "Eligible request duration at serving completion. Define error and timeout observations and pair with inference availability."),
		latency("inference-ttft", "inference", "time-to-first-token", "Time to first token", "inference_time_to_first_token_seconds", "inference", .5, "Streams reaching a first token only. Failed or never-started streams are excluded; pair with inference availability. This is a conditional latency SLO."),
	}
}
func base(id, category, sli, title, eligible, good, service, requirement string, m v1.MeasurementSpec) v1.SLOTemplate {
	m.Labels = map[string]string{"service": service}
	m.ScrapeIntervalSeconds = 15
	m.ObservationPoint = eligible + ". " + requirement
	t := v1.SLOTemplate{ID: id, Revision: 1, Category: category, SLI: sli, Title: title, Description: "Illustrative instrumentation; verify metric names, labels, units and event semantics against your telemetry.", EligibleEvents: eligible, GoodEvents: good, Requirements: []string{requirement, "Use bounded service, model and evaluator-version labels. Never label metrics with prompts, output content, user IDs or task IDs."}, Manifest: v1.NewSLO(id, "production", v1.SLOSpec{DisplayName: title, Service: service, Source: "primary", Lifecycle: v1.LifecycleExperimental, Target: .99, WindowSeconds: 28 * 86400, StepSeconds: 60, Measurement: &m})}
	t.Digest = model.DefinitionDigest(t)
	return t
}
func event(id, category, sli, title, eligible, good, metric, label, value, service, requirement string) v1.SLOTemplate {
	return base(id, category, sli, title, eligible, good, service, requirement, v1.MeasurementSpec{Type: "availability", Metric: metric, CountFrom: "counter", SuccessLabels: map[string]string{label: value}})
}
func latency(id, category, sli, title, metric, service string, threshold float64, eligible string) v1.SLOTemplate {
	return base(id, category, sli, title, eligible, "Observed duration at most the selected threshold", service, "Requires a classic histogram in seconds with an exact matching le bucket and count series; summaries and percentile gauges are unsupported.", v1.MeasurementSpec{Type: "latency", Metric: metric, ThresholdSeconds: &threshold})
}

// List returns the built-in catalog. Configured registries use the same contract.
func List(category string) (v1.SLOTemplateCatalog, error) { return Default().List(category) }
func Get(id string) (v1.SLOTemplate, error)               { return Default().Get(id) }
func Render(id string, values v1.TemplateValues) (v1.SLOTemplateDraft, error) {
	return Default().Render(id, values)
}
func renderTemplate(t v1.SLOTemplate, values v1.TemplateValues) (v1.SLOTemplateDraft, error) {
	var err error
	d := t.Manifest
	if values.Name != "" {
		d.Metadata.Name = values.Name
	}
	if values.Project != "" {
		d.Metadata.Project = values.Project
	}
	if values.Service != "" {
		d.Spec.Service = values.Service
		if _, ok := d.Spec.Measurement.Labels["service"]; ok && t.Extension == nil {
			d.Spec.Measurement.Labels["service"] = values.Service
		}
	}
	if values.Source != "" {
		d.Spec.Source = values.Source
	}
	if values.Measurement != nil {
		d.Spec.Measurement = values.Measurement
	}
	if values.Target != nil {
		d.Spec.Target = *values.Target
	}
	if values.WindowSeconds != nil {
		d.Spec.WindowSeconds = *values.WindowSeconds
	}
	if values.StepSeconds != nil {
		d.Spec.StepSeconds = *values.StepSeconds
	}
	if err = model.SLODefinition(d).Validate(); err != nil {
		return v1.SLOTemplateDraft{}, err
	}
	body, err := yaml.Marshal(d)
	if err != nil {
		return v1.SLOTemplateDraft{}, err
	}
	return v1.SLOTemplateDraft{Extension: t.Extension, APIVersion: v1.APIVersion, Kind: "SLOTemplateDraft", TemplateID: t.ID, TemplateRevision: t.Revision, TemplateDigest: t.Digest, ManifestDigest: model.DefinitionDigest(d), Manifest: d, YAML: string(body), Warnings: append([]string{"Example metrics and numerical defaults are starting points, not observed evidence or recommended production commitments.", "Experimental SLO: review the target, window and measurement before committing to alerts.", "Telemetry is unverified. Verify the complete measurement against the intended tenant connection before planning and applying. No resources or rules were changed."}, t.Requirements...)}, nil
}
