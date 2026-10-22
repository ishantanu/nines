package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ishantanu/nines/internal/model"
)

func addOperationResources(s *mcp.Server, a *apiClient) {
	for _, item := range []struct{ collection, suffix, name, kind string }{
		{"slos", "", "slo", ""},
		{"slos", "status", "slo_status", ""},
		{"slos", "context", "slo_context", ""},
		{"journeys", "context", "journey_context", ""},
		{"slos", "history", "slo_history", ""},
		{"slos", "rules", "slo_rules", ""},
		{"slos", "manifest", "slo_manifest", model.SLOKind},
		{"journeys", "manifest", "journey_manifest", model.JourneyKind},
	} {
		prefix := "nines://" + a.tenant + "/" + item.collection + "/"
		suffix := ""
		if item.suffix != "" {
			suffix = "/" + item.suffix
		}
		media := "application/json"
		if item.kind != "" {
			media = "application/yaml"
		}
		s.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: prefix + "{id}" + suffix, Name: item.name, MIMEType: media, Description: "Tenant-bound " + item.name + "; SLO history uses the current configuration revision"}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			uri := req.Params.URI
			if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
				return nil, errors.New("invalid resource URI")
			}
			id := strings.TrimSuffix(strings.TrimPrefix(uri, prefix), suffix)
			if err := validID(id); err != nil {
				return nil, err
			}
			path := "/api/" + item.collection + "/" + id + suffix
			var text string
			if item.kind != "" {
				var err error
				text, err = a.manifest(ctx, path, item.kind)
				if err != nil {
					return nil, err
				}
			} else {
				var data any
				var err error
				if item.suffix == "context" {
					data, err = readContext[json.RawMessage](ctx, a, path)
				} else {
					data, err = a.request(ctx, http.MethodGet, path, nil)
				}
				if err != nil {
					return nil, err
				}
				b, err := json.Marshal(output{Tenant: a.tenant, Data: data})
				if err != nil {
					return nil, errors.New("could not encode resource")
				}
				text = string(b)
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: media, Text: text}}}, nil
		})
	}
}
