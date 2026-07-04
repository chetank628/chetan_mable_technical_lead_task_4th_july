# Mable Tracking API

A Gin + SQLite Go service that ingests tracking events through the
`github.com/mable/mono/pipeline` library on a long-running **windowed** pipeline,
persists events and per-stage pipeline metadata, serves cookie-based JWT auth,
and computes analytics.

See [`api-design.md`](./api-design.md) for the full design rationale.

## Run

```bash
cd api
go run .
# listens on :8080 (dev mode, SQLite file ./mable.db)
```

### Configuration (env vars)

| Var | Default | Purpose |
| --- | --- | --- |
| `APP_ENV` | `dev` | `dev`/`test`/`prod`; gates cookie Secure/SameSite and JWT-secret requirement |
| `ADDR` | `:8080` | HTTP listen address |
| `DB_PATH` | `mable.db` | SQLite file (`:memory:` for tests) |
| `JWT_SECRET` | dev fallback | **Required in prod** |
| `JWT_TTL` | `24h` | Session lifetime |
| `CORS_ORIGIN` | `http://localhost:5173` | Exact SPA origin (no `*` with credentials) |
| `MAX_BODY_BYTES` | `1048576` | Ingest body size cap (413 over) |
| `INGEST_BUFFER_DEPTH` | `8192` | Bounded ingest channel capacity |
| `WINDOW_DURATION` | `500ms` | Max time per pipeline window |
| `WINDOW_MAX_EVENTS` | `2048` | Max events per pipeline window |
| `PIPELINE_BATCH_SIZE` | `256` | `pipeline.WithBatchSize` |
| `PIPELINE_WORKERS` | `0` (NumCPU) | `pipeline.WithWorkerCount` |
| `PIPELINE_CHANNEL_BUFFER` | `8` | `pipeline.WithChannelBufferDepth` |
| `PIPELINE_BATCH_TIMEOUT` | `5ms` | `pipeline.WithBatchTimeout` |
| `DEDUP_CAPACITY` | `200000` | Dedup LRU size |

## Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `POST` | `/api/events` | no | Ingest one event or an array; returns `202` |
| `POST` | `/api/auth/signup` | no | Create account, set session cookie |
| `POST` | `/api/auth/login` | no | Login, set session cookie |
| `POST` | `/api/auth/logout` | no | Clear session cookie |
| `GET` | `/api/auth/me` | cookie | Current user |
| `GET` | `/api/stats` | cookie | Analytics (`?since=&until=&granularity=`) |
| `GET` | `/health` | no | Liveness/readiness (DB ping) |
| `GET` | `/metrics` | no | Prometheus-format ingest counters |

## Quick demo

```bash
# ingest (no auth) — consent gate requires consent:true
curl -s -XPOST localhost:8080/api/events -H 'Content-Type: application/json' \
  -d '{"event_type":"Purchase","session_id":"s1","user_id":"u1","amount":42,"currency":"usd","consent":true}'

# signup -> stores cookie in jar
curl -s -c jar.txt -XPOST localhost:8080/api/auth/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"supersecret"}'

# stats (uses cookie)
curl -s -b jar.txt 'localhost:8080/api/stats?granularity=minute'

# ops
curl -s localhost:8080/health
curl -s localhost:8080/metrics
```

## Test

```bash
cd api
go test -race ./...
```
