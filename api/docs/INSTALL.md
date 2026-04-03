# Instalacion y despliegue operativo

Objetivo: dejar el gateway funcionando con Ollama desacoplado de la API.

Topologia recomendada:

- Compose API: [docker-compose.api.yml](../../docker-compose.api.yml).
- Compose Ollama/WebUI: [docker-compose.ollama.yml](../../docker-compose.ollama.yml).
- Compose Qdrant: [docker-compose.qdrant.yml](../../docker-compose.qdrant.yml).
- Compose Mongo: [docker-compose.mongo.yml](../../docker-compose.mongo.yml).

Archivos relevantes:

- [docker-compose.ollama.yml](../../docker-compose.ollama.yml)
- [docker-compose.api.yml](../../docker-compose.api.yml)
- [docker-compose.qdrant.yml](../../docker-compose.qdrant.yml)
- [docker-compose.mongo.yml](../../docker-compose.mongo.yml)
- [ENV_VARS.md](ENV_VARS.md)

## Opcion A: todo en una misma maquina

1. Levantar Ollama/WebUI:

```bash
cd <repo-root>
docker compose -f docker-compose.ollama.yml up -d
```

2. Levantar Qdrant:

```bash
docker compose -f docker-compose.qdrant.yml up -d
```

3. Levantar Mongo:

```bash
docker compose -f docker-compose.mongo.yml up -d
```

4. Levantar API:

```bash
docker compose -f docker-compose.api.yml up -d
```

3. Verificar:

```bash
curl -fsS http://localhost:11434/ >/dev/null && echo "ollama ok"
curl -fsS http://localhost:8081/health | cat
curl -fsS http://localhost:6333/ | cat
```

## Opcion B: despliegue distribuido (A/B)

- Maquina A: Ollama.
- Maquina B: Qdrant + Mongo.
- Maquina API: servidor Go.

Variables minimas en maquina API:

```bash
export PORT=8081
export OLLAMA_URL=http://<A_HOST>:11434
export QDRANT_URL=http://<B_HOST>:6333
export MONGO_URI=mongodb://admin:<password>@<B_HOST>:27017
export JWT_SECRET="$(openssl rand -hex 32)"
```

Para levantar solo servicios de datos en B, puedes usar [docker-compose-machine-b.yml](docker-compose-machine-b.yml).

## Ejecutar API en modo binario (sin Docker)

```bash
cd <repo-root>/api
go mod tidy
go build -o bin/server ./cmd/server
./bin/server
```

## Notas de seguridad

- No expongas Mongo ni Qdrant directamente a internet.
- Mantén `JWT_SECRET` fuera del repositorio (secret manager o variable de entorno segura).
- Protege puertos internos con firewall/VPN.
- Endpoints `/internal/*` y `/api-docs` deben consumirse desde localhost.
