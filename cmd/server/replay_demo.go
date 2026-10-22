package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/replay"
	"github.com/ishantanu/nines/internal/tenancy"
)

func seedReplay(ctx context.Context, s *replay.Service) error {
	now := time.Now().Add(-12 * time.Second).UnixMilli()
	events := []map[string]any{
		{"type": 4, "timestamp": now, "data": map[string]any{"href": "/synthetic-checkout", "width": 1024, "height": 640}},
		{"type": 2, "timestamp": now + 1, "data": map[string]any{"initialOffset": map[string]int{"left": 0, "top": 0}, "node": map[string]any{"type": 0, "id": 1, "childNodes": []any{
			map[string]any{"type": 1, "id": 2, "name": "html"},
			map[string]any{"type": 2, "id": 3, "tagName": "html", "attributes": map[string]string{}, "childNodes": []any{
				map[string]any{"type": 2, "id": 4, "tagName": "head", "attributes": map[string]string{}, "childNodes": []any{map[string]any{"type": 2, "id": 5, "tagName": "style", "attributes": map[string]string{}, "childNodes": []any{map[string]any{"type": 3, "id": 6, "textContent": "body{margin:0;background:#f4f3f7;font:24px system-ui;padding:60px}main{padding:40px;background:white;border:1px solid #ccc;border-radius:8px}button{padding:16px 40px;background:#cbbaf3;border:1px solid #9a87c5;border-radius:6px;font-size:22px}"}}}}},
				map[string]any{"type": 2, "id": 7, "tagName": "body", "attributes": map[string]string{}, "childNodes": []any{map[string]any{"type": 2, "id": 8, "tagName": "main", "attributes": map[string]string{}, "childNodes": []any{map[string]any{"type": 2, "id": 9, "tagName": "h1", "attributes": map[string]string{}, "childNodes": []any{map[string]any{"type": 3, "id": 10, "textContent": "Synthetic checkout"}}}, map[string]any{"type": 2, "id": 11, "tagName": "button", "attributes": map[string]string{}, "childNodes": []any{map[string]any{"type": 3, "id": 12, "textContent": "Continue"}}}}}}},
			}},
		}}}},
		{"type": 3, "timestamp": now + 3000, "data": map[string]any{"source": 1, "positions": []any{map[string]int{"x": 160, "y": 220, "id": 11, "timeOffset": 0}}}},
		{"type": 3, "timestamp": now + 7000, "data": map[string]any{"source": 2, "type": 2, "id": 11, "x": 160, "y": 220}},
		{"type": 3, "timestamp": now + 10000, "data": map[string]any{"source": 3, "id": 1, "x": 0, "y": 20}},
	}
	raw := []json.RawMessage{}
	for _, event := range events {
		b, err := json.Marshal(event)
		if err != nil {
			return err
		}
		raw = append(raw, b)
	}
	refs := []v1.ReplayReference{}
	slos, err := s.Platform.Store.List(ctx)
	if err != nil {
		return err
	}
	if len(slos) > 0 {
		rev := slos[0].Revision
		if rev < 1 {
			rev = 1
		}
		refs = append(refs, v1.ReplayReference{Kind: v1.SLOKind, ID: slos[0].ID, Revision: rev})
	}
	journeys, err := s.Platform.Journeys.Journeys(ctx)
	if err != nil {
		return err
	}
	if len(journeys) > 0 {
		j := journeys[0]
		refs = append(refs, v1.ReplayReference{Kind: v1.JourneyKind, ID: j.Manifest.Metadata.Name, Revision: j.Revision})
	}
	ctx = tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: s.Platform.TenantID, Role: "owner", Subject: "synthetic-demo", Kind: "demo", Name: "Synthetic demo fixture"})
	_, err = s.Create(ctx, v1.CreateSessionReplay{ID: "synthetic-checkout-session", Title: "Checkout navigation (synthetic)", Consent: true, Policy: replay.Policy, References: refs, Events: raw})
	if err != nil {
		return fmt.Errorf("seed synthetic replay: %w", err)
	}
	return nil
}
