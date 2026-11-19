package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestIntentReviewPacketMCPUsesReadOnlyArtifact(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/intent-conversations/chat/review-artifact" || r.Header.Get("X-Nines-Tenant") != "commerce" {
			t.Error("review export changed route or scope")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v1.IntentReviewArtifact{Tenant: "commerce", Conversation: "chat", ChangeSet: v1.IntentChangeSet{Digest: "frozen", Recommendation: &v1.IntentRecommendation{ReviewPacket: &v1.IntentReviewPacket{CandidateDiscovery: &v1.IntentReviewCandidateReceipt{State: "prior_goal", RunID: "saved-discovery"}, Destination: v1.IntentReviewDestination{Source: "primary", State: "not_verified"}}}}})
	}))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce"})
	result := call(t, ctx, session, "get_intent_review_artifact", map[string]string{"id": "chat"})
	body, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Tenant string                  `json:"tenant"`
		Data   v1.IntentReviewArtifact `json:"data"`
	}
	if err = json.Unmarshal(body, &out); err != nil || out.Tenant != "commerce" || out.Data.ChangeSet.Recommendation == nil || out.Data.ChangeSet.Recommendation.ReviewPacket == nil || out.Data.ChangeSet.Recommendation.ReviewPacket.Destination.State != "not_verified" || out.Data.ChangeSet.Recommendation.ReviewPacket.CandidateDiscovery == nil || out.Data.ChangeSet.Recommendation.ReviewPacket.CandidateDiscovery.State != "prior_goal" {
		t.Fatal("MCP lost frozen packet", string(body), err)
	}
}

func TestIntentClarificationMCPPreservesGoalExplicitly(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in v1.AppendIntentConversation
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !in.PreserveGoal || in.Goal != nil || r.URL.Path != "/api/intent-conversations/chat/messages" || r.Header.Get("X-Nines-Tenant") != "commerce" {
			t.Error("MCP clarification lost preservation or scope", err)
		}
		_ = json.NewEncoder(w).Encode(v1.IntentConversation{APIVersion: v1.APIVersion, Tenant: "commerce", IntentConversationSummary: v1.IntentConversationSummary{ID: "chat", Version: 3, State: "draft"}})
	}))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce", AllowWrites: true})
	result := call(t, ctx, session, "append_intent_answer", map[string]any{"conversationId": "chat", "input": map[string]any{"id": "retries", "expectedVersion": 2, "text": "Retries count separately.", "preserveGoal": true}})
	if result.IsError {
		t.Fatal("MCP clarification failed", result)
	}
}
