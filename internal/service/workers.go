package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/store"
)

func (s *Service) Run(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(collectionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			configs, err := s.Store.List(ctx)
			if err != nil {
				log.Error("list SLOs", "error", err)
				continue
			}
			for _, c := range configs {
				if c.Retired() {
					continue
				}
				if s.CollectorFor(c) != nil && c.HasQueries() {
					// Re-read the last two completed intervals after a one-step source
					// delay. Gaps beyond this overlap need an explicit backfill.
					end := s.Now().Add(-time.Duration(c.StepSeconds) * time.Second).UTC().Truncate(time.Duration(c.StepSeconds) * time.Second)
					job, cancel := context.WithTimeout(ctx, 45*time.Second)
					overlap := int64(2)
					if c.ProviderWindow != nil {
						overlap = max(overlap, (int64(collectionInterval/time.Second)+c.StepSeconds-1)/c.StepSeconds+1)
					}
					_, err = s.Collect(job, c.ID, end.Add(-time.Duration(overlap*c.StepSeconds)*time.Second), end)
					cancel()
					if err != nil {
						log.Error("collect", "slo", c.ID, "error", err)
					}
				}
				if err = s.Evaluate(ctx, c.ID); err != nil {
					log.Error("evaluate", "slo", c.ID, "error", err)
				}
			}
		}
	}
}

type Dispatcher struct {
	Store    store.Store
	TenantID string
	URL      string
	HTTP     *http.Client
	Now      func() time.Time
}

func NewDispatcher(st store.Store, url string) *Dispatcher {
	return &Dispatcher{Store: st, URL: url, Now: time.Now, HTTP: &http.Client{Transport: observability.Transport{PeerService: "alert-webhook"}, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (d *Dispatcher) Once(ctx context.Context) (worked bool, dispatchErr error) {
	item, err := d.Store.Claim(ctx, d.Now(), 30*time.Second)
	if err != nil || item == nil {
		return false, err
	}
	var deliveryErr error
	ctx, finishSpan := observability.Operation(ctx, "alert.deliver", d.TenantID)
	defer func() {
		if dispatchErr != nil {
			finishSpan(dispatchErr)
		} else {
			finishSpan(deliveryErr)
		}
	}()
	body, err := json.Marshal(item.Event)
	if err != nil {
		return true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(body))
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", "application/json")
	key := item.Event.ID
	if d.TenantID != "" {
		key = d.TenantID + ":" + key
		req.Header.Set("X-Nines-Tenant", d.TenantID)
	}
	req.Header.Set("Idempotency-Key", key)
	message := ""
	response, err := d.HTTP.Do(req)
	if err != nil {
		message = "webhook transport failed"
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			message = fmt.Sprintf("webhook returned HTTP %d", response.StatusCode)
		}
	}
	if message != "" {
		deliveryErr = errors.New("webhook delivery failed")
	}
	backoff := time.Duration(1<<min(item.Attempts, 10)) * time.Second
	// Persist with a fresh bounded context, even when request delivery timed out.
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return true, d.Store.Finish(finish, item.Event.ID, item.Attempts, d.Now().Add(backoff), message)
}
func (d *Dispatcher) Run(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 20; i++ {
				ok, err := d.Once(ctx)
				if err != nil {
					log.Error("dispatch", "error", err)
					break
				}
				if !ok {
					break
				}
			}
		}
	}
}
