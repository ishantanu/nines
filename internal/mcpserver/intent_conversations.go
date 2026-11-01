package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type intentKubernetesExport struct {
	ConversationID string `json:"conversationId"`
	Namespace      string `json:"namespace"`
	Digest         string `json:"digest"`
}
type intentAuditPage struct {
	ID      string `json:"id"`
	After   int64  `json:"after,omitempty" jsonschema:"Continue after nextAfter from the preceding page; zero starts the export"`
	Through int64  `json:"through,omitempty" jsonschema:"Preserve the through snapshot from the first page; zero selects current sequence"`
}
type intentConversationPage struct {
	Cursor string `json:"cursor,omitempty"`
}
type intentConversationMessage struct {
	ConversationID string                      `json:"conversationId"`
	Input          v1.AppendIntentConversation `json:"input"`
}
type intentConversationRun struct {
	ConversationID string                 `json:"conversationId"`
	Input          v1.CreateIntentFlowRun `json:"input"`
}
type intentConversationCancel struct {
	ConversationID  string `json:"conversationId"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func addIntentConversations(s *mcp.Server, a *apiClient, c Config) {
	register(s, a, "list_intent_conversations", "Read one tenant-scoped metadata page of saved intent conversations. This does not invoke a model or apply resources.", true, func(ctx context.Context, in intentConversationPage) (any, error) {
		path := "/api/intent-conversations"
		if len(in.Cursor) > 300 {
			return nil, errors.New("invalid cursor")
		}
		if in.Cursor != "" {
			path += "?cursor=" + url.QueryEscape(in.Cursor)
		}
		return a.request(ctx, http.MethodGet, path, nil)
	})
	register(s, a, "get_intent_conversation", "Read a saved intent conversation, confirmed binding, preparation evidence and SLO/CUJ proposal. Native model correlation state and credentials are not exposed.", true, idRead(func(ctx context.Context, path string) (any, error) { return a.request(ctx, http.MethodGet, path, nil) }, "/api/intent-conversations/", ""))
	register(s, a, "get_intent_workflow_status", "Read a compact workflow cursor: phase, exact version/digests, next operation, human review gate and saved runtime outcome. This is guidance, not authorization. It never calls a model, queries telemetry, approves, applies or rolls back. Use the human CLI/API for review gates.", true, idRead(func(ctx context.Context, path string) (any, error) { return a.request(ctx, http.MethodGet, path, nil) }, "/api/intent-conversations/", "/status"))
	register(s, a, "get_intent_review_artifact", "Read a complete audit-verified snapshot of the current proposal, exact changes, rationale, review status and rollback evidence. May contain sensitive definitions and reviewer notes; share only with authorized recipients. This never authorizes or applies a change.", true, idRead(func(ctx context.Context, path string) (any, error) { return a.request(ctx, http.MethodGet, path, nil) }, "/api/intent-conversations/", "/review-artifact"))
	register(s, a, "get_intent_audit", "Read one tenant-scoped decision audit page and integrity result. Follow nextAfter with the same through value until nextAfter is absent; a first page may be incomplete. Cannot approve, reject, apply or roll back definitions.", true, func(ctx context.Context, in intentAuditPage) (any, error) {
		if err := validID(in.ID); err != nil {
			return nil, err
		}
		if in.After < 0 || in.Through < 0 || (in.Through > 0 && in.After > in.Through) {
			return nil, errors.New("invalid audit sequence range")
		}
		return a.request(ctx, http.MethodGet, "/api/intent-conversations/"+in.ID+"/audit?after="+strconv.FormatInt(in.After, 10)+"&through="+strconv.FormatInt(in.Through, 10), nil)
	})
	register(s, a, "intent_workflow_capabilities", "Read the intent skill contract, tool budgets and model availability. This neither starts a run nor grants binding confirmation or apply permission.", true, func(ctx context.Context, _ emptyInput) (any, error) {
		return a.request(ctx, http.MethodGet, "/api/intent-conversations/capabilities", nil)
	})
	register(s, a, "verify_intent_changes", "Read back applied definitions, drift and the saved post-change runtime/SLI outcome, including any review or rollback recommendation. This read does not query providers, approve exceptions or change resources.", true, idRead(func(ctx context.Context, path string) (any, error) { return a.request(ctx, http.MethodGet, path, nil) }, "/api/intent-conversations/", "/change-set/verification"))
	register(s, a, "export_intent_kubernetes", "Render an exact saved SLO/CUJ change-set digest as Kubernetes YAML in an explicit namespace for GitOps review. This is an offline artifact; it does not connect to Kubernetes or authorize deployment.", true, func(ctx context.Context, in intentKubernetesExport) (any, error) {
		if e := validID(in.ConversationID); e != nil {
			return nil, e
		}
		if e := validID(in.Namespace); e != nil {
			return nil, e
		}
		if len(in.Digest) != 64 {
			return nil, errors.New("exact change-set digest is required")
		}
		return a.request(ctx, http.MethodGet, "/api/intent-conversations/"+in.ConversationID+"/change-set/kubernetes?namespace="+url.QueryEscape(in.Namespace)+"&digest="+url.QueryEscape(in.Digest), nil)
	})
	if !c.AllowWrites {
		return
	}
	register(s, a, "prepare_intent_changes", "Freeze a prepared intent proposal into a reviewable SLO/CUJ change set at an exact conversation version. Requires enabled writes. Cannot approve or apply changes.", false, func(ctx context.Context, in intentConversationCancel) (any, error) {
		if e := validID(in.ConversationID); e != nil {
			return nil, e
		}
		return a.request(ctx, http.MethodPost, "/api/intent-conversations/"+in.ConversationID+"/change-set", map[string]int64{"expectedVersion": in.ExpectedVersion})
	})
	register(s, a, "create_intent_conversation", "Save an intent-authoring conversation with immutable service/source/population scope. Requires enabled writes and an API writing role. Does not call a model or apply an SLO.", false, func(ctx context.Context, in v1.CreateIntentConversation) (any, error) {
		if err := validID(in.ID); err != nil {
			return nil, err
		}
		return a.request(ctx, http.MethodPost, "/api/intent-conversations", in)
	})
	register(s, a, "append_intent_answer", "Save a user clarification or typed goal with exact conversation version. Invalidates previous selection/proposal; cannot alter scope or apply resources.", false, func(ctx context.Context, in intentConversationMessage) (any, error) {
		if err := validID(in.ConversationID); err != nil {
			return nil, err
		}
		return a.request(ctx, http.MethodPost, "/api/intent-conversations/"+in.ConversationID+"/messages", in.Input)
	})
	// Starting a model-directed run can transmit explicitly consented goal/binding
	// metadata to the configured provider. It is not a closed-world read operation.
	open := true
	mcp.AddTool(s, &mcp.Tool{Name: "start_intent_preparation", Description: "Start a bounded, durable guided or model-directed preparation run. Requires enabled writes; modelConsent must explicitly allow transmitting goal, measurement and tenant template metadata and model cost. Optional systemOneConsent separately authorizes a bounded Jev definition assessment; it is advisory and cannot apply changes. Binding confirmation remains a separate human UI/API operation. No resource apply/deployment occurs.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, OpenWorldHint: &open}}, func(ctx context.Context, _ *mcp.CallToolRequest, in intentConversationRun) (*mcp.CallToolResult, output, error) {
		if err := validID(in.ConversationID); err != nil {
			return nil, output{}, err
		}
		data, err := a.request(ctx, http.MethodPost, "/api/intent-conversations/"+in.ConversationID+"/runs", in.Input)
		return nil, output{Tenant: a.tenant, Data: data}, err
	})
	register(s, a, "cancel_intent_preparation", "Cancel a queued/running preparation at an exact conversation version. Only its initiator or an owner may cancel. Does not undo definitions, since preparation does not apply them.", false, func(ctx context.Context, in intentConversationCancel) (any, error) {
		if err := validID(in.ConversationID); err != nil {
			return nil, err
		}
		return a.request(ctx, http.MethodPost, "/api/intent-conversations/"+in.ConversationID+"/cancel", map[string]int64{"expectedVersion": in.ExpectedVersion})
	})
}
