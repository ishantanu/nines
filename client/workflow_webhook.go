package client

import (
	"net/http"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/workflowhook"
)

// VerifyWorkflowWebhook authenticates a payload; persistently deduplicate its ID
// after successful processing to protect against retries and replay.
func VerifyWorkflowWebhook(body []byte, headers http.Header, signingKey string, now time.Time, maxSkew time.Duration) (v1.WorkflowEvent, error) {
	return workflowhook.Verify(body, headers, signingKey, now, maxSkew)
}
