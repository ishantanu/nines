package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type nativeResultsInput struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}
type nativeDraftInput struct {
	Source   string `json:"source"`
	NativeID string `json:"nativeID"`
	Name     string `json:"name"`
	Project  string `json:"project"`
	Service  string `json:"service"`
}

func addNativeSLOOperations(s *mcp.Server, a *apiClient) {
	register(s, a, "native_slo_results", "Read independent native Datadog/New Relic results for the exact published current revision. Requires explicit RFC3339 range of at most 31 days. Not Nines coverage or alert delivery evidence.", true, func(ctx context.Context, in nativeResultsInput) (any, error) {
		if e := validID(in.ID); e != nil {
			return nil, e
		}
		f, e := time.Parse(time.RFC3339, in.From)
		if e != nil {
			return nil, e
		}
		t, e := time.Parse(time.RFC3339, in.To)
		if e != nil || !t.After(f) || t.Sub(f) > 31*24*time.Hour {
			return nil, errors.New("provide a valid from/to range of at most 31 days")
		}
		q := url.Values{"from": {in.From}, "to": {in.To}}
		return a.request(ctx, http.MethodGet, "/api/slos/"+in.ID+"/native-results?"+q.Encode(), nil)
	})
	register(s, a, "list_native_slos", "Read a bounded catalog from a configured native Datadog/New Relic connection; ID is the connection name. Uses only server-approved credentials and entity/account routes.", true, func(ctx context.Context, in idInput) (any, error) {
		if e := validID(in.ID); e != nil {
			return nil, e
		}
		return a.request(ctx, http.MethodGet, "/api/connections/"+in.ID+"/native-slos", nil)
	})
	register(s, a, "draft_native_slo_import", "Prepare experimental Nines YAML from a supported native SLO. No adoption, synchronization or writes. Review using plan_slo; apply requires independent authorization. Does not import alerts or notifications.", true, func(ctx context.Context, in nativeDraftInput) (any, error) {
		if e := validID(in.Source); e != nil {
			return nil, e
		}
		if e := validID(in.Name); e != nil {
			return nil, e
		}
		if in.NativeID == "" || len(in.NativeID) > 512 || in.Project == "" || in.Service == "" {
			return nil, errors.New("nativeID, name, project and service required")
		}
		q := url.Values{"nativeID": {in.NativeID}, "name": {in.Name}, "project": {in.Project}, "service": {in.Service}}
		return a.request(ctx, http.MethodGet, "/api/connections/"+in.Source+"/native-slo-draft?"+q.Encode(), nil)
	})
}
