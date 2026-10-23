package rules

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

// Deploy persists only the reviewed definition, optionally reloads its pinned
// Prometheus origin, and returns separate activation/evaluation evidence. Partial
// activation failure never erases the fact that a file was already written.
func (m *Manager) Deploy(ctx context.Context, s model.SLO, expected string) (Deployment, error) {
	result, err := m.Apply(ctx, s, expected)
	if err != nil {
		return result, err
	}
	if result.Target.Kind == "operator" {
		return result, nil
	}
	unknown := ruleStage("unknown", "activation_unverified", "Backend activation has not been verified.", "Inspect the current runtime before retrying; saved configuration alone does not prove activation.")
	result.Activation = &unknown
	evaluation := ruleStage("unknown", "evaluator_unavailable", "Runtime evaluation has not been verified.", "Check the configured evaluator route and credentials.")
	result.Evaluation = &evaluation
	plan, err := m.Plan(s)
	if err != nil || plan.Digest != expected {
		stage := ruleStage("blocked", "activation_plan_changed", "The destination changed after configuration was saved.", "Refresh and review the current plan before retrying activation.")
		result.Activation = &stage
		return result, nil
	}
	if plan.Target.Kind == "prometheus" && !plan.Target.Reload {
		stage := ruleStage("manual", "prometheus_reload_required", "The rule file is written. Automatic reload is disabled.", "Mount this directory in Prometheus rule_files and arrange a reload, or enable reviewed automatic reload on the trusted connection.")
		result.Activation = &stage
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		evaluation = InspectEvaluation(probe, s, plan.Group, plan.Target, time.Now().UTC())
		result.Evaluation = &evaluation
		return result, nil
	}
	if result.State == "unchanged" {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		current := InspectEvaluation(probe, s, plan.Group, plan.Target, time.Now().UTC())
		cancel()
		if current.State == "evaluating" {
			stage := ruleStage("verified", "rules_already_evaluating", "The current reviewed group is already evaluating; no activation request was repeated.", "Verify query output, measurement coverage and notification delivery separately.")
			result.Activation = &stage
			result.Evaluation = &current
			return result, nil
		}
	}
	ambiguous := false
	if plan.Target.Kind == "prometheus" {
		if err := m.authorizeWrite(ctx, s, expected); err != nil {
			stage := ruleStage("blocked", "reload_not_authorized", "Configuration was saved, but reload authorization or the reviewed destination changed.", "Inspect saved content, then obtain a current reviewed plan before retrying.")
			result.Activation = &stage
			return result, nil
		}
		code, reloadErr := reloadPrometheus(ctx, plan.Target)
		stage := ruleStage("accepted", "prometheus_reload_accepted", "Prometheus accepted the reload request. Runtime evaluation is checked separately.", "If this group is not loaded, check rule_files, the shared mount and evaluation errors.")
		if reloadErr != nil || code != http.StatusOK {
			stage = ruleStage("failed", "prometheus_reload_failed", "The reload request did not receive a successful acknowledgement. The written file is retained.", "Check lifecycle enablement, credentials and Prometheus configuration. Inspect runtime evidence before retrying.")
			if code == 401 || code == 403 {
				stage.Code = "prometheus_reload_access_denied"
			}
			if code == 404 || code == 405 {
				stage.Code = "prometheus_lifecycle_unavailable"
			}
			ambiguous = reloadErr != nil || code >= 500
		}
		result.Activation = &stage
	} else {
		stage := ruleStage("accepted", "mimir_activation_pending", "The ruler stores the reviewed group and activates it through its own polling.", "Wait for ruler activation and inspect runtime errors; no separate reload request is required.")
		result.Activation = &stage
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		evaluation = InspectEvaluation(probe, s, plan.Group, plan.Target, time.Now().UTC())
		result.Evaluation = &evaluation
		if evaluation.State != "not_loaded" || probe.Err() != nil {
			break
		}
		select {
		case <-probe.Done():
			return result, nil
		case <-time.After(500 * time.Millisecond):
		}
	}
	if ambiguous && evaluation.State == "evaluating" {
		stage := ruleStage("verified", "reload_verified_by_runtime", "The reload response was ambiguous, but the exact current group is evaluating.", "No reload request was repeated. Verify query output and coverage separately.")
		result.Activation = &stage
	}
	return result, nil
}

func reloadPrometheus(ctx context.Context, target Target) (int, error) {
	base, err := url.Parse(target.QueryURL)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "http" && base.Scheme != "https") {
		return 0, errInvalidReload
	}
	base.RawPath = ""
	base.Path = strings.TrimRight(base.Path, "/") + "/-/reload"
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, http.MethodPost, base.String(), nil)
	if err != nil {
		return 0, errInvalidReload
	}
	if target.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+target.BearerToken)
	} else if target.BasicUsername != "" {
		req.SetBasicAuth(target.BasicUsername, target.BasicPassword)
	}
	client := http.Client{Timeout: 3 * time.Second, Transport: observability.Transport{PeerService: "prometheus-reload"}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return 0, errInvalidReload
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode, nil
}

var errInvalidReload = errors.New("configured Prometheus reload could not be acknowledged")
