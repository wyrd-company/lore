#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -z "${AI_GATEWAY_API_KEY:-}" && -f "$root/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$root/.env"
  set +a
fi
base_url="${LORE_E2E_BASE_URL:-http://127.0.0.1:18081}"
database_base="${TEST_DATABASE_URL:-postgres://lore:lore@localhost:5432/lore?sslmode=disable}"
schema="lore_e2e_${RANDOM}_$$"
separator="?"
[[ "$database_base" == *\?* ]] && separator="&"
database_url="${database_base}${separator}search_path=${schema}%2Cpublic"
workdir="$(mktemp -d "${TMPDIR:-/tmp}/lore-e2e.XXXXXX")"
server_log="$workdir/server.log"
state_path="$workdir/state.json"
server_pid=""

cleanup() {
  local status=$?
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  if (( status != 0 )); then
    echo "Lore server log (failure tail):"
    tail -80 "$server_log" 2>/dev/null || true
  fi
  (cd "$root" && go run ./e2e/dbtool drop "$database_base" "$schema") >/dev/null 2>&1 || true
  rm -rf "$workdir"
  exit "$status"
}
trap cleanup EXIT

: "${AI_GATEWAY_API_KEY:?AI_GATEWAY_API_KEY is required: the e2e gate never substitutes fake embeddings}"
cd "$root"
go run ./e2e/dbtool create "$database_base" "$schema"
cp -R internal/adapters/testdata "$workdir/fixtures"

DATABASE_URL="$database_url" ./bin/lore-server migrate
DATABASE_URL="$database_url" AI_GATEWAY_API_KEY="$AI_GATEWAY_API_KEY" \
  LORE_LISTEN_ADDRESS="127.0.0.1:18081" PUBLIC_BASE_URL="$base_url" \
  LORE_INGEST_TOKEN="e2e-ingest" LORE_ADMIN_TOKEN="e2e-admin" \
  ./bin/lore-server serve >"$server_log" 2>&1 &
server_pid=$!

for _ in {1..80}; do
  if curl --fail --silent "$base_url/health/ready" >/dev/null; then break; fi
  sleep 0.25
done
curl --fail --silent "$base_url/health/ready" >/dev/null

export LORE_E2E_BASE_URL="$base_url"
export LORE_E2E_DATABASE_URL="$database_url"
export LORE_E2E_WORKDIR="$workdir"
export LORE_E2E_STATE_PATH="$state_path"
export LORE_E2E_LORE_BIN="$root/bin/lore"

go test -tags=e2e -count=1 -v ./e2e -run '^TestPrepareGate$'
npm run e2e --prefix web
go test -tags=e2e -count=1 -v ./e2e -run '^TestFinalizeGate$'

echo "E2E gate passed: real CLI, PostgreSQL, Lore server, Vercel AI Gateway, and browser."
