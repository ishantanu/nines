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
- **From another alerting tool.** Alertmanager, PagerDuty, cloud alerts or a script can send alerts that open or join incidents; see below.
- **By declaration.** An editor can declare an incident at any time, for chosen SLOs or none. Declaring early carries no penalty: alerts on those SLOs join the declared incident instead of opening another.

One incident groups related alerts:

- A firing alert joins the open incident that already includes its SLO.
- Otherwise it joins an open incident that shares a critical user journey with the SLO, because a broken journey is one user-visible problem.
- Otherwise it opens a new incident.

Nines checks after every evaluation, so incidents catch up after a restart or a missed evaluation, and concurrent alerts join one incident rather than opening two.

## Alerts from other tools

SLOs catch problems users feel on the journeys you measure. Causes elsewhere, such as an unmeasured service, a certificate or a network path, can still reach users before an SLO covers them. Send your paging alerts to Nines so those incidents are recorded too.

`POST /api/incidents/alerts` accepts:

- **An Alertmanager webhook** as is (one notification group is one alert);
- **`{"signals": [...]}`** from any other tool, each with:
  - `source`, such as `pagerduty`;
  - `key`, the same across repeated notifications;
  - `status`: `firing` or `resolved`;
  - `title`, and optionally `severity`, `url`, `startedAt`, `slos` and `journeys`.

How alerts are handled:

- **What opens an incident.** Only firing alerts open incidents. Alerts with severity `warning`, `info` or `none` are ignored, so route only paging alerts here.
- **Repeats and resolution.** A repeated notification changes nothing. A resolved alert is recorded; when nothing fires any more, the incident moves to monitoring, as with SLO alerts.
- **Grouping.** Label an alert with `nines_slo` or `nines_journey` (comma-separated IDs) to group it with incidents on those SLOs or journeys. A later burn alert on the same journey joins it.
- **"Not detected by any SLO".** An incident no SLO alert fired for is marked this way. Nines cannot measure its impact, and the portal offers to define an SLO that would catch the next one.
- **Credentials.** Sending alerts needs a token with the editor role. Give each tool its own token, granted for exactly one tenant: then the tool needs no `X-Nines-Tenant` header, which Alertmanager cannot always set. Links must be http or https without embedded credentials, or they are dropped.

```yaml
# alertmanager.yml
receivers:
  - name: nines
    webhook_configs:
      - url: https://nines.example/api/incidents/alerts
        http_config:
          authorization:
            credentials_file: /etc/alertmanager/nines-token
route:
  routes:
    - matchers: [severity="critical"]
      receiver: nines
      continue: true
```

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

For each affected SLO, the impact is the bad events from when users started to be affected until the incident's resolution (or now), divided by the error budget the SLO's window allowed at that moment. These are the same quantities the SLO's status uses.

- **When impact started.** A burn alert fires after its windows and pending time have passed, so users are affected before it fires. Nines walks back from the alert through consecutive intervals that burned budget faster than the SLO allows, up to the alert's long window, and reports where that run began. A gap in the data stops the walk, so the start is never earlier than the evidence shows.
- **Missing data.** When some intervals during the incident have no data, the observed bad events are reported as a floor ("at least"), because the missing intervals can only add more. The budget consumed stays unknown: the missing traffic would also have changed the budget.

The impact is computed when you read the incident, never stored. The budget consumed is **unknown**, with the reason given, when:

- any interval during the incident has no data;
- the SLO's window had no traffic, so it allowed no budget;
- the SLO's definition changed after the incident, so its samples are not comparable;
- the telemetry provider evaluates the SLO's windows itself, so per-interval samples are not stored.

## In the portal

Open **Incidents** under Reliability. The list shows each incident's state and its largest known budget impact.

- **Detail.** For each affected SLO, the detail shows:
  - when impact started, and how long before the alert that was;
  - the alert's firing or cleared time;
  - bad events, where "At least" means some data is missing;
  - the budget consumed, or **Unknown** with the reason;
  - data coverage and the definition revision in force.

  Below that are the affected journeys and the timeline.
- **Lifecycle.** Each button asks for a note, which goes on the timeline with your name. Viewers and agents see the incident but cannot change it.
- **Declare incident** opens one for chosen SLOs.
- **Start review** opens a [reliability review](architecture.md) scoped to the incident, when reliability reviews are turned on.

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

## Review an incident

A [reliability review](architecture.md) can be scoped to an incident. It pins the incident revision it started from, and each evidence capture saves the incident record as it is at that moment, with its timeline and impact, under a digest. The review keeps them even as the incident moves on.

## Coming next

The incident learning work continues in these issues:

- a postmortem required by the [error budget policy](error-budget-policy.md) (#86);
- [blameless postmortems](postmortems.md) on incident reviews (#87), now available through the API;
- CAST (#88) and STPA (#89) analyses;
- links to runbooks, logs, traces, changes and incident tools (#90).
