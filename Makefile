.PHONY: help build test test-integration integration-test run clean docker-build docker-up docker-down lint fmt

help: ## Mostrar esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Compilar el binario
	cd api && go build -o bin/server ./cmd/server

test: ## Ejecutar tests
	cd api && go test -v -count=1 ./...

test-integration: ## Ejecutar tests de integración
	cd api && go test -tags=integration -v ./...

integration-test: ## Ejecutar harness de integración aislado con Docker Compose
	chmod +x test/integration/harness/run.sh test/integration/harness/seed.sh
	test/integration/harness/run.sh

test-coverage: ## Ejecutar tests con cobertura
	cd api && go test -v -count=1 -coverprofile=coverage.out ./...
	cd api && go tool cover -html=coverage.out -o coverage.html

run: ## Ejecutar el servidor localmente
	cd api/cmd/server && go run main.go

clean: ## Limpiar archivos generados
	rm -rf api/bin api/coverage.*

docker-build: ## Construir imágenes Docker
	docker compose -f docker-compose.api.yml build

docker-up: ## Levantar servicios con Docker Compose
	docker compose -f docker-compose.ollama.yml up -d
	docker compose -f docker-compose.qdrant.yml up -d
	docker compose -f docker-compose.mongo.yml up -d
	docker compose -f docker-compose.api.yml up -d

docker-down: ## Detener servicios Docker
	docker compose -f docker-compose.api.yml down
	docker compose -f docker-compose.mongo.yml down
	docker compose -f docker-compose.qdrant.yml down
	docker compose -f docker-compose.ollama.yml down

docker-logs: ## Ver logs de los servicios
	docker compose -f docker-compose.api.yml logs -f

lint: ## Ejecutar go vet y otros linters
	cd api && go vet ./...
	cd api && go fmt ./...

fmt: ## Formatear código
	cd api && go fmt ./...

deps: ## Descargar dependencias
	cd api && go mod download
	cd api && go mod tidy

.DEFAULT_GOAL := help
