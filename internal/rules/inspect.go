package rules

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/prometheus/promql/parser"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

// Inspect reads the configured destination and runtime API. It never deploys,
// reloads, or treats a successful write as proof of evaluation or notification.
func (m *Manager) Inspect(ctx context.Context, s model.SLO, now time.Time) (deployment, evaluation v1.IntegrationStage) {
	evaluation = ruleStage("unknown", "evaluator_unavailable", "External rule evaluation has not been verified.", "Check the configured evaluator URL, permissions and availability.")
	if _, ok := m.target(ConnectionID(s)); !ok {
		deployment = ruleStage("not_configured", "destination_not_configured", "No rule destination is configured.", "Configure an administrator-owned destination, then review and deploy rules. Collection does not deploy rules.")
		return
	}
	if target, ok := m.target(ConnectionID(s)); ok && target.Kind == "operator" {
		if m.External == nil {
			return ruleStage("unknown", "operator_unavailable", "Operator deployment is unavailable.", "Check the server publisher configuration."), evaluation
		}
		return m.External.Inspect(ctx, s, target, now)
	}
	if target, ok := m.target(ConnectionID(s)); ok && target.Kind == "dash0" {
		return m.inspectNative(ctx, s, target, now)
	}
	if target, ok := m.target(ConnectionID(s)); ok && target.Kind == "newrelic" {
		return m.inspectNewRelic(ctx, s, target, now)
	}
	if target, ok := m.target(ConnectionID(s)); ok && target.Kind == "datadog" {
		return m.inspectDatadog(ctx, s, target, now)
	}
	if target, ok := m.target(ConnectionID(s)); ok && target.Kind == "googlecloud" {
		return m.inspectGoogle(ctx, s, target, now)
	}
	p, err := m.PlanContext(ctx, s)
	if err != nil {
		deployment = ruleStage("unknown", "rule_plan_invalid", "The current definition cannot produce a deployable rule plan.", "Validate the definition and destination before requesting another plan.")
		return
	}
	deployment = m.inspectStored(ctx, s, p)
	evaluation = inspectRuntime(ctx, s, p, now)
	return
}

// InspectEvaluation verifies loaded content and evaluation without requiring a local
// rule file. Kubernetes operators persist their desired content as PrometheusRule.
func InspectEvaluation(ctx context.Context, s model.SLO, group Group, target Target, now time.Time) v1.IntegrationStage {
	return inspectRuntime(ctx, s, Plan{Group: group, Target: target}, now)
}

func ruleStage(state, code, message, action string) v1.IntegrationStage {
	return v1.IntegrationStage{State: state, Code: code, Message: message, NextAction: action}
}

func inspectionHTTP(component string, code int) v1.IntegrationStage {
	out := ruleStage("unknown", component+"_unavailable", "The configured "+component+" API could not be inspected.", "Check the configured URL, backend availability and gateway routing, then inspect again.")
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		out.Code = component + "_access_denied"
		out.NextAction = "Ask an administrator to verify the configured credentials, permissions and Mimir organization."
	case http.StatusNotFound:
		out.Code = component + "_api_unavailable"
		out.NextAction = "Verify the configured HTTP prefix and API enablement; a missing API does not prove that rules are absent."
	}
	return out
}

