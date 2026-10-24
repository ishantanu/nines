package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func conversationPath(id string) (string, error) {
	if !identifier.MatchString(id) {
		return "", errors.New("invalid intent conversation ID")
	}
	return "/api/intent-conversations/" + id, nil
}
func (c *Client) IntentConversations(ctx context.Context, cursor string) (v1.IntentConversationList, error) {
	var out v1.IntentConversationList
	path := "/api/intent-conversations"
	if cursor != "" {
		if len(cursor) > 300 {
			return out, errors.New("invalid conversation cursor")
		}
		path += "?cursor=" + url.QueryEscape(cursor)
	}
	e := c.scopedRequest(ctx, "GET", path, nil, 512<<10, &out)
	return out, e
}
func (c *Client) IntentConversation(ctx context.Context, id string) (v1.IntentConversation, error) {
	var out v1.IntentConversation
	p, e := conversationPath(id)
	if e == nil {
		e = c.scopedRequest(ctx, "GET", p, nil, 3<<20, &out)
	}
	if e == nil && out.ID != id {
		e = errors.New("unexpected conversation identity")
	}
	return out, e
}
func (c *Client) conversationWrite(ctx context.Context, path string, in any) (v1.IntentConversation, error) {
	var out v1.IntentConversation
	b, e := json.Marshal(in)
	if e == nil && len(b) > 32<<10 {
		return out, errors.New("conversation request exceeds 32 KiB")
	}
	if e == nil {
		e = c.scopedRequest(ctx, "POST", path, b, 3<<20, &out)
	}
	return out, e
}
func (c *Client) CreateIntentConversation(ctx context.Context, in v1.CreateIntentConversation) (v1.IntentConversation, error) {
	return c.conversationWrite(ctx, "/api/intent-conversations", in)
}
func (c *Client) AppendIntentConversation(ctx context.Context, id string, in v1.AppendIntentConversation) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/messages", in)
}
func (c *Client) ConfirmIntentBinding(ctx context.Context, id string, in v1.ConfirmIntentBinding) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/selection", in)
}
func (c *Client) StartIntentFlowRun(ctx context.Context, id string, in v1.CreateIntentFlowRun) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/runs", in)
}
func (c *Client) CancelIntentFlowRun(ctx context.Context, id string, version int64) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/cancel", map[string]int64{"expectedVersion": version})
}

func (c *Client) PrepareIntentChangeSet(ctx context.Context, id string, version int64) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/change-set", map[string]int64{"expectedVersion": version})
}
func (c *Client) ApproveIntentChangeSet(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/change-set/approve", in)
}
func (c *Client) ApplyIntentChangeSet(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/change-set/apply", in)
}
func (c *Client) VerifyIntentChangeSet(ctx context.Context, id string) (v1.IntentChangeSetVerification, error) {
	var out v1.IntentChangeSetVerification
	p, e := conversationPath(id)
	if e == nil {
		e = c.scopedRequest(ctx, "GET", p+"/change-set/verification", nil, 32<<10, &out)
	}
	if e == nil && out.Conversation != id {
		e = errors.New("unexpected conversation identity")
	}
	return out, e
}

func (c *Client) ExportIntentKubernetes(ctx context.Context, id, namespace, digest string) (v1.IntentKubernetesExport, error) {
	var out v1.IntentKubernetesExport
	p, e := conversationPath(id)
	if e != nil {
		return out, e
	}
	if !identifier.MatchString(namespace) || len(digest) != 64 {
		return out, errors.New("provide namespace and exact change-set digest")
	}
	e = c.scopedRequest(ctx, "GET", p+"/change-set/kubernetes?namespace="+url.QueryEscape(namespace)+"&digest="+url.QueryEscape(digest), nil, 4<<20, &out)
	if e == nil && (out.Namespace != namespace || out.ChangeSetDigest != digest) {
		e = errors.New("unexpected Kubernetes export scope")
	}
	return out, e
}

func (c *Client) IntentAudit(ctx context.Context, id string) (v1.IntentAuditLog, error) {
	return c.IntentAuditPage(ctx, id, 0, 0)
}
func (c *Client) IntentAuditPage(ctx context.Context, id string, after, through int64) (v1.IntentAuditLog, error) {
	var out v1.IntentAuditLog
	p, e := conversationPath(id)
	if e == nil {
		e = c.scopedRequest(ctx, "GET", p+"/audit?after="+strconv.FormatInt(after, 10)+"&through="+strconv.FormatInt(through, 10), nil, 3<<20, &out)
	}
	if e == nil && out.Conversation != id {
		e = errors.New("unexpected audit conversation identity")
	}
	return out, e
}
func (c *Client) RejectIntentChangeSet(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/change-set/reject", in)
}
func (c *Client) PrepareIntentRollback(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/change-set/rollback", in)
}
func (c *Client) ApproveIntentRollback(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/change-set/rollback/approve", in)
}
func (c *Client) ApplyIntentRollback(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/change-set/rollback/apply", in)
}
func (c *Client) RejectIntentRollback(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
	p, e := conversationPath(id)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	return c.conversationWrite(ctx, p+"/change-set/rollback/reject", in)
}
func (c *Client) ExportIntentRollbackKubernetes(ctx context.Context, id, namespace, digest string) (v1.IntentKubernetesExport, error) {
	var out v1.IntentKubernetesExport
	p, e := conversationPath(id)
	if e != nil {
		return out, e
	}
	if !identifier.MatchString(namespace) || len(digest) != 64 {
		return out, errors.New("provide namespace and exact rollback digest")
	}
	e = c.scopedRequest(ctx, "GET", p+"/change-set/rollback/kubernetes?namespace="+url.QueryEscape(namespace)+"&digest="+url.QueryEscape(digest), nil, 4<<20, &out)
	if e == nil && (out.Namespace != namespace || out.ChangeSetDigest != digest) {
		e = errors.New("unexpected rollback export scope")
	}
	return out, e
}
