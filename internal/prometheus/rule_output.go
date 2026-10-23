package prometheus

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

var outputDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// RuleOutput probes the query client's own credentials and organization. It
// never borrows a ruler-write credential or infers freshness from query time.
func (c *Client) RuleOutput(ctx context.Context, identity model.RuleOutputIdentity, now time.Time) v1.IntegrationStage {
	stage := func(state, code, message, action string) v1.IntegrationStage {
		return v1.IntegrationStage{State: state, Code: code, Message: message, NextAction: action}
	}
	unknown := stage("unknown", "rule_output_unavailable", "Recorded rule output could not be verified in the configured query dataset.", "Check query credentials, routing and backend availability. A successful ruler evaluation does not prove that its output reaches this dataset.")
	if !tenancy.ValidID(identity.Tenant) || !tenancy.ValidID(identity.Source) || !tenancy.ValidID(identity.SLO) || !outputDigest.MatchString(identity.DefinitionDigest) || identity.Revision < 1 || identity.StepSeconds < 1 || identity.StepSeconds > 3600 || len(identity.ResourceUID) > 128 || now.IsZero() {
		return unknown
	}
	selector := Selector(model.RuleOutputMetric, map[string]string{"nines_managed": "true", "nines_tenant": identity.Tenant, "nines_source": identity.Source, "nines_slo": identity.SLO, "nines_resource_uid": identity.ResourceUID})
	// An instant query response timestamp is its evaluation time. timestamp()
	// exposes the stored sample time; the RHS also requires marker value 1.
	expression := "timestamp(" + selector + ") and (" + selector + " == 1)"
	q := url.Values{"query": {expression}, "time": {strconv.FormatFloat(float64(now.UnixMilli())/1000, 'f', 3, 64)}, "limit": {"9"}, "timeout": {"3s"}}
	var data struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"`
		} `json:"result"`
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := c.telemetryRequest(bounded, "query", q, &data); err != nil || data.ResultType != "vector" || data.Result == nil || len(data.Result) > 8 {
		return unknown
	}
	if len(data.Result) == 0 {
		return stage("not_observed", "rule_output_missing", "No configuration marker is visible in the configured query dataset.", "For older rules, review and explicitly redeploy to add the marker. Otherwise allow evaluation/ingestion delay, then check query/ruler organization, cluster routing and remote-write. Missing output alone does not identify the failed boundary.")
	}
	var observed time.Time
	matches := 0
	for _, sample := range data.Result {
		labels := sample.Metric
		if labels["nines_tenant"] != identity.Tenant || labels["nines_source"] != identity.Source || labels["nines_slo"] != identity.SLO || labels["nines_resource_uid"] != identity.ResourceUID || labels["nines_managed"] != "true" || !outputDigest.MatchString(labels["nines_definition_digest"]) || len(sample.Value) != 2 {
			return unknown
		}
		if labels["nines_definition_digest"] != identity.DefinitionDigest || labels["nines_revision"] != strconv.FormatInt(identity.Revision, 10) {
			continue
		}
		var text string
		if json.Unmarshal(sample.Value[1], &text) != nil {
			return unknown
		}
		seconds, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > float64(now.Add(24*time.Hour).Unix()) {
			return unknown
		}
		observed = time.UnixMilli(int64(seconds * 1000)).UTC()
		matches++
	}
	if matches == 0 {
		return stage("out_of_date", "rule_output_out_of_date", "Only other definition digests or revisions were found in the query dataset.", "Compare the reviewed definition and loaded rules with this query route; explicitly reconcile or reload after review.")
	}
	if matches != 1 {
		return stage("unknown", "rule_output_ambiguous", "Multiple outputs match this definition identity.", "Check duplicate rule owners, extra label dimensions and query aggregation before relying on this evidence.")
	}
	age := now.Sub(observed)
	if age < -5*time.Second || age > max(3*time.Duration(identity.StepSeconds)*time.Second, time.Minute) {
		return stage("stale", "rule_output_stale", "The matching configuration marker is not recent.", "Check evaluation, ingestion delay and clock synchronization. Instant-query success does not make an old stored sample current.")
	}
	out := stage("verified", "rule_output_verified", "A fresh marker for this definition and revision is visible through the configured query source.", "Verify SLI traffic coverage and alert delivery separately. This marker verifies output visibility, not physical dataset topology or business semantics.")
	out.ObservedAt = &observed
	return out
}
