package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestObserveIntentChangeSetPinsScopeAndRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/intent-conversations/review/change-set/observe" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer key" {
			t.Error("scope changed")
		}
		var in v1.ReviewIntentChangeSet
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.ExpectedVersion != 4 || !in.Confirm || in.Digest != "digest" {
			t.Error("request changed", in)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v1.IntentConversation{APIVersion: v1.APIVersion, Tenant: "alpha", IntentConversationSummary: v1.IntentConversationSummary{ID: "review", Version: 5}})
	}))
	defer server.Close()
	c, err := New(Config{URL: server.URL, Tenant: "alpha", Token: "key"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.ObserveIntentChangeSet(context.Background(), "review", v1.ReviewIntentChangeSet{ExpectedVersion: 4, Digest: "digest", Confirm: true})
	if err != nil || out.ID != "review" {
		t.Fatal(out, err)
	}
}
