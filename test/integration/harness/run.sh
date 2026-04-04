#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.integration.yml"
PROJECT_NAME="${IT_COMPOSE_PROJECT:-ollama-gateway-it}"
REPORT_FILE="${IT_REPORT_FILE:-$SCRIPT_DIR/last-report.txt}"
KEEP_UP="${IT_KEEP_UP:-0}"

TEST_CMD_DEFAULT="cd \"$REPO_ROOT/api\" && go test -tags=integration -v ./..."
TEST_CMD="${IT_TEST_CMD:-$TEST_CMD_DEFAULT}"

IT_MONGO_PORT="${IT_MONGO_PORT:-27018}"
IT_QDRANT_PORT="${IT_QDRANT_PORT:-6334}"
IT_OLLAMA_PORT="${IT_OLLAMA_PORT:-11435}"

cleanup() {
  if [[ "$KEEP_UP" == "1" ]]; then
    echo "[cleanup] skipping docker compose down (IT_KEEP_UP=1)" | tee -a "$REPORT_FILE"
    return
  fi
  echo "[cleanup] stopping and removing integration services" | tee -a "$REPORT_FILE"
  docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" down -v --remove-orphans >>"$REPORT_FILE" 2>&1 || true
}

trap cleanup EXIT

: >"$REPORT_FILE"
start_ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

echo "[harness] start: $start_ts" | tee -a "$REPORT_FILE"
echo "[harness] compose project: $PROJECT_NAME" | tee -a "$REPORT_FILE"

echo "[harness] starting Mongo/Qdrant/Ollama" | tee -a "$REPORT_FILE"
docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" up -d >>"$REPORT_FILE" 2>&1

echo "[harness] waiting for service health" | tee -a "$REPORT_FILE"
for _ in {1..60}; do
  mongo_id="$(docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" ps -q mongo 2>/dev/null || true)"
  qdrant_id="$(docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" ps -q qdrant 2>/dev/null || true)"
  ollama_id="$(docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" ps -q ollama 2>/dev/null || true)"

  mongo_ok="$(docker inspect --format='{{.State.Health.Status}}' "$mongo_id" 2>/dev/null || true)"
  qdrant_ok="$(docker inspect --format='{{.State.Health.Status}}' "$qdrant_id" 2>/dev/null || true)"
  ollama_ok="$(docker inspect --format='{{.State.Health.Status}}' "$ollama_id" 2>/dev/null || true)"
  if [[ "$mongo_ok" == "healthy" && "$qdrant_ok" == "healthy" && "$ollama_ok" == "healthy" ]]; then
    break
  fi
  sleep 2
done

if [[ "${mongo_ok:-}" != "healthy" || "${qdrant_ok:-}" != "healthy" || "${ollama_ok:-}" != "healthy" ]]; then
  echo "[harness] one or more services are not healthy" | tee -a "$REPORT_FILE"
  exit 1
fi

echo "[harness] seeding test data" | tee -a "$REPORT_FILE"
IT_MONGO_PORT="$IT_MONGO_PORT" IT_QDRANT_PORT="$IT_QDRANT_PORT" IT_OLLAMA_PORT="$IT_OLLAMA_PORT" \
  "$SCRIPT_DIR/seed.sh" >>"$REPORT_FILE" 2>&1

echo "[harness] running tests" | tee -a "$REPORT_FILE"
set +e
bash -lc "$TEST_CMD" >>"$REPORT_FILE" 2>&1
test_exit=$?
set -e

end_ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
if [[ $test_exit -eq 0 ]]; then
  result="PASS"
else
  result="FAIL"
fi

echo "[harness] result: $result" | tee -a "$REPORT_FILE"
echo "[harness] finished: $end_ts" | tee -a "$REPORT_FILE"
echo "[harness] report: $REPORT_FILE"

exit $test_exit
