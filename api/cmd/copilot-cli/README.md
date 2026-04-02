Copilot CLI

Usage:

Build:

```sh
go build -o bin/copilot-cli ./cmd/copilot-cli
```

Run (streaming):

```sh
./bin/copilot-cli -prompt "Implementa una función para sumar dos enteros" -url http://localhost:8081/openai/v1/chat/completions -stream=true
```

Run (non-streaming):

```sh
./bin/copilot-cli -prompt "Hola" -stream=false
```

This is a minimal helper to integrate editor workflows quickly with the local gateway.
