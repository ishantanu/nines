package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) SLOTemplates(ctx context.Context, category string) (v1.SLOTemplateCatalog, error) {
	var out v1.SLOTemplateCatalog
	err := c.scopedRequest(ctx, http.MethodGet, "/api/slo-templates?"+url.Values{"category": {category}}.Encode(), nil, 1<<20, &out)
	return out, err
}
func (c *Client) SLOTemplate(ctx context.Context, id string) (v1.SLOTemplateDetail, error) {
	var out v1.SLOTemplateDetail
	if !identifier.MatchString(id) {
		return out, errors.New("invalid template ID")
	}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/slo-templates/"+id, nil, 1<<20, &out)
	return out, err
}
func (c *Client) RenderSLOTemplate(ctx context.Context, id string, in v1.TemplateValues) (v1.SLOTemplateDraft, error) {
	var out v1.SLOTemplateDraft
	if !identifier.MatchString(id) {
		return out, errors.New("invalid template ID")
	}
	body, err := json.Marshal(in)
	if err == nil {
		err = c.scopedRequest(ctx, http.MethodPost, "/api/slo-templates/"+id+"/render", body, 1<<20, &out)
	}
	return out, err
}
