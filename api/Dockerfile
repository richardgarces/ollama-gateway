FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12

ARG VCS_REF=unknown
ARG VERSION=dev

LABEL org.opencontainers.image.source="https://github.com/example/ollama_saas_project"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.description="Ollama SaaS Gateway API"
LABEL org.opencontainers.image.revision="${VCS_REF}"

COPY --from=builder /server /server

EXPOSE 8081
USER nonroot:nonroot

ENTRYPOINT ["/server"]
