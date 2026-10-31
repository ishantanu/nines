// Package workflowhook delivers signed, minimal workflow notifications.
package workflowhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

type Config struct {
	EndpointEnv   string    `json:"endpointEnv" yaml:"endpointEnv"`
	SigningKeyEnv string    `json:"signingKeyEnv" yaml:"signingKeyEnv"`
	Since         time.Time `json:"since" yaml:"since"`
}
type Settings struct {
	Endpoint, Key string
	Since         time.Time
}
type Event = v1.WorkflowEvent
type Job struct {
	Record   v1.AuditRecord
	Token    string
	Attempts int
}

var ErrEmpty = errors.New("no notification due")
var ErrLease = errors.New("notification lease lost")

type Repository interface {
	ClaimWorkflowEvent(context.Context, string, string, time.Time, string) (Job, error)
	FinishWorkflowEvent(context.Context, string, string, string, string, bool, string) error
}

func Validate(endpoint, key string, since time.Time) error {
	u, e := url.Parse(endpoint)
	if e != nil {
		return errors.New("invalid workflow webhook endpoint")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || len(endpoint) > 2048 || (u.Scheme != "https" && (u.Scheme != "http" || !loopback)) {
		return errors.New("workflow webhook endpoint must be HTTPS or loopback HTTP, without credentials, query or fragment")
	}
	if len(key) < 32 || len(key) > 1024 || strings.ContainsFunc(key, unicode.IsControl) {
		return errors.New("workflow webhook signing key must contain 32–1024 bytes without control characters")
	}
	if since.IsZero() {
		return errors.New("workflow webhook since is required")
	}
	return nil
}
func Eligible(r v1.AuditRecord) bool {
	if r.Event.Outcome != "succeeded" && r.Event.Outcome != "blocked" {
		return false
	}
	switch r.Event.Action {
	case "change_set.proposed", "change_set.approved", "change_set.applied", "change_set.rejected", "rollback.proposed", "rollback.approved", "rollback.applied", "rollback.rejected":
		return true
	case "change_set.observed", "rollback.observed":
		return r.Outcome != nil && r.Outcome.State != "pending"
	}
	return false
}
func Envelope(r v1.AuditRecord) Event {
	e := Event{APIVersion: v1.APIVersion, ID: r.ID, Tenant: r.Tenant, Conversation: r.Conversation, Action: r.Event.Action, Outcome: r.Event.Outcome, At: r.Event.At, Version: r.Event.Version, ChangeSetDigest: r.Event.ChangeSetDigest}
	if r.Outcome != nil {
		e.ObservationState = r.Outcome.State
	}
	return e
}

type Dispatcher struct {
	Repository Repository
	Tenant     string
	Settings   Settings
	HTTP       *http.Client
}

func New(repo Repository, tenant string, settings Settings) (*Dispatcher, error) {
	if repo == nil || tenant == "" {
		return nil, errors.New("workflow webhook requires a repository and tenant")
	}
	if err := Validate(settings.Endpoint, settings.Key, settings.Since); err != nil {
		return nil, err
	}
	return &Dispatcher{Repository: repo, Tenant: tenant, Settings: settings, HTTP: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (d *Dispatcher) Target() string {
	sum := sha256.Sum256([]byte(d.Settings.Endpoint))
	return hex.EncodeToString(sum[:])
}
func (d *Dispatcher) Once(ctx context.Context) (bool, error) {
	token := model.NewID()
	job, err := d.Repository.ClaimWorkflowEvent(ctx, d.Tenant, d.Target(), d.Settings.Since, token)
	if errors.Is(err, ErrEmpty) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	delivered := true
	code := ""
	if Eligible(job.Record) {
		body, e := json.Marshal(Envelope(job.Record))
		if e != nil {
			return true, e
		}
		stamp := fmt.Sprint(time.Now().Unix())
		mac := hmac.New(sha256.New, []byte(d.Settings.Key))
		_, _ = mac.Write([]byte(stamp + "."))
		_, _ = mac.Write(body)
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, d.Settings.Endpoint, bytes.NewReader(body))
		if e != nil {
			return true, e
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nines-Event-Id", job.Record.ID)
		req.Header.Set("X-Nines-Timestamp", stamp)
		req.Header.Set("X-Nines-Signature", "v1="+hex.EncodeToString(mac.Sum(nil)))
		resp, e := d.HTTP.Do(req)
		if e != nil {
			delivered = false
			code = "transport_error"
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				delivered = false
				code = fmt.Sprintf("http_%d", resp.StatusCode)
			}
		}
	}
	// Persist completion despite caller cancellation after an external delivery.
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if e := d.Repository.FinishWorkflowEvent(finish, d.Tenant, d.Target(), job.Record.ID, token, delivered, code); e != nil {
		return true, e
	}
	if !delivered {
		return true, errors.New("workflow webhook delivery failed")
	}
	return true, nil
}
func (d *Dispatcher) Run(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		for range 20 {
			found, err := d.Once(ctx)
			if err != nil {
				if ctx.Err() == nil {
					log.WarnContext(ctx, "workflow webhook delivery incomplete")
				}
				break
			}
			if !found {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Verify authenticates one exact payload and bounds its delivery timestamp.
// Callers must persistently deduplicate Event.ID after successful processing.
func Verify(body []byte, headers http.Header, key string, now time.Time, maxSkew time.Duration) (Event, error) {
	var event Event
	if len(body) == 0 || len(body) > 64<<10 || len(key) < 32 || len(key) > 1024 || strings.ContainsFunc(key, unicode.IsControl) || maxSkew <= 0 || maxSkew > time.Hour {
		return event, errors.New("invalid webhook verification input")
	}
	one := func(name string) (string, bool) {
		values := headers.Values(name)
		if len(values) != 1 {
			return "", false
		}
		return values[0], true
	}
	stamp, ok := one("X-Nines-Timestamp")
	if !ok {
		return event, errors.New("invalid webhook timestamp")
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || strconv.FormatInt(seconds, 10) != stamp {
		return event, errors.New("invalid webhook timestamp")
	}
	delta := now.Sub(time.Unix(seconds, 0))
	if delta > maxSkew || delta < -maxSkew {
		return event, errors.New("webhook timestamp outside accepted range")
	}
	signature, ok := one("X-Nines-Signature")
	if !ok || !strings.HasPrefix(signature, "v1=") {
		return event, errors.New("invalid webhook signature")
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "v1="))
	if err != nil || len(provided) != sha256.Size {
		return event, errors.New("invalid webhook signature")
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return event, errors.New("invalid webhook signature")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&event); err != nil || dec.Decode(new(any)) != io.EOF {
		return Event{}, errors.New("invalid webhook event")
	}
	id, err := uuid.Parse(event.ID)
	headerID, ok := one("X-Nines-Event-Id")
	if err != nil || id.Version() != 4 || id.String() != event.ID || !ok || headerID != event.ID || event.APIVersion != v1.APIVersion || event.Tenant == "" || event.Conversation == "" || event.Action == "" || event.At.IsZero() {
		return Event{}, errors.New("invalid webhook event")
	}
	return event, nil
}
