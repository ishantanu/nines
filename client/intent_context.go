package client

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func IntentContextPath(in v1.IntentContextInput) (string, error) {
	if !intentContextText(in.Service) || !identifier.MatchString(in.Source) || !intentContextText(in.Project) || in.Limit < 0 || in.Limit > 10 || len(in.Cursor) > 2048 || in.Journey == "" && in.JourneyRevision != 0 || in.Journey != "" && (!identifier.MatchString(in.Journey) || in.JourneyRevision < 1) {
		return "", errors.New("provide exact service/project values (1–200 bytes) and a configured source ID, limit 1–10 and a positive revision with a journey")
	}
	q := url.Values{"service": {in.Service}, "source": {in.Source}, "project": {in.Project}}
	if in.Journey != "" {
		q.Set("journey", in.Journey)
		q.Set("journeyRevision", strconv.FormatInt(in.JourneyRevision, 10))
	}
	if in.Limit > 0 {
		q.Set("limit", strconv.Itoa(in.Limit))
	}
	if in.Cursor != "" {
		q.Set("cursor", in.Cursor)
	}
	return "/api/intent-context?" + q.Encode(), nil
}

func (c *Client) IntentContext(ctx context.Context, in v1.IntentContextInput) (v1.IntentContext, error) {
	var out v1.IntentContext
	path, err := IntentContextPath(in)
	if err == nil {
		err = c.contextRead(ctx, path, &out)
	}
	return out, err
}

func intentContextText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 200 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
