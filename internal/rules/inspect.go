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
	"time"

	"github.com/prometheus/prometheus/promql/parser"
	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

// Inspect reads the configured destination and runtime API. It never deploys,
// reloads, or treats a successful write as proof of evaluation or notification.
func (m *Manager) Inspect(ctx context.Context, s model.SLO, now time.Time) (deployment, evaluation v1.IntegrationStage) {
	deployment = v1.IntegrationStage{State: "unknown", Message: "Could not inspect the configured rule destination."}
	evaluation = v1.IntegrationStage{State: "unknown", Message: "External rule evaluation has not been verified."}
	if _, ok := m.target(ConnectionID(s)); !ok {
		deployment = v1.IntegrationStage{State: "not_configured", Message: "Configure a rule destination, then review and deploy rules. Collecting measurements does not deploy rules."}
		return
	}
	p, err := m.Plan(s)
	if err != nil {
		deployment.Message = "The current definition cannot produce a deployable rule plan."
		return
	}
	var stored Group
	switch p.Target.Kind {
	case "prometheus":
		file := filepath.Join(p.Target.Directory, p.Target.Namespace+"-"+p.Group.Name+".yaml")
		f, err := os.Open(file)
		if os.IsNotExist(err) {
			deployment = v1.IntegrationStage{State: "not_deployed", Message: "No rule file exists for this SLO. Review and deploy its rules."}
		} else if err == nil {
			defer func() {
				_ = f.Close()
			}()
			var data File
			if stat, e := f.Stat(); e == nil && stat.Mode().IsRegular() && stat.Size() <= 4<<20 && yaml.NewDecoder(io.LimitReader(f, 4<<20)).Decode(&data) == nil && len(data.Groups) == 1 {
				stored = data.Groups[0]
			}
		}
	case "mimir":
		path := "/config/v1/rules/" + url.PathEscape(p.Target.Namespace) + "/" + url.PathEscape(p.Group.Name)
		code, body, err := rulerRequest(ctx, p.Target, http.MethodGet, path, nil)
		if err == nil && code == http.StatusNotFound {
			deployment = v1.IntegrationStage{State: "not_deployed", Message: "No rule group exists in the configured Mimir namespace."}
		}
		if err == nil && code == http.StatusOK {
			_ = yaml.Unmarshal(body, &stored)
		}
	}
	if stored.Name != "" {
		if sameGroup(stored, p.Group) {
			deployment = v1.IntegrationStage{State: "deployed", Message: "The configured destination contains the current rule definition."}
		} else {
			deployment = v1.IntegrationStage{State: "out_of_date", Message: "The stored rules differ from the current definition. Review a new plan before deploying."}
		}
	}
	base := p.Target.QueryURL
	if p.Target.Kind == "mimir" {
		base = p.Target.RulerURL
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return
	}
	endpoint.Path += "/api/v1/rules"
	q := endpoint.Query()
	q.Set("rule_group[]", p.Group.Name)
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return
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
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode != http.StatusOK {
		return
	}
	var envelope struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
		Data     struct {
			Groups []runtimeGroup `json:"groups"`
		} `json:"data"`
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 || json.Unmarshal(b, &envelope) != nil || envelope.Status != "success" || len(envelope.Warnings) > 0 {
		return
	}
	var matches []runtimeGroup
	for _, group := range envelope.Data.Groups {
		if group.Name == p.Group.Name {
			matches = append(matches, group)
		}
	}
	if len(matches) == 0 {
		evaluation = v1.IntegrationStage{State: "not_loaded", Message: "The evaluator has not loaded this group. For Prometheus, check rule_files and reload after deployment."}
		return
	}
	if len(matches) != 1 || !runtimeMatches(matches[0], p.Group) {
		evaluation = v1.IntegrationStage{State: "out_of_date", Message: "Loaded rules are ambiguous or differ from the current definition."}
		return
	}
	group := matches[0]
	var latest time.Time
	for _, rule := range group.Rules {
		if rule.Health == "err" || rule.LastError != "" {
			evaluation = v1.IntegrationStage{State: "failed", Message: "The evaluator reports a rule error. Inspect the provider Rules page for details."}
			return
		}
		if rule.Health != "ok" || rule.LastEvaluation.IsZero() {
			evaluation = v1.IntegrationStage{State: "pending", Message: "The group is loaded but has not completed a successful evaluation."}
			return
		}
		age := now.Sub(rule.LastEvaluation)
		if age < -5*time.Second || age > max(3*time.Duration(s.StepSeconds)*time.Second, time.Minute) {
			evaluation = v1.IntegrationStage{State: "stale", Message: "The loaded group has no recent successful evaluation."}
			return
		}
		if latest.IsZero() || rule.LastEvaluation.Before(latest) {
			latest = rule.LastEvaluation
		}
	}
	evaluation = v1.IntegrationStage{State: "evaluating", Message: "All current rules report recent successful evaluations. This does not verify traffic coverage or notification delivery.", ObservedAt: &latest}
	return
}

type runtimeGroup struct {
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
