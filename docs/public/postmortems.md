# Blameless postmortems

> **Experimental.** Postmortems need `NINES_FEATURES=incidents,reliability-reviews`. They may change in any release; see [feature maturity](maturity.md).

A postmortem is written on a [reliability review](architecture.md) of an [incident](incidents.md). It records what happened, why it made sense to the people involved at the time, and what will change, so the same conditions do not hurt users again. It is about systems and conditions, never about who to blame.

## Start one

On the incident, choose **Start review** (it reopens the incident's review if one exists), then choose **Postmortem** in the review's header. Capture evidence first: the timeline and impact come from the incident record saved in the review, so nobody retypes them.

## The template

| Section             | What goes in it                                                                                                   |
| ------------------- | ----------------------------------------------------------------------------------------------------------------- |
| Summary             | What users experienced and what was done, in a few sentences.                                                     |
| Timeline            | Filled in from the incident (alerts, state changes, when impact started). Add what Nines could not know: a deploy, a decision in chat. |
| Impact              | Filled in from the incident: budget consumed per SLO, or unknown with the reason, and affected journeys.          |
| Contributing causes | Several, each a **trigger**, a **latent condition** that was already there, or a missing or weak **safeguard**. There is no "root cause" field: incidents rarely have one. |
| Lessons             | What went well, what went poorly, where we got lucky.                                                              |
| Action items        | Each prevents, detects or mitigates, with an owner and due date, linked to the SLOs and journeys it changes.      |

- **Action items stay visible.** Open items show on the SLOs and journeys they link until marked done, and `GET /api/action-items?slo=ID` lists them.
- **Incidents no SLO detected.** When no SLO alert fired for the incident, the postmortem keeps a standing action item: "Define an SLO that would have caught this incident". It can be marked done, not removed, so the coverage gap becomes tracked work.

## Writing blamelessly

Hindsight makes past choices look obviously wrong. People acted on what they knew then, with the tools and pressures they had. As you write, Nines flags wording that judges with hindsight or blames, and suggests a neutral way to say it:

| Flagged                         | Instead                                                                  |
| ------------------------------- | ------------------------------------------------------------------------ |
| "should have", "could have"     | Say what was known at the time and why the action made sense then.       |
| "failed to", "forgot", "neglected" | Say what happened, and the conditions that made the step easy to miss. |
| "human error", "operator error" | Name the conditions and missing safeguards that let a normal action cause harm. |
| "root cause"                    | List contributing causes.                                                |
| "obviously", "clearly"          | Describe what was visible at the time.                                   |

Flags are advice, never a block: you decide.

## Revisions

- **History.** Each save adds a revision; earlier ones stay in the review, with who saved them and when.
- **Conflicts.** A save names the review version it was made against, so two authors cannot overwrite each other.
- **Who writes.** People write postmortems: saving needs the owner or editor role. Agents can help in review messages.
- **Publishing.** Mark a postmortem published to make it visible from the incident.

## Using the API

```sh
# The latest revision, or a new postmortem started from the incident.
curl -H "Authorization: Bearer $TOKEN" -H "X-Nines-Tenant: $TENANT" \
  "$NINES_URL/api/investigations/REVIEW_ID/postmortem"

# Save a revision at the review's current version.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "X-Nines-Tenant: $TENANT" \
  -H "Content-Type: application/json" \
  -d '{"id":"pm-1","expectedVersion":3,"postmortem":{...}}' \
  "$NINES_URL/api/investigations/REVIEW_ID/postmortem"
```

A save returns the review and any hindsight findings.

Source: *Site Reliability Engineering*, 2nd edition, chapters 10 to 12.
