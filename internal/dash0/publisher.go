package dash0

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/openslo"
)

// Publication carries only reviewable content and upstream preconditions.
// It never contains credentials or a caller-selected route.
type Publication struct {
	Document     openslo.Document `json:"document"`
	Origin       string           `json:"origin"`
	PriorVersion string           `json:"priorVersion,omitempty"`
	PriorDigest  string           `json:"priorDigest,omitempty"`
	Matches      bool             `json:"matches"`
	Warnings     []string         `json:"warnings"`
}

func (c *Client) Prepare(ctx context.Context, s model.SLO, tenant, connection, namespace string) (Publication, error) {
	exported, err := openslo.Export(v1.ServiceLevelObjective(model.DefinitionFromSLO(s)))
	if err != nil {
		return Publication{}, err
	}
	if s.WindowSeconds != 28*86400 || s.Target >= 1 {
		return Publication{}, model.Invalid("Dash0 native SLOs require a 28-day rolling window and target strictly below 1")
	}
	if namespace == "" || tenant == "" || connection == "" {
		return Publication{}, model.Invalid("native SLO ownership requires tenant, connection and namespace")
	}
	doc := exported.Document
	doc.Spec.TimeWindow[0].Duration = "28d"
	origin := "nines-" + model.DefinitionDigest([]string{namespace, tenant, connection, c.dataset, s.ID})[:40]
	doc.Metadata.Labels = map[string]string{"dash0.com/origin": origin, "nines.dev/tenant": tenant, "nines.dev/source": connection, "nines.dev/slo": s.ID, "nines.dev/owner": namespace, "nines.dev/revision": strconv.FormatInt(model.ConfigRevision(s), 10)}
	doc.Metadata.Annotations = map[string]string{"dash0.com/display-name": s.Name, "dash0.com/enabled": "true"}
	p := Publication{Document: doc, Origin: origin, Warnings: exported.Warnings}
	p.Warnings = append(p.Warnings, "Dash0 evaluates native counters over fixed five-minute windows with a five-minute settling delay. This is separate from Nines' configurable windows and coverage checks.", "This publication manages only the native SLO. Native Dash0 check rules are not deployed; Nines burn policies remain independently configured.")
	current, exists, err := c.ReadNative(ctx, origin)
	if err != nil {
		return p, err
	}
	if !exists {
		return p, nil
	}
	if !owned(current, doc) {
		return p, model.Invalid("refusing to replace a Dash0 SLO without matching Nines ownership")
	}
	version := current.Metadata.Labels["dash0.com/version"]
	if version == "" || len(version) > 128 {
		return p, model.ErrUnavailable
	}
	p.PriorVersion = version
	p.PriorDigest = model.DefinitionDigest(current)
	p.Matches = matches(current, doc)
	return p, nil
}
func owned(current, want openslo.Document) bool {
	for _, key := range []string{"dash0.com/origin", "nines.dev/tenant", "nines.dev/source", "nines.dev/slo", "nines.dev/owner"} {
		if current.Metadata.Labels[key] != want.Metadata.Labels[key] {
			return false
		}
	}
	return current.Metadata.Name == want.Metadata.Name
}
func matches(current, want openslo.Document) bool {
	if current.Kind != "SLO" || (current.APIVersion != "openslo/v1" && current.APIVersion != "openslo.com/v1") || model.DefinitionDigest(current.Spec) != model.DefinitionDigest(want.Spec) {
		return false
	}
	for key, value := range want.Metadata.Labels {
		if current.Metadata.Labels[key] != value {
			return false
		}
	}
	for key, value := range want.Metadata.Annotations {
		if current.Metadata.Annotations[key] != value {
			return false
		}
	}
	return true
}
func (c *Client) ReadNative(ctx context.Context, origin string) (openslo.Document, bool, error) {
	var doc openslo.Document
	code, body, err := c.nativeRequest(ctx, http.MethodGet, "/api/slos/"+url.PathEscape(origin), nil)
	if err != nil {
		return doc, false, err
	}
	if code == 404 {
		return doc, false, nil
	}
	if code != 200 || json.Unmarshal(body, &doc) != nil || doc.Kind != "SLO" || (doc.APIVersion != "openslo/v1" && doc.APIVersion != "openslo.com/v1") {
		return doc, false, model.ErrUnavailable
	}
	return doc, true, nil
}

// Publish requires the exact resource snapshot shown in the reviewed plan. POST
// with a unique origin fences concurrent creation; versioned PUT fences updates.
func (c *Client) Publish(ctx context.Context, p Publication) (string, error) {
	current, exists, err := c.ReadNative(ctx, p.Origin)
	if err != nil {
		return "", err
	}
	if (p.PriorVersion != "") != exists || exists && (model.DefinitionDigest(current) != p.PriorDigest || current.Metadata.Labels["dash0.com/version"] != p.PriorVersion || !owned(current, p.Document)) {
		return "", model.ErrRevisionConflict
	}
	if exists && matches(current, p.Document) {
		return "unchanged", nil
	}
	method, path := http.MethodPost, "/api/slos"
	doc := p.Document
	if exists {
		method = http.MethodPut
		path += "/" + url.PathEscape(p.Origin)
		doc.Metadata.Labels["dash0.com/version"] = p.PriorVersion
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	code, _, err := c.nativeRequest(ctx, method, path, body)
	if err != nil || code < 200 || code >= 300 {
		return "", fmt.Errorf("Dash0 publication outcome is unverified; inspect and refresh the plan before retrying: %w", model.ErrUnavailable)
	}
	saved, found, err := c.ReadNative(ctx, p.Origin)
	delete(doc.Metadata.Labels, "dash0.com/version")
	if err != nil || !found || !owned(saved, doc) || !matches(saved, doc) {
		return "", fmt.Errorf("Dash0 publication read-back did not verify the desired SLO: %w", model.ErrUnavailable)
	}
	return "stored", nil
}
func (c *Client) nativeRequest(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	endpoint := c.endpoint + path + "?" + url.Values{"dataset": {c.dataset}}.Encode()
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, model.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return 0, nil, model.ErrUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return 0, nil, model.ErrUnavailable
	}
	return response.StatusCode, data, nil
}

// Disable fences the update with the reviewed version. It does not DELETE a
// resource: Dash0 does not document a conditional-delete precondition.
func (c *Client) Disable(ctx context.Context, p Publication) error {
	current, exists, err := c.ReadNative(ctx, p.Origin)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if p.PriorVersion == "" || current.Metadata.Labels["dash0.com/version"] != p.PriorVersion || model.DefinitionDigest(current) != p.PriorDigest || !owned(current, p.Document) {
		return model.ErrRevisionConflict
	}
	if current.Metadata.Annotations["dash0.com/enabled"] == "false" {
		return nil
	}
	if current.Metadata.Annotations == nil {
		current.Metadata.Annotations = map[string]string{}
	}
	current.Metadata.Annotations["dash0.com/enabled"] = "false"
	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	code, _, err := c.nativeRequest(ctx, http.MethodPut, "/api/slos/"+url.PathEscape(p.Origin), body)
	if err != nil || code < 200 || code >= 300 {
		return model.ErrUnavailable
	}
	saved, found, err := c.ReadNative(ctx, p.Origin)
	if err != nil || !found || !owned(saved, p.Document) || saved.Metadata.Annotations["dash0.com/enabled"] != "false" {
		return model.ErrUnavailable
	}
	return nil
}
