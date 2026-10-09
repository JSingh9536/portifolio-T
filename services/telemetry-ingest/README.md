# telemetry-ingest (Go)

This service ingests robot telemetry, stores it in TimescaleDB/Postgres, and streams it live over SSE.

| Endpoint | Purpose |
|---|---|
| `POST /v1/telemetry` | `{"samples": [...]}` (up to 5 000 per batch). Returns `202` with `{accepted, rejected:[{index,error}]}`. Returns `422` if every row is invalid |
| `GET /v1/sites/{site}/latest` | Latest sample per robot |
| `GET /v1/robots/{robot}/series?from&to&bucket` | Time-bucketed pressure, flow and uplift, plus the injected volume (trapezoidal integral of flow) |
| `GET /v1/stream?site=` | Server-Sent Events, `event: sample` |
| `GET /healthz`, `GET /metrics` | Health (includes a DB ping) and Prometheus metrics |

Configuration:

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | unset | Postgres/TimescaleDB DSN. If unset, the service uses the in-memory store |
| `ADDR` | `:8080` | Listen address |
| `CORS_ORIGIN` | `*` | Origin allowed to call the API |

```bash
go test -race ./...
TEST_DATABASE_URL=postgres://user:pass@localhost:5432/scratch go test ./internal/store/
```
