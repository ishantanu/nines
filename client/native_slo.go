package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) NativeSLOResults(ctx context.Context, id string, from, to time.Time) (v1.NativeSLOResults, error) {
	var out v1.NativeSLOResults
	if !identifier.MatchString(id) || !to.After(from) || from.IsZero() || to.Sub(from) > 31*24*time.Hour {
		return out, errors.New("provide a valid SLO and from/to range of at most 31 days")
	}
	q := url.Values{"from": {from.UTC().Format(time.RFC3339Nano)}, "to": {to.UTC().Format(time.RFC3339Nano)}}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/slos/"+id+"/native-results?"+q.Encode(), nil, 1<<20, &out)
	return out, err
}
func (c *Client) NativeSLOCatalog(ctx context.Context, source string) (v1.NativeSLOCatalog, error) {
	var out v1.NativeSLOCatalog
	if !identifier.MatchString(source) {
		return out, errors.New("invalid connection identifier")
	}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/connections/"+source+"/native-slos", nil, 1<<20, &out)
	return out, err
}
func (c *Client) NativeSLODraft(ctx context.Context, source, nativeID, name, project, service string) (v1.NativeSLODraft, error) {
	var out v1.NativeSLODraft
	if !identifier.MatchString(source) || !identifier.MatchString(name) || nativeID == "" || len(nativeID) > 512 || project == "" || service == "" {
		return out, errors.New("provide connection, native ID, name, project and service")
	}
	q := url.Values{"nativeID": {nativeID}, "name": {name}, "project": {project}, "service": {service}}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/connections/"+source+"/native-slo-draft?"+q.Encode(), nil, 1<<20, &out)
	return out, err
}
