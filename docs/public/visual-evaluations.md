# Visual journey evidence

Assess a screenshot against a visible success criterion, then review the result against an exact critical user journey revision.

- This is an **experimental evaluator**, not a synthetic browser runner. Supply a screenshot from your own synthetic check or an image you have permission to share.
- The first adapter uses Jev image inputs. The service uses a typed visual-assessor interface; other providers require their own adapter and acceptance evidence. No OpenAI service is required.
- Nines records model judgments separately from human review, ordinary journey run evidence and SLI measurements. An image cannot prove backend completion or end-to-end reliability.
- There is no automatic SLO change, metric publication, approval, replay-frame upload or conversion of a probability into a good event.

## Enable the evaluator

Add an explicit opt-in to the server-owned tenant configuration, alongside the existing Jev configuration:

```yaml
systemOne:
  provider: typesafe
  model: jev-1.14.0
  apiKeyEnv: JEV_API_KEY
  imagesEnabled: true
```

- Setting an API key or enabling text advice alone does not enable images. `imagesEnabled` defaults to false.
- The adapter uses the existing TypeSafe endpoint and pinned model, with the image opt-in header required by the provider. It accepts no caller-selected endpoint or remote image URL.
- Platform administrators can pause the provider or limit daily requests. Text and image calls share the same tenant request allowance.
- The API key stays on the server. The portal sends its image only to the authenticated Nines tenant endpoint; Nines makes the provider request after validation and a durable consent reservation.
- For the optional Jev playground, set `imagesEnabled: true` in the selected server-owned `deploy/playground/tenants-jev.yaml` or `deploy/playground/tenants-mimir-jev.yaml`, then rebuild the stack using your existing Compose overrides. Existing Jev playgrounds keep images disabled until this setting changes.

## Assess and review in the portal

1. Open **Critical user journeys**, choose a journey, then open **Run evidence**.
2. Under **Visual evidence**, select a redacted PNG or JPEG. The preview remains in the browser until you submit it. Images must be at most 2 MiB and 280–1568 pixels on each side.
3. Describe a visible criterion, such as an order confirmation with no payment error. Do not include credentials or personal data.
4. Review the displayed provider/model and sharing notice. Check the permission box for this image and criterion, then select **Share image and assess**.
5. Expand the receipt to inspect the locally held image, criterion and the three probabilities: success visible, relevant error visible and sufficient visual evidence. These are model judgments, not calibrated reliability measurements or SLO attainment.
6. Expand the receipt, record **Success**, **Failure** or **Inconclusive**, explain your decision and select **Save review**. A review is immutable. It does not create a run or change an SLO.

- Only tenant owners and editors can authorize image sharing and review results. The `agent` role can read receipts but cannot grant image-sharing permission or review itself.
- Consent starts unchecked and resets when the input changes or a request completes. Every new image or criterion requires fresh authorization.
- The latest assessed image and criterion stay only in tab memory for review. **Clear local evidence**, completing the review or leaving the journey clears that local copy. Previously saved receipts require your original evidence; choose Inconclusive when it is unavailable.
- Provider failures, pauses, missing results and pending requests are evaluator gaps; they are not application failures. Low evidence sufficiency or contradictory judgments require inspection, not automatic acceptance.

## CLI and Go client

```sh
nines visual capabilities --context production --agent
# Review the displayed provider/model and retain its routeDigest before authorizing sharing.

# Use a fixed UUID for this exact input. Do not generate a new UUID for a network retry.
nines visual assess purchase \
  --id 9c51ee23-6996-4147-a71b-aed5e03b980d \
  --revision 2 \
  --route-digest REVIEWED_ROUTE_DIGEST \
  --image redacted-confirmation.png \
  --criterion 'An order confirmation is visible with no payment error.' \
  --consent --context production --agent

nines visual list purchase --context production --agent

# Inspect the receipt first. An owner/editor records the actual human review.
nines visual review purchase 9c51ee23-6996-4147-a71b-aed5e03b980d \
  --digest RECEIPT_REQUEST_DIGEST \
  --outcome inconclusive \
  --reason 'The image shows confirmation, but backend completion is unverified.' \
  --context production --agent
```

- All commands require the server. `--agent` selects deterministic output; it does not grant approval or sharing authority. Skills must obtain authorization from the responsible human before using `--consent`.
- The request must include the provider/model contract digest returned by capabilities. A changed route rejects stale consent before any provider call.
- Reusing the same ID and exact request returns the saved receipt without another model call. Changing the image, criterion, actor, route or journey revision under that ID returns a conflict. A failed or interrupted assessment is not automatically retried; a pending receipt is inconclusive.
- The Go client exposes `VisualEvaluations`, `AssessVisual` and `ReviewVisual`. HTTP routes are `GET/POST /api/journeys/{id}/visual-evaluations` and `POST /api/journeys/{id}/visual-evaluations/{evaluation}/review`. `GET /api/visual-evaluations/capabilities` reports the configured route and limits.

## Privacy and retained evidence

- The sharing notice is versioned as `visual-sharing-v1`. The receipt records the authorizing actor, server acceptance time, purpose, journey revision/digest, normalized image hash, criterion hash, evaluator contract/model, typed judgments and any subsequent human review.
- The API consent fields are caller attestations. A receipt does not independently prove the user's authority, a lawful basis or compliance with privacy law.
- Nines validates dimensions before decoding pixels, checks PNG chunk types for animation, re-encodes PNG/JPEG pixels to PNG and removes metadata and trailing bytes. It does **not** detect or redact sensitive information inside those pixels. Inspect and redact before sharing.
- Nines does not persist the image or criterion text. They are processed in memory and sent to the configured provider. Do not send personal data, secrets or customer screenshots without the required authorization and provider agreement.
- Receipts and reviews remain visible to authorized tenant readers for 30 days. Expired receipts are hidden immediately and physically pruned by the hourly evidence-retention worker or subsequent receipt requests. Their hashes and review text may still be sensitive metadata. Database backups have separate operator-owned retention.
- Provider retention, processing locations, contractual controls and deletion requests are governed by the deployment operator's arrangement with the provider. Nines cannot delete external provider copies.
- The [replay workflow](session-replay.md#privacy-and-consent) has separate capture and tenant-sharing consent. Recording a session never authorizes sending it to an image model.

## Acceptance boundary

- Deterministic tests cover image bounds and metadata removal, request opt-in, response validation, consent rejection, tenant isolation, idempotent reservation, shared administrator limits and immutable review.
- Synthetic adapter responses test the integration contract; they do not establish real image-understanding accuracy, probability calibration or supported visual tasks.
- Real-model image acceptance remains unverified until a pinned model has been evaluated against labeled, authorized screenshots, including ambiguous outcomes, poor resolution, errors and malicious instructions inside images.
- TypeSafe currently labels images and Box questions experimental. See its [image API documentation](https://docs.typesafe.ai/images). Nines does not use Box output in this first implementation.