func (m *Manager) inspectStored(ctx context.Context, s model.SLO, p Plan) v1.IntegrationStage {
	var stored Group
	switch p.Target.Kind {
	case "prometheus":
		file := filepath.Join(p.Target.Directory, p.Target.Namespace+"-"+p.Group.Name+".yaml")
		data, err := readRuleFile(file)
		if os.IsNotExist(err) {
			return ruleStage("not_deployed", "rule_file_missing", "No rule file exists for this SLO.", "Review a current plan and deploy it to the configured shared directory.")
		}
		if err != nil {
			return ruleStage("blocked", "rule_file_unreadable", "The configured rule file cannot be safely read.", "Ask an administrator to check access, regular-file type, the 4 MiB size limit and single-document YAML. Symlinks are refused.")
		}
		stored = data.Groups[0]
	case "mimir":
		path := "/config/v1/rules/" + url.PathEscape(p.Target.Namespace) + "/" + url.PathEscape(p.Group.Name)
		code, body, err := rulerRequest(ctx, p.Target, http.MethodGet, path, nil)
		if err != nil {
			return inspectionHTTP("ruler", 0)
		}
		if code == http.StatusNotFound {
			return ruleStage("not_deployed", "rule_group_missing", "No rule group was found at the configured Mimir namespace route.", "Verify the HTTP prefix and namespace, then review and deploy a current plan.")
		}
		if code != http.StatusOK {
			return inspectionHTTP("ruler", code)
		}
		stored, err = decodeRuleGroup(body)
		if err != nil || stored.Name == "" {
			return ruleStage("unknown", "ruler_response_invalid", "The ruler returned an invalid rule group.", "Check the configured gateway and ruler API; provider response text is not exposed here.")
		}
	default:
		return ruleStage("not_configured", "destination_unsupported", "This destination type is not supported by the rule manager.", "Configure Prometheus rule files or a Mimir ruler destination.")
	}
	if !Owned(stored, m.Tenant, p.Target.Connection, s.ID) || !ownedUID(stored, m.OwnerUID) {
		return ruleStage("blocked", "rule_ownership_conflict", "The stored rules belong to another tenant, SLO or resource owner.", "Resolve ownership with an administrator. Do not overwrite foreign or operator-owned rules.")
	}
	if !sameGroup(stored, p.Group) {
		return ruleStage("out_of_date", "stored_rules_drifted", "The stored rules differ from the current definition.", "Review a new plan and its destination before explicitly deploying it.")
	}
	if p.Target.Kind == "prometheus" {
		return ruleStage("written", "stored_rules_match", "The current rule file is written to the configured directory.", "Verify the shared rule_files mount, reload acknowledgement and runtime evaluation separately.")
	}
	return ruleStage("stored", "stored_rules_match", "The configured ruler stores the current group.", "Check ruler activation and runtime evaluation separately; stored rules do not prove alert delivery.")
}

