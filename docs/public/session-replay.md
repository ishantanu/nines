# Session replay

Session replay captures the current Nines application tab after consent. It preserves page layout and interactions.

- Approved navigation, control labels and known UI states remain readable; resource names, queries, metric values and entered text stay masked.
- It is a DOM replay, not a video of the desktop or another browser tab, and it remains experimental.

## Record your own session

1. Open **Session replay** in your tenant and expand **Record this tab**.
2. Read the recording policy, select the consent checkbox, then choose **Start recording**.
3. Navigate through the workspace. The recording indicator and **Stop recording** control remain available on SLO details and other pages.
4. Recording pauses on access-control, telemetry-connection and API/client screens, and while the API credential dialog is open. It resumes when you return to an eligible workspace page.
5. Choose **Stop recording**. Nines opens Session replay with the local recording ready to save. Enter a title and choose **Save recording** to upload it to the current tenant.

- Starting a recording does not upload it.
  - Unsaved events stay in the tab. **Discard** clears the local recording; changing tenant or signing out also discards local capture immediately, including while the logout request is pending.
  - A recorder initialization or masking failure stops capture and explains the problem.
  - An older save response cannot discard a newer recording or move you back after navigating away.
- The demo's **Checkout navigation (synthetic)** recording is a sample page.
  - To see the screen you visited, record your own session, select the saved recording and seek past its initial snapshot.
  - New captures use the `selective-v2` policy: approved static copy is readable and private values appear as asterisks.
  - Older `masked-v1` recordings remain playable with their original blanket masking; recapture to get readable labels.

## Play and download

- Select a saved recording, then use **Play**, **Pause** or the position slider.
  - Page changes, scrolling and supported layout elements are reconstructed within the player.
  - Empty or malformed streams and missing player resources produce a visible error with disabled controls.
  - Saved event bytes are checked against their stored SHA-256 before the server returns them.
  - It scales the recorded viewport to fit the panel; it does not reflow the original session to your current viewport.
- Choose **Download replay (.html)** to export a self-contained HTML file containing the recording and player.
  - Open that file in a browser to review it separately from Nines.
  - The download link is attached to the document before it is clicked, and removed afterward.
  - An export is not MP4 or WebM.
  - No external player download or Nines API connection should be needed to play the file; verify this in your target browser using the acceptance checklist below.
- Download only recordings you are authorized to retain.
  - Exported files leave Nines tenant RBAC, archival and retention controls.
  - They contain layout styles, classes, geometry and recording metadata even though private text and inputs are masked.

## What remains readable

- The recorder and server share a finite catalog of product-owned copy.
  - An explicitly annotated label is readable only when its complete text exactly matches its catalog entry.
  - This covers workspace navigation, page titles, SLO table headers, detail tabs, integration stages and known alert/data states.
  - An annotation cannot authorize arbitrary text or an entire subtree.
  - Dynamic SLO/journey/tenant names, provider errors, queries, labels, timestamps and metric values remain masked, even when their value happens to match a public label outside an annotation. Input events always remain masked.
  - Credentials, configured private regions and sensitive screens remain excluded or paused.
- This makes it possible to see which page/control was used and whether collection, rule evaluation or an alert failed without exposing its private values.
  - Unknown static copy also stays masked until explicitly reviewed and added; there is no unmask-all switch.
  - The server validates the same catalog independently for snapshots, newly added nodes and text mutations.
  - Attribute mutations cannot turn an existing private node into a readable label.
  - Recordings identify the consent policy used; legacy uploads keep the blanket-masked policy.
- For contributors: the shared catalog is `internal/replay/public-labels.json`; wrap only product-owned copy in `ReplayLabel`.
  - Never annotate user input, resource display names, raw errors or provider responses.
  - Add tests for readable copy and adjacent private values whenever extending the catalog.

## Limits and access

- Capture stops after five minutes, 2,000 retained events or its approximately 1.9 MB client limit. Server ingestion independently enforces a two-MiB bound.
- Saved recordings expire after seven days, with a maximum of 50 sessions per tenant.
- Saving requires a tenant write role. Viewers can inspect recordings available to their tenant; only owners can archive or restore them.
- Resource links identify exact SLO or critical user journey revisions, or stored firing alerts. A link is an association, not proof of causation.
- Scripts, media, frames, arbitrary attributes, URLs and unsupported event sources are removed or excluded. Layout CSS and classes remain; masking is not anonymization.
- Recordings made before a masking/layout correction may need to be captured again. Exporting cannot restore styles already removed from a stored recording.

## Browser acceptance checklist

- Before relying on replay in a browser/version, use a disposable tenant and synthetic data:

1. Verify consent is required and the recording indicator remains visible while navigating into an SLO.
2. Scroll, open ordinary dialogs and change page views. Visit a credential screen with a synthetic sentinel value, then return and stop the recording.
3. Save it, play/pause/seek and confirm the recorded pages appear with readable navigation/control labels and masked private text. Inspect the stored event payload to check the sentinel and original URL are absent.
4. Download the HTML, stop Nines and reopen the file. Check play/pause/seek and inspect the browser's network panel for unexpected requests. Repeat with the network unavailable.
5. Check viewport resizing, theme changes, tenant switching, session expiry, viewer permissions and empty/corrupt recordings. Test each intended desktop browser separately.

