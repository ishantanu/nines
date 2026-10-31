package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestVerifyWorkflowWebhook(t *testing.T) {
	key := "01234567890123456789012345678901"
	now := time.Unix(1700000000, 0)
	event := v1.WorkflowEvent{APIVersion: v1.APIVersion, ID: "f3cce635-10d4-4e5f-97bb-b29d9d814392", Tenant: "tenant", Conversation: "conversation", Action: "change_set.proposed", Outcome: "succeeded", At: now, Version: 1}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	signed := func(b []byte, stamp string) http.Header {
		h := http.Header{}
		mac := hmac.New(sha256.New, []byte(key))
		_, _ = mac.Write([]byte(stamp + "."))
		_, _ = mac.Write(b)
		h.Set("X-Nines-Timestamp", stamp)
		h.Set("X-Nines-Signature", "v1="+hex.EncodeToString(mac.Sum(nil)))
		h.Set("X-Nines-Event-Id", event.ID)
		return h
	}
	stamp := strconv.FormatInt(now.Unix(), 10)
	got, err := VerifyWorkflowWebhook(body, signed(body, stamp), key, now, 5*time.Minute)
	if err != nil || got.ID != event.ID {
		t.Fatalf("valid event: %v", err)
	}
	t.Run("modified body", func(t *testing.T) {
		_, err := VerifyWorkflowWebhook(append(body, ' '), signed(body, stamp), key, now, 5*time.Minute)
		if err == nil {
			t.Fatal("accepted modified body")
		}
	})
	t.Run("stale", func(t *testing.T) {
		old := strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10)
		_, err := VerifyWorkflowWebhook(body, signed(body, old), key, now, 5*time.Minute)
		if err == nil {
			t.Fatal("accepted stale timestamp")
		}
	})
	t.Run("future", func(t *testing.T) {
		future := strconv.FormatInt(now.Add(6*time.Minute).Unix(), 10)
		_, err := VerifyWorkflowWebhook(body, signed(body, future), key, now, 5*time.Minute)
		if err == nil {
			t.Fatal("accepted future timestamp")
		}
	})
	t.Run("duplicate headers", func(t *testing.T) {
		h := signed(body, stamp)
		h.Add("X-Nines-Signature", h.Get("X-Nines-Signature"))
		_, err := VerifyWorkflowWebhook(body, h, key, now, 5*time.Minute)
		if err == nil {
			t.Fatal("accepted duplicate signature")
		}
	})
	t.Run("wrong event id", func(t *testing.T) {
		h := signed(body, stamp)
		h.Set("X-Nines-Event-Id", "different")
		_, err := VerifyWorkflowWebhook(body, h, key, now, 5*time.Minute)
		if err == nil {
			t.Fatal("accepted mismatched event id")
		}
	})
	t.Run("bad key", func(t *testing.T) {
		_, err := VerifyWorkflowWebhook(body, signed(body, stamp), "short", now, 5*time.Minute)
		if err == nil {
			t.Fatal("accepted short key")
		}
	})
	t.Run("malformed", func(t *testing.T) {
		b := []byte("{}")
		_, err := VerifyWorkflowWebhook(b, signed(b, stamp), key, now, 5*time.Minute)
		if err == nil {
			t.Fatal("accepted malformed event")
		}
	})
}
