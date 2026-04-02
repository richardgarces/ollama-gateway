.PHONY: help build test test-integration run clean docker-build docker-up docker-down lint fmt

help: ## Mostrar esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Compilar el binario
	cd api && go build -o bin/server ./cmd/server

test: ## Ejecutar tests
	cd api && go test -v -count=1 ./...

test-integration: ## Ejecutar tests de integración
	cd api && go test -tags=integration -v ./...

test-coverage: ## Ejecutar tests con cobertura
	cd api && go test -v -count=1 -coverprofile=coverage.out ./...
	cd api && go tool cover -html=coverage.out -o coverage.html

run: ## Ejecutar el servidor localmente
	cd api/cmd/server && go run main.go

clean: ## Limpiar archivos generados
	rm -rf api/bin api/coverage.*

docker-build: ## Construir imágenes Docker
	docker-compose build

docker-up: ## Levantar servicios con Docker Compose
	docker-compose up -d

docker-down: ## Detener servicios Docker
	docker-compose down

docker-logs: ## Ver logs de los servicios
	docker-compose logs -f

lint: ## Ejecutar go vet y otros linters
	cd api && go vet ./...
	cd api && go fmt ./...

fmt: ## Formatear código
	cd api && go fmt ./...

deps: ## Descargar dependencias
	cd api && go mod download
	cd api && go mod tidy

.DEFAULT_GOAL := help
