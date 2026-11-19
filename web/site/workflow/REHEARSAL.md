# Rehearsal: reviewed SLO workflow

- Reference: Stageshow choreography (retained changing text) and static-card (one focal result). Kept the existing Nines terminal composition rather than introducing another product's chrome.
- Beats: Inspect, Validate, Plan, Review. User-triggered selection; no autoplay or loop. The initial, server-rendered frame is the review gate.
- Checked in the in-app browser: 1280 px desktop and 390 px mobile, all four stages, keyboard arrow navigation and Home, active/focus states, readable command text, and no horizontal page overflow at 390 px. The outgoing receipt is hidden from assistive technology.
- Reduced-motion and initial-visible rendering are covered by component tests. The illustration makes no network requests; the example does not apply or approve a real SLO.
- Review findings addressed: formerly low-contrast command text; mobile tab labels use a fixed four-column layout with 44 px targets; step numbers disappear at narrow widths; the complete command wraps instead of introducing a horizontal scrollbar.
- Third-party motion source and MIT license are recorded in PROVENANCE.md and LICENSE and included in the site's bundled notices.
- Not checked: Stageshow film.mjs or automated film gates (browser interactions were verified through the available computer-use interface), frame-by-frame animation capture, real assistive-technology speech, or every browser/OS. No claim of full WCAG certification or a measured production LCP result.
