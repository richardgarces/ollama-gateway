# Lista de comprobación operativa rápida

- [ ] `/health` responde 200
- [ ] `GET /metrics` devuelve snapshot JSON
- [ ] `/metrics/prometheus` expone métricas Prometheus
- [ ] Revisar `systemctl status ollama-gateway` (si usa systemd)
- [ ] Comprobar conectividad a `OLLAMA_URL` y `QDRANT_URL`
- [ ] Verificar espacio en disco y uso de volúmenes de Qdrant/MongoDB
- [ ] Revisar rotación de logs y tamaño de logs
