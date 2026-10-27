CREATE TABLE IF NOT EXISTS slos (
 id text PRIMARY KEY,
 config jsonb NOT NULL,
 alert_state jsonb NOT NULL DEFAULT '{"phase":"inactive"}'::jsonb
);
CREATE TABLE IF NOT EXISTS samples (
 slo_id text NOT NULL REFERENCES slos(id),
 time_seconds bigint NOT NULL,
 good double precision NOT NULL CHECK (good >= 0 AND good < 'Infinity'::float8),
 total double precision NOT NULL CHECK (total >= good AND total < 'Infinity'::float8),
 PRIMARY KEY (slo_id,time_seconds)
);
CREATE TABLE IF NOT EXISTS alert_outbox (
 id text PRIMARY KEY,
 event jsonb NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed')),
 attempts integer NOT NULL DEFAULT 0,
 next_attempt timestamptz NOT NULL,
 last_error text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS alert_outbox_pending ON alert_outbox(next_attempt) WHERE status='pending';

CREATE TABLE IF NOT EXISTS journey_revisions (
 journey_id text NOT NULL,
 revision bigint NOT NULL CHECK (revision > 0),
 document jsonb NOT NULL,
 PRIMARY KEY(journey_id, revision)
);
CREATE TABLE IF NOT EXISTS journey_runs (
 journey_id text NOT NULL,
 run_id text NOT NULL,
 revision bigint NOT NULL,
 received_at timestamptz NOT NULL,
 document jsonb NOT NULL,
 PRIMARY KEY(journey_id,run_id),
 FOREIGN KEY(journey_id,revision) REFERENCES journey_revisions(journey_id,revision)
);
CREATE INDEX IF NOT EXISTS journey_runs_recent ON journey_runs(journey_id,received_at DESC,run_id);

-- Written in the same transaction as an approved journey revision. A replay
-- recovers this exact operation even if workflow status persistence failed.
CREATE TABLE IF NOT EXISTS journey_apply_receipts (
 operation_id text PRIMARY KEY,
 document jsonb NOT NULL
);

ALTER TABLE slos ADD COLUMN IF NOT EXISTS collection_health jsonb NOT NULL DEFAULT '{"state":"not_observed","points":0}'::jsonb;

CREATE TABLE IF NOT EXISTS topology_revisions (
 kind text NOT NULL,
 resource_id text NOT NULL,
 revision bigint NOT NULL CHECK (revision>0),
 document jsonb NOT NULL,
 PRIMARY KEY (kind,resource_id,revision)
);

-- Overlapping provider estimates are separate from disjoint interval counts.
CREATE TABLE IF NOT EXISTS window_observations (
 slo_id text NOT NULL REFERENCES slos(id),
 window_seconds bigint NOT NULL CHECK(window_seconds>0),
 time_seconds bigint NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 good double precision NOT NULL CHECK(good>=0 AND good<'Infinity'::float8),
 total double precision NOT NULL CHECK(total>=good AND total<'Infinity'::float8),
 coverage double precision NOT NULL CHECK(coverage>=0 AND coverage<=1),
 PRIMARY KEY(slo_id,window_seconds,time_seconds)
);

-- Atomic multi-resource intent application receipts, scoped by tenant schema.
CREATE TABLE IF NOT EXISTS definition_batch_receipts (
 operation_id text PRIMARY KEY,
 document jsonb NOT NULL
);

ALTER TABLE slos ADD COLUMN IF NOT EXISTS operator_route text NOT NULL DEFAULT '';

-- Frozen snapshots from immediately before a definition revision was replaced.
-- Active samples stay separate; old measurements are never reinterpreted.
CREATE TABLE IF NOT EXISTS slo_revision_archives (
 slo_id text NOT NULL REFERENCES slos(id),
 revision bigint NOT NULL CHECK(revision > 0),
 summary jsonb NOT NULL,
 document jsonb NOT NULL,
 PRIMARY KEY(slo_id, revision)
);

ALTER TABLE slos ADD COLUMN IF NOT EXISTS rule_automation jsonb;

-- Retirement receipts survive irreversible deletion. Names are never recycled.
CREATE TABLE IF NOT EXISTS slo_tombstones (
 slo_id text PRIMARY KEY,
 document jsonb NOT NULL
);
