# Workflow webhooks

Nines can send signed notifications about reviewed intent changes to an integration owned by your team. They are disabled by default and configured per tenant by an operator. A notification does not approve, apply or roll back a change.

## Configure a tenant

Add the following under the tenant entry in the server's tenant configuration:

```yaml
workflowWebhook:
  endpointEnv: NINES_WORKFLOW_WEBHOOK_URL
  signingKeyEnv: NINES_WORKFLOW_WEBHOOK_SECRET
  since: "2026-10-29T00:00:00Z"
```

- Set the named environment variables in the server deployment. The endpoint must use HTTPS; HTTP is accepted only for a loopback host. URL credentials, query strings and fragments are rejected. The signing key must contain 32 to 1024 bytes and no control characters. Store it in your deployment's secret manager.
- `since` is a required, fixed RFC 3339 cutoff for audit events. Choose it deliberately: enabling this configuration can send eligible historical events from that time. Keep it stable across restarts.
- Each tenant has its own destination and signing key. The endpoint is trusted operator configuration, not something an agent can choose. Secure the receiver and give it access only to the tenants it needs.

## Events and delivery

Events contain an ID, tenant and conversation identifiers, audit action and outcome, time, conversation version, and applicable change digest or observation state. The payload does not include the proposal, review notes, model input, credentials or telemetry. Identifiers and digests can still be sensitive. Use the authenticated review-artifact API when an authorized reviewer needs details.

- Notifications cover successful and blocked proposal, approval, application and rejection actions for changes and rollbacks. Observation events are sent only when the observation is no longer pending.
- The audit journal is the source of truth. PostgreSQL-backed delivery claims and retries survive server restarts; the in-memory store does not persist them.
- Delivery is at least once, not ordered. Return a 2xx response only after safely accepting the event. Other responses and transport errors are retried with backoff. Receivers must persistently deduplicate the event ID after successful processing.
- Retries preserve the event ID but can have a new timestamp and signature. Rotating a signing key does not replay delivered events. Changing the endpoint creates a new delivery target and may resend events since the cutoff.
- Failures are recorded for retry and logged without the destination or payload. This version has no delivery-status dashboard, automatic GitHub dispatch, native chat adapter, or generic subscription API.

## Verify a request

Nines sends JSON with `X-Nines-Event-Id`, `X-Nines-Timestamp` (Unix seconds) and `X-Nines-Signature` (`v1=` followed by a lowercase hexadecimal HMAC-SHA256). The signed bytes are the ASCII timestamp, a period, then the exact request body. Enforce a body limit before reading and reject stale timestamps. The Go client provides a verifier:

```go
// Limit and read the request body before this call.
event, err := client.VerifyWorkflowWebhook(body, r.Header, signingKey, time.Now(), 5*time.Minute)
if err != nil {
    http.Error(w, "invalid notification", http.StatusUnauthorized)
    return
}
// Atomically record event.ID with your own durable processing or queue.
// A duplicate should return 2xx without repeating the action.
```

Verification proves possession of the signing key and checks the timestamp; it does not prevent replay within the accepted window or authorize a mutation. Never use the notification itself as approval. Fetch a fresh review artifact and use independently authorized identities for approval and application.

## Connect existing workflows

Use the webhook to enqueue work in an incident or internal automation system. Fetch the review artifact through the tenant-scoped API, CLI or read-only MCP when the recipient needs the exact change and audit history. The [headless workflow guide](headless-workflows.md) includes an example for a CI job summary. The [GitHub review publisher](github-reviews.md) can explicitly publish a reviewed snapshot; automatic webhook dispatch and native chat integrations remain future work.
