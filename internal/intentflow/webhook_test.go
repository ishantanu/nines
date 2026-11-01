package intentflow

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/workflowhook"
)

func TestWorkflowWebhookDeliveryRetryAndScope(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := updatedChangeFixture(t, pg)
			since := time.Now().UTC().Add(-time.Second)
			var err error
			v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
			if err != nil {
				t.Fatal(err)
			}
			key := "fixture-signing-key-at-least-32-bytes"
			calls := 0
			var id string
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, e := io.ReadAll(r.Body)
				if e != nil {
					t.Error(e)
					return
				}
				timestamp := r.Header.Get("X-Nines-Timestamp")
				mac := hmac.New(sha256.New, []byte(key))
				_, _ = mac.Write([]byte(timestamp + "."))
				_, _ = mac.Write(body)
				if r.Header.Get("X-Nines-Signature") != "v1="+hex.EncodeToString(mac.Sum(nil)) {
					t.Error("invalid event signature")
				}
				var event workflowhook.Event
				if json.Unmarshal(body, &event) != nil || event.Tenant != "alpha" || event.Conversation != v.ID || event.Action != "change_set.proposed" || event.ChangeSetDigest != v.ChangeSet.Digest || event.ID != r.Header.Get("X-Nines-Event-Id") {
					t.Error("invalid event", string(body))
				}
				if strings.Contains(string(body), "eligible") || strings.Contains(string(body), "Actor") || strings.Contains(string(body), "previous") {
					t.Error("sensitive evidence was sent", string(body))
				}
				if id != "" && event.ID != id {
					t.Error("retry changed the idempotency ID")
				}
				id = event.ID
				if calls == 1 {
					w.WriteHeader(503)
				} else {
					w.WriteHeader(204)
				}
			}))
			defer receiver.Close()
			repo, ok := s.Repository.(workflowhook.Repository)
			if !ok {
				t.Fatal("missing webhook repository")
			}
			d, err := workflowhook.New(repo, "alpha", workflowhook.Settings{Endpoint: receiver.URL, Key: key, Since: since})
			if err != nil {
				t.Fatal(err)
			}
			if got, e := d.Once(ctx); !got || e == nil {
				t.Fatal("failed delivery was not recorded", got, e)
			}
			if got, e := d.Once(ctx); got || e != nil {
				t.Fatal("retry ignored backoff", got, e)
			}
			switch db := s.Repository.(type) {
			case *memory:
				db.mu.Lock()
				for k, state := range db.notifications {
					state.NextAttempt = time.Time{}
					db.notifications[k] = state
				}
				db.mu.Unlock()
			case *postgres:
				if _, err = db.pool.Exec(context.Background(), `UPDATE intent_webhook_deliveries SET next_attempt='epoch'`); err != nil {
					t.Fatal(err)
				}
			}
			if got, e := d.Once(ctx); !got || e != nil {
				t.Fatal("retry failed", got, e)
			}
			if got, e := d.Once(ctx); got || e != nil || calls != 2 {
				t.Fatal("delivered event repeated", got, e, calls)
			}
			if _, e := repo.ClaimWorkflowEvent(ctx, "beta", d.Target(), since, "another"); e != workflowhook.ErrEmpty {
				t.Fatal("tenant isolation", e)
			}
		})
	}
}
