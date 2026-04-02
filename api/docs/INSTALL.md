# Instalación y despliegue mínimo

Objetivo: dejar la API en ejecución conectada a una instancia de Ollama (máquina A) y a servicios de datos (máquina B).

Requisitos (máquina API):
- Go 1.24+ instalado.
- Acceso de red hacia la máquina A (OLLAMA_URL) y máquina B (QDRANT_URL, MONGO_URL).
- Variables de entorno configuradas (ver `ENV_VARS.md`).

Pasos resumidos:

1) Clonar el repo en la máquina que ejecutará la API:

```bash
git clone <repo> && cd ollama_saas_project/api
```

2) Copiar ejemplo de variables de entorno y ajustar:

```bash
cp .env.example .env
# editar .env con OLLAMA_URL (máquina A) y QDRANT_URL/MONGO_URL (máquina B)
```

3) Instalar dependencias y compilar (opcional):

```bash
go mod tidy
go build -o bin/server ./cmd/server
```

4) Ejecutar directamente (modo desarrollo):

```bash
PORT=8081 JWT_SECRET="$(openssl rand -hex 32)" OLLAMA_URL=http://<ollama-host>:11434 ./bin/server
```

5) Ejecutar como servicio (sugerencia systemd):
- Crear `/etc/systemd/system/ollama-gateway.service` con `ExecStart=/opt/ollama-gateway/bin/server` y las variables de entorno.
- Recargar y arrancar: `systemctl daemon-reload && systemctl start ollama-gateway`.

Nota sobre red y seguridad:
- Ollama puede estar en otra máquina; asegúrate de que el puerto (por defecto 11434) esté accesible desde la máquina API y que haya reglas de firewall adecuadas.
- Para producción, colocar la API detrás de un reverse-proxy (NGINX) con TLS.

Ejemplo rápido: servicios de datos en máquina B (docker-compose minimal para Qdrant):

```yaml
version: '3.8'
services:
  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
    volumes:
      - qdrant_data:/qdrant/storage

volumes:
  qdrant_data:
```
