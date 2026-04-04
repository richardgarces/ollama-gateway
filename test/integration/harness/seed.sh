#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

IT_MONGO_HOST="${IT_MONGO_HOST:-127.0.0.1}"
IT_MONGO_PORT="${IT_MONGO_PORT:-27018}"
IT_MONGO_USER="${IT_MONGO_USER:-admin}"
IT_MONGO_PASSWORD="${IT_MONGO_PASSWORD:-integration}"
IT_MONGO_DB="${IT_MONGO_DB:-ollama_gateway}"

IT_QDRANT_HOST="${IT_QDRANT_HOST:-127.0.0.1}"
IT_QDRANT_PORT="${IT_QDRANT_PORT:-6334}"
IT_QDRANT_COLLECTION="${IT_QDRANT_COLLECTION:-integration_seed}"

IT_OLLAMA_HOST="${IT_OLLAMA_HOST:-127.0.0.1}"
IT_OLLAMA_PORT="${IT_OLLAMA_PORT:-11435}"
IT_OLLAMA_SEED_MODEL="${IT_OLLAMA_SEED_MODEL:-}"

mongo_uri="mongodb://${IT_MONGO_USER}:${IT_MONGO_PASSWORD}@${IT_MONGO_HOST}:${IT_MONGO_PORT}/?authSource=admin"

echo "[seed] Mongo: inserting minimal documents"
mongosh "$mongo_uri" --quiet --eval "
  const dbName='${IT_MONGO_DB}';
  const dbRef=db.getSiblingDB(dbName);
  dbRef.profiles.updateOne(
    { user_id: 'integration-user' },
    { \$set: { user_id:'integration-user', preferred_model:'local-rag', temperature:0.2, max_tokens:256, updated_at:new Date() }, \$setOnInsert: { created_at:new Date() } },
    { upsert: true }
  );
  dbRef.feedback.insertOne({ rating:5, model:'integration-model', created_at:new Date(), comment:'integration seed' });
"

echo "[seed] Qdrant: creating collection and upserting one point"
curl -fsS -X PUT "http://${IT_QDRANT_HOST}:${IT_QDRANT_PORT}/collections/${IT_QDRANT_COLLECTION}" \
  -H "Content-Type: application/json" \
  -d '{"vectors":{"size":4,"distance":"Cosine"}}' >/dev/null || true

curl -fsS -X PUT "http://${IT_QDRANT_HOST}:${IT_QDRANT_PORT}/collections/${IT_QDRANT_COLLECTION}/points?wait=true" \
  -H "Content-Type: application/json" \
  -d '{"points":[{"id":1,"vector":[0.1,0.2,0.3,0.4],"payload":{"source":"integration-seed","created_at":"now"}}]}' >/dev/null

echo "[seed] Ollama: health check"
curl -fsS "http://${IT_OLLAMA_HOST}:${IT_OLLAMA_PORT}/" >/dev/null

if [[ -n "${IT_OLLAMA_SEED_MODEL}" ]]; then
  echo "[seed] Ollama: pulling model ${IT_OLLAMA_SEED_MODEL}"
  curl -fsS -X POST "http://${IT_OLLAMA_HOST}:${IT_OLLAMA_PORT}/api/pull" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"${IT_OLLAMA_SEED_MODEL}\",\"stream\":false}" >/dev/null
fi

echo "[seed] completed"
