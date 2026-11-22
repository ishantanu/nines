# Incident records

> **Experimental.** Turn it on with `NINES_FEATURES=incidents`. It may change in any release; see [feature maturity](maturity.md).

An incident record captures user-visible reliability loss. Nines fills in what it already knows, so responders and postmortem authors never retype it:

- when each critical burn alert fired and cleared;
- the error budget the incident consumed on each affected SLO;
- the critical user journeys those SLOs belong to;
- the SLO and journey revisions in force at the time.

Nines does not page anyone or run incident command. Keep your paging and response tools; links to them are planned (#90).

## How incidents open

- **From a critical burn alert.** When a policy with `severity: critical` fires, or the single unnamed policy of an SLO that has only one, Nines opens an incident starting when the alert fired.
- **By declaration.** An editor can declare an incident at any time, for chosen SLOs or none. Declaring early carries no penalty: alerts on those SLOs join the declared incident instead of opening another.

One incident groups related alerts:

- A firing alert joins the open incident that already includes its SLO.
- Otherwise it joins an open incident that shares a critical user journey with the SLO, because a broken journey is one user-visible problem.
- Otherwise it opens a new incident.

Nines checks after every evaluation, so incidents catch up after a restart or a missed evaluation, and concurrent alerts join one incident rather than opening two.

## Lifecycle

| State      | Meaning                                                                 |
| ---------- | ----------------------------------------------------------------------- |
| Detected   | Something is wrong and users are affected.                               |
| Mitigated  | The impact has stopped; the cause may still be present.                  |
| Monitoring | Waiting to see whether the fix holds.                                    |
| Resolved   | Closed. A later burn opens a new incident.                               |

- Responders move an incident between states. Each change records who made it, when, and an optional note.
- When every critical alert in an incident has cleared, Nines moves a detected or mitigated incident to monitoring. If an alert fires again while monitoring, it goes back to detected. A flapping alert therefore stays one incident.
- Only a person resolves an incident.
- A change states the revision it was made against, so two responders cannot overwrite each other's decision.

## Impact

For each affected SLO, the impact is the bad events from the incident's start to its resolution (or now), divided by the error budget the SLO's window allowed at that moment. These are the same quantities the SLO's status uses.

The impact is computed when you read the incident, never stored. It is **unknown**, with the reason given, when:

- any interval during the incident has no data;
- the SLO's window had no traffic, so it allowed no budget;
- the SLO's definition changed after the incident, so its samples are not comparable;
- the telemetry provider evaluates the SLO's windows itself, so per-interval samples are not stored.

## Using the API

Reading incidents needs any role. Declaring and changing state needs the editor or owner role; agents can read but not declare or resolve.

```sh
# Recent incidents, newest first, with impact.
curl -H "Authorization: Bearer $TOKEN" -H "X-Nines-Tenant: $TENANT" "$NINES_URL/api/incidents?limit=20"

# Declare one.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "X-Nines-Tenant: $TENANT" \
  -d '{"title":"Checkout errors after the 14:00 deploy","slos":["checkout-availability"]}' \
  "$NINES_URL/api/incidents"

# Mark it mitigated, at the revision you read.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "X-Nines-Tenant: $TENANT" \
  -d '{"state":"mitigated","expectedRevision":1,"note":"Rolled back."}' \
  "$NINES_URL/api/incidents/INCIDENT_ID/transition"
```

## Coming next

The incident learning work continues in these issues:

- a postmortem required by the [error budget policy](error-budget-policy.md) (#86);
- blameless postmortems (#87);
- CAST (#88) and STPA (#89) analyses;
- links to runbooks, logs, traces, changes and incident tools (#90).

Until then, incidents are available through the API only.
