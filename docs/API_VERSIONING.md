# API Versioning and Migration Guide

## Overview

The gateway now supports versioned endpoints under `/api/v1` and `/api/v2`.
Legacy unversioned endpoints under `/api/...` remain available for backward compatibility.

- Legacy deprecation target date: `2026-12-31`
- Successor version for migrated endpoints: `/api/v2/...`

## Legacy Deprecation Headers

Legacy endpoints include the following headers:

- `Deprecation: true`
- `X-API-Deprecated: true`
- `X-API-Sunset-Date: 2026-12-31`
- `Sunset: 2026-12-31T23:59:59Z`
- `Link: </api/v2/...>; rel="successor-version"`
- `Warning: 299 - "Deprecated API: migrate to /api/v2/... before 2026-12-31"`

## Route Migration Map

- `POST /api/generate` -> `POST /api/v2/generate`
- `POST /api/search` -> `POST /api/v2/search`
- `POST /api/models/recommend` -> `POST /api/v2/models/recommend`
- `POST /api/v1/chat/completions` -> `POST /api/v2/chat/completions`
- `GET /api/profile` -> `GET /api/v2/profile`
- `PUT /api/profile` -> `PUT /api/v2/profile`

## Deprecated Field Translation in v2

`v2` endpoints include compatibility aliases for deprecated payload fields.
When a translation is applied, the response includes `X-API-Translated-Fields`.

### POST /api/v2/generate

- `query` -> `prompt`
- `input` -> `prompt`

### POST /api/v2/search

- `top_k` -> `top`
- `k` -> `top`
- `q` -> `query`

### POST /api/v2/models/recommend

- `task` -> `task_type`
- `sla_ms` -> `sla_latency_ms`
- `budget_tokens` -> `token_budget`

## Client Migration Checklist

1. Move clients to `/api/v2/...` paths.
2. Keep current payloads initially; aliases are translated by `v2`.
3. Monitor `X-API-Translated-Fields` and remove deprecated fields from clients.
4. Complete migration before `2026-12-31`.
