CREATE TABLE IF NOT EXISTS incidents (
 incident_id text PRIMARY KEY,
 revision bigint NOT NULL CHECK (revision > 0),
 state text NOT NULL,
 opened_at timestamptz NOT NULL,
 document jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS incidents_open ON incidents (opened_at) WHERE state <> 'resolved';
CREATE INDEX IF NOT EXISTS incidents_recent ON incidents (opened_at DESC);
