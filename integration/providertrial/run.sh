#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
: "${DD_API_KEY:?Set DD_API_KEY privately}"
: "${DD_APP_KEY:?Set DD_APP_KEY privately}"
export NEWRELIC_API_KEY="${NEWRELIC_API_KEY:-not-configured}"
export NEWRELIC_INGEST_KEY="${NEWRELIC_INGEST_KEY:-not-configured}"
export NINES_TRIAL_TOKEN_FILE="${NINES_TRIAL_TOKEN_FILE:-${TMPDIR:-/tmp}/nines-provider-trial-token}"
if [ ! -f "$NINES_TRIAL_TOKEN_FILE" ]; then
  (umask 077; openssl rand -hex 32 > "$NINES_TRIAL_TOKEN_FILE")
fi
case "${1:-}" in
  up) docker compose -p nines-provider-trial -f compose.provider-trial.yaml up --build -d seed nines ;;
  add-newrelic)
    docker compose -p nines-provider-trial -f compose.provider-trial.yaml build seed
    docker compose -p nines-provider-trial -f compose.provider-trial.yaml run --rm seed-newrelic
    docker compose -p nines-provider-trial -f compose.provider-trial.yaml up -d --no-deps --force-recreate nines
    ;;
  verify) docker compose -p nines-provider-trial -f compose.provider-trial.yaml run --rm verify ;;
  logs) docker compose -p nines-provider-trial -f compose.provider-trial.yaml logs --no-log-prefix ;;
  down) docker compose -p nines-provider-trial -f compose.provider-trial.yaml down ;;
  *) echo "Usage: $0 up|add-newrelic|verify|logs|down" >&2; exit 1 ;;
esac
