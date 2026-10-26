package intenteval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentmodel"
)

func selectionFixtureClient(c selectionCase, profile fixtureProfile) (*intentmodel.Client, func(), error) {
	registry, cleanup, err := selectionRegistry(c.Extension)
	if err != nil {
		return nil, nil, err
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(io.LimitReader(r.Body, 65537)).Decode(&body) != nil {
			http.Error(w, "invalid fixture request", 400)
			return
		}
		names, last := selectionRequestState(body)
		name := "ask_user"
		args := map[string]any{"question": "Which metric, instrumentation or reviewed evaluation rubric can you supply in the pinned scope before we confirm a measurement?"}
		switch {
		case c.Clarify:
		case last == "" && names["list_slo_templates"]:
			name = "list_slo_templates"
			args = map[string]any{"category": c.Category}
		case last == "list_slo_templates" && names["render_slo_template"]:
			template, e := registry.Get(c.Templates[0])
			if e != nil {
				http.Error(w, "invalid fixture template", 500)
				return
			}
			measurement := &v1.MeasurementSpec{Type: c.Outcome, Metric: c.Metric, Labels: c.Population, SuccessLabels: c.Success, ThresholdSeconds: c.ThresholdSeconds, ScrapeIntervalSeconds: 15, ObservationPoint: "All eligible completions, including failures after retries"}
			if c.Conditional {
				measurement.ObservationPoint = "Only streams reaching a first token; failed or never-started streams are excluded and need separate request availability"
			}
			values := map[string]any{"measurement": measurement}
			if c.Extension {
				values = map[string]any{"inputs": map[string]any{"metric": c.Metric, "population": c.Population}}
			}
			name = "render_slo_template"
			args = map[string]any{"id": template.ID, "expectedTemplateDigest": template.Digest, "values": values}
		case last == "render_slo_template" && names["find_measurements"]:
			name = "find_measurements"
			args = map[string]any{}
		}
		raw, _ := json.Marshal(args)
		workflowToolStream(w, profile.style, fmt.Sprintf("selection-fixture-%s", name), name, string(raw))
	}))
	backend, err := intentmodel.NewConfigured(intentmodel.Config{Provider: profile.provider, Endpoint: server.URL + profile.path, Model: "fixture-model", APIStyle: profile.style, OutputMode: profile.output}, "fixture-evaluation-key")
	if err != nil {
		server.Close()
		cleanup()
		return nil, nil, errors.New("selection fixture configuration failed")
	}
	return backend, func() { server.Close(); cleanup() }, nil
}
func selectionRequestState(body map[string]any) (map[string]bool, string) {
	names := map[string]bool{}
	last := ""
	var walk func(any, bool)
	walk = func(value any, tools bool) {
		switch v := value.(type) {
		case []any:
			for _, x := range v {
				walk(x, tools)
			}
		case map[string]any:
			if name, ok := v["name"].(string); ok {
				if tools {
					names[name] = true
				} else if v["type"] == "tool_use" || v["arguments"] != nil {
					last = name
				}
			}
			if function, ok := v["functionCall"].(map[string]any); ok {
				last, _ = function["name"].(string)
			}
			for _, key := range []string{"function", "functionDeclarations", "content", "parts", "tool_calls"} {
				if x, ok := v[key]; ok {
					walk(x, tools)
				}
			}
		}
	}
	walk(body["tools"], true)
	walk(body["messages"], false)
	walk(body["contents"], false)
	return names, last
}