- The [verification record](verification.md) states which checks actually ran.
  - The in-app-browser acceptance run verified an actual HTML download file as well as playback inside Nines.
  - It does not establish Chrome, Firefox, Safari or offline-file playback; those require their own runs. See [experimental features](experimental.md) for the support boundary.

## Issue 21 alpha acceptance

- The automated baseline now covers consent/start/stop, private-page pause/resume, tenant switching, viewer permissions, API-rejected sessions and role changes, stale save completion, malformed streams, player controls and missing-resource handling.
  - Memory and PostgreSQL checks cover tenant/user boundaries, archive/restore, physical expiry pruning and corrupted stored payload rejection.
  - DOM-based tests use simulated browser APIs and do not certify browser-engine behavior.
- On 2026-10-24, the maintainer accepted the implemented replay fixes for the experimental alpha and chose to defer additional desktop-browser testing.
  - The automated checks, current-app in-app-browser capture/playback and maintainer-reported offline fixture playback are sufficient for this scoped acceptance.
  - This does not certify Chrome, Firefox or Safari compatibility; browser-specific failures remain reportable defects.
  - Issue #21 can close when the implementation merges, with this limitation stated explicitly.
- The following checklist is retained for future compatibility work; it is not a blocker for this alpha. To reproduce it, build the desired commit and run an isolated demo:

```sh
make server
env -i PATH="$PATH" ./bin/nines-server -demo -addr 127.0.0.1:18221
```

- Open `http://127.0.0.1:18221/login` and use the displayed demo credentials, `admin` / `admin`.
  - This uses disposable memory state and no external integrations.
  - Demo sign-in is a sample UI, not production authentication; real role/session boundaries are tested separately by the API and application regression suites. Leave the normal Compose playground alone.
- Run the following in **each** supported desktop browser, recording the browser version and OS:

1. Check that **Start recording** is disabled until consent is selected. Start, visit the SLO list and an SLO detail, scroll, type a synthetic value into an ordinary search field, and open a dialog/disclosure. Confirm the recording indicator remains present.
2. Switch light/dark theme and resize the window between wide and narrow sizes. Open **API connection**, enter only a synthetic sentinel such as `replay-test-secret`, close without connecting, then visit **Telemetry connections** and return. The sensitive screens must pause recording and eligible screens must resume it.
3. Stop from the SLO detail, save with a synthetic title, and verify play/pause/seek reconstruct the pages you visited. Check that approved navigation, SLO table headers and detail tabs are readable, while SLO names, queries, metric values and input values are masked, geometry follows viewport/theme changes and the credential dialog/sentinel are absent. The synthetic checkout fixture is not evidence of current-tab capture.
4. Choose **Download replay (.html)** and confirm that a file actually appears in Downloads. Stop this demo with Ctrl-C, make the browser/network unavailable, and open the downloaded file directly. Play, pause and seek across the recording. It must need no server, remote asset, browser extension or permitted network request. Record any console/CSP error or blank view as a failure.
5. Restart the demo for the next browser. In a separate capture, switch tenants or sign out and back in; no unsaved capture or consent may survive. Do not confuse demo sign-out with the separately tested server session-expiry boundary.

- Use a disposable recording.
  - To inspect masking, open its events response in Developer Tools while the server is running and search for your sentinel and original source URL.
  - Check both storage response and downloaded HTML; keep synthetic evidence only.
  - Downloaded files remain outside server retention/RBAC even when the server record is archived or expires.
- Compatibility coverage for [issue #21](https://github.com/ishantanu/nines/issues/21), deferred by the maintainer for the alpha:

| Browser / exact version / OS | Current-tab capture, navigation and scroll | Dialogs, theme and viewport | Pause / discard boundaries | Play / pause / seek | Download and direct offline reopen | Result / notes     |
| ---------------------------- | ------------------------------------------ | --------------------------- | -------------------------- | ------------------- | ---------------------------------- | ------------------ |
| Chrome                       | Unverified                                 | Unverified                  | Unverified                 | Unverified          | Unverified                         | Deferred for alpha |
| Firefox                      | Unverified                                 | Unverified                  | Unverified                 | Unverified          | Unverified                         | Deferred for alpha |
| Safari                       | Unverified                                 | Unverified                  | Unverified                 | Unverified          | Unverified                         | Deferred for alpha |

- Record the merged Git commit and the alpha compatibility limitation in the closing PR.
  - The in-app-browser run and generated standalone artifact do not establish the unverified desktop rows.
  - The maintainer reported successful offline playback without a browser/version matrix.
  - Any later browser results should name the exact version, OS and tested commit; do not replace unverified entries with inferred passes.