func inspectRuntime(ctx context.Context, s model.SLO, p Plan, now time.Time) v1.IntegrationStage {
	base := p.Target.QueryURL
	if p.Target.Kind == "mimir" {
		base = p.Target.RulerURL
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return ruleStage("unknown", "evaluator_url_invalid", "No valid evaluator API URL is configured.", "Ask an administrator to configure the intended query or ruler URL with its HTTP prefix.")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v1/rules"
	q := endpoint.Query()
	if p.Target.Kind == "mimir" {
		q.Set("rule_group", p.Group.Name)
		q.Set("file", p.Target.Namespace)
	} else {
		q.Set("rule_group[]", p.Group.Name)
	}
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return inspectionHTTP("evaluator", 0)
	}
	if p.Target.Kind == "mimir" {
		req.Header.Set("X-Scope-OrgID", p.Target.MimirOrgID)
	}
	if p.Target.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.Target.BearerToken)
	} else if p.Target.BasicUsername != "" {
		req.SetBasicAuth(p.Target.BasicUsername, p.Target.BasicPassword)
	}
	client := http.Client{Timeout: 5 * time.Second, Transport: observability.Transport{PeerService: "rule-evaluator"}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return inspectionHTTP("evaluator", 0)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return inspectionHTTP("evaluator", res.StatusCode)
	}
	var envelope struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
		Data     struct {
			Groups    []runtimeGroup `json:"groups"`
			NextToken string         `json:"groupNextToken"`
		} `json:"data"`
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 || json.Unmarshal(body, &envelope) != nil || envelope.Status != "success" || envelope.Data.Groups == nil || len(envelope.Warnings) > 0 || envelope.Data.NextToken != "" {
		return ruleStage("unknown", "evaluator_response_incomplete", "The evaluator response is invalid, partial or contains warnings.", "Inspect the backend API and response limits; partial evidence is not proof of current evaluation.")
	}
	var matches []runtimeGroup
	wrongNamespace, missingNamespace := false, false
	for _, group := range envelope.Data.Groups {
		if group.Name != p.Group.Name {
			continue
		}
		if p.Target.Kind == "mimir" && group.File != p.Target.Namespace {
			wrongNamespace = true
			missingNamespace = missingNamespace || group.File == ""
			continue
		}
		matches = append(matches, group)
	}
	if len(matches) == 0 {
		if missingNamespace {
			return ruleStage("unknown", "ruler_namespace_unverified", "The runtime response does not identify the Mimir namespace.", "Use a ruler API that reports each group's namespace before treating evaluation as verified.")
		}
		if wrongNamespace {
			return ruleStage("not_loaded", "ruler_namespace_mismatch", "The matching group name was found only in another Mimir namespace.", "Verify the reviewed namespace and wait for its ruler group to load. A group in another namespace is not sufficient.")
		}
		action := "Check the shared rule_files mount and reload Prometheus. For Prometheus Operator, verify namespace and ruleSelector labels."
		if p.Target.Kind == "mimir" {
			action = "Check the configured Mimir namespace, ruler polling delay and ownership; inspect again after the group loads."
		}
		return ruleStage("not_loaded", "runtime_group_missing", "The evaluator has not loaded this group.", action)
	}
	if len(matches) != 1 || !runtimeMatches(matches[0], p.Group) {
		return ruleStage("out_of_date", "loaded_rules_drifted", "Loaded rules are ambiguous or differ from the current definition.", "Compare the current reviewed plan with loaded expressions, labels, intervals and pending durations; reload or reconcile explicitly.")
	}
	var oldest time.Time
	for _, rule := range matches[0].Rules {
		if rule.Health == "err" || rule.LastError != "" {
			return ruleStage("failed", "rule_evaluation_failed", "The evaluator reports a rule error.", "Inspect the backend Rules page for expression, permission and source errors. Raw provider errors are not exposed here.")
		}
		if rule.Health != "ok" || rule.LastEvaluation.IsZero() {
			return ruleStage("pending", "rule_evaluation_pending", "The group is loaded but has not completed a successful evaluation.", "Wait for its configured evaluation interval, then check again. Investigate persistent pending state in the backend.")
		}
		age := now.Sub(rule.LastEvaluation)
		if age < -5*time.Second || age > max(3*time.Duration(s.StepSeconds)*time.Second, time.Minute) {
			return ruleStage("stale", "rule_evaluation_stale", "The loaded group has no recent successful evaluation.", "Check ruler scheduling, backend availability and clock synchronization before treating the group as current.")
		}
		if oldest.IsZero() || rule.LastEvaluation.Before(oldest) {
			oldest = rule.LastEvaluation
		}
	}
	out := ruleStage("evaluating", "rules_evaluating", "All current rules report recent successful evaluations.", "Verify recorded samples and traffic coverage separately. Configure and test Alertmanager routing and notification delivery independently.")
	out.ObservedAt = &oldest
	return out
}

type runtimeGroup struct {
	File     string        `json:"file"`
	Name     string        `json:"name"`
	Interval float64       `json:"interval"`
	Rules    []runtimeRule `json:"rules"`
}
type runtimeRule struct {
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	Query          string            `json:"query"`
	Labels         map[string]string `json:"labels"`
	Annotations    map[string]string `json:"annotations"`
	Duration       float64           `json:"duration"`
	Health         string            `json:"health"`
	LastError      string            `json:"lastError"`
	LastEvaluation time.Time         `json:"lastEvaluation"`
}

func runtimeMatches(actual runtimeGroup, expected Group) bool {
	interval, err := DurationSeconds(expected.Interval)
	if err != nil || actual.Interval != float64(interval) || len(actual.Rules) != len(expected.Rules) {
		return false
	}
	for i, rule := range actual.Rules {
		want := expected.Rules[i]
		name, kind := want.Record, "recording"
		if want.Alert != "" {
			name, kind = want.Alert, "alerting"
		}
		if rule.Name != name || rule.Type != kind || !maps.Equal(rule.Labels, want.Labels) || !maps.Equal(rule.Annotations, want.Annotations) {
			return false
		}
		if kind == "alerting" {
			seconds, err := DurationSeconds(want.For)
			if err != nil || float64(seconds) != rule.Duration {
				return false
			}
		}
		a, ea := parser.NewParser(parser.Options{}).ParseExpr(rule.Query)
		b, eb := parser.NewParser(parser.Options{}).ParseExpr(want.Expr)
		if ea != nil || eb != nil || a.String() != b.String() {
			return false
		}
	}
	return true
}
