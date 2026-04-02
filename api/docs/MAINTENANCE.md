# Mantenimiento y operaciones

Este documento recoge tareas habituales para mantener la API y los servicios asociados.

Arrancar / Detener:
- Arrancar (systemd): `systemctl start ollama-gateway`.
- Detener: `systemctl stop ollama-gateway`.
- Logs: `journalctl -u ollama-gateway -f` o `tail -f /var/log/ollama-gateway.log` si rediriges logs a archivo.

Backups y persistencia:
- Qdrant: usar snapshots y backups del volumen asociado.
- MongoDB: `mongodump` periódico y copia de seguridad fuera de la máquina B.

Verificaciones diarias (health checks):
- Endpoint `GET /health` debe responder 200.
- Endpoint `GET /metrics` debe devolver métricas JSON.
- Revisar `uptime`, `memory`, y latencia a Ollama (logs de llamadas a `/api/generate` y `/api/embeddings`).

Actualizaciones:
- Para actualizar la API: pull del repo, `go build`, `systemctl restart ollama-gateway`.
- Si cambia esquema/DB, aplicar migraciones con cuidado y en ventana de mantenimiento.

Incidentes y recuperación:
- Si Ollama no responde: comprobar conectividad (ping, curl $OLLAMA_URL). Revisar logs de Ollama (máquina A).
- Si Qdrant/Mongo fallan: restaurar desde backup y esperar a que la API reconecte.

Rotación de JWT secret:
- Para rotar `JWT_SECRET`, generar nuevo secreto y desplegarlo en las máquinas, luego reiniciar el servicio; validar que tokens emitidos con anterior secreto expiran o implementar doble aceptación transitoria si se desea periodo de transición.

Chequeos de integridad:
- Scripts recomendados: `curl -sSf http://localhost:8081/health || exit 1` integrable en monitoreo (Prometheus/Alertmanager).
