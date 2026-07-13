#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base_url="${LORE_E2E_BASE_URL:-http://127.0.0.1:18081}"
database_url="${TEST_DATABASE_URL:-postgres://lore:lore@localhost:5432/lore?sslmode=disable}"
server_log="${TMPDIR:-/tmp}/lore-web-e2e-server.log"
server_pid=""

cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

cd "$root"
DATABASE_URL="$database_url" ./bin/lore-server migrate
DATABASE_URL="$database_url" LORE_LISTEN_ADDRESS="127.0.0.1:18081" PUBLIC_BASE_URL="$base_url" LORE_INGEST_TOKEN="web-e2e-ingest" LORE_ADMIN_TOKEN="web-e2e-admin" ./bin/lore-server serve >"$server_log" 2>&1 &
server_pid=$!

for _ in {1..40}; do
  if curl --fail --silent "$base_url/health/ready" >/dev/null; then break; fi
  sleep 0.25
done
curl --fail --silent "$base_url/health/ready" >/dev/null

./bin/lore projects create --slug web-e2e --name "Web interface smoke" --server "$base_url" --token web-e2e-admin
./bin/lore upload tasks --project web-e2e --source-instance kanban --complete --server "$base_url" --token web-e2e-ingest internal/adapters/testdata/kanban
./bin/lore upload notes --project web-e2e --source-instance mnemonic --complete --server "$base_url" --token web-e2e-ingest internal/adapters/testdata/notes
./bin/lore upload briefing --project web-e2e --source-instance architecture --server "$base_url" --token web-e2e-ingest internal/adapters/testdata/briefing/architecture.html
./bin/lore upload repository --project web-e2e --source-instance fixture --repository wyrd-company/fixture --branch main --server "$base_url" --token web-e2e-ingest internal/adapters/testdata/repository
./bin/lore upload conversations --source-instance codex --provider codex --mapping web/e2e/project-map.yml --server "$base_url" --token web-e2e-ingest internal/adapters/testdata/conversations/codex

LORE_E2E_BASE_URL="$base_url" npm run e2e --prefix web
