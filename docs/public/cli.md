### Intent preparation context

- Run `nines --context production --agent intent context --service api --source primary --project production` to read saved SLO evidence before drafting a goal.
- This is a server read with no model call, provider query or resource write. Use `--journey NAME --journey-revision N` for optional revision-pinned critical user journey metadata.
- Follow `nextCursor` using `--cursor`; keep service/source/project and journey arguments unchanged. An empty inventory page is not evidence of absent telemetry.
- See the [intent workflow milestones](intent-roadmap.md) and run `make test-intent-context` for isolated fixture acceptance.

### Save evidence for intent review

- `nines --context production --agent intent conversations evidence ID -f evidence.yaml` explicitly reads telemetry and saves a server-generated receipt for a ready, human-confirmed inline intent.
- The request requires the current `expectedVersion`, exact `intentDigest` and `readTelemetry: true`. It cannot include a report or tenant/source override. History and alternatives are optional, explicit fields.
- This is a write to the preparation record. It does not apply an SLO or grant approval. See [the full request and review sequence](intents.md#attach-evidence-to-a-reviewed-proposal).

### Device authorization screen

`nines login --method device` prints a short-lived code and a verification URL. Open that URL, sign in with a configured account or SSO provider and enter the code from your own terminal. The browser shows the server, tenant and current role before authorization. Cancel any request you did not start. The screen supports narrow viewports, keyboard focus and the browser's light/dark preference; its visual styling does not change code expiry, PKCE, CSRF or tenant authorization.
