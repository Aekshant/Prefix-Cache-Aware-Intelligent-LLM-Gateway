# LLM Gateway — Prefix-Cache-Aware Router

A lightweight Go gateway that sits in front of several LLM workers (for example [Ollama](https://ollama.com) instances) and routes each chat request to the worker most likely to already have its prompt prefix in cache. That lowers **time-to-first-token (TTFT)** for repeated system prompts and multi-turn conversations.

It streams responses back as **Server-Sent Events** and exports latency and cache metrics to **Prometheus**.

```
                ┌──────────────────────────────┐
  client ──POST─▶        LLM Gateway :8080     │──▶ worker-1 (ollama :11434)
  /api/chat     │  hash(prefix) → worker pin   │
     ◀── SSE ───│  TTL janitor · load fallback │──▶ worker-2 (ollama :11435)
                └──────────────┬───────────────┘
                               │ /metrics
                               ▼
                          Prometheus
```

## How it works

1. **Prefix hashing:** the first `PREFIX_BYTES` bytes of the request body are hashed with xxHash. Requests that share a system prompt or conversation start produce the same hash.
2. **Sticky routing:** the gateway keeps an in-memory map from prefix hash to worker.
   - **Hit:** if that worker has fewer than `MAX_ACTIVE_CONNS` in-flight requests, the request goes to it.
   - **Miss, or the pinned worker is saturated:** a worker is picked round-robin, and the prefix is pinned to it.
3. **TTL eviction:** a background janitor runs every `JANITOR_INTERVAL` and drops entries that haven't been used for `CACHE_TTL`.
4. **Streaming and metrics:** the worker's line-delimited response is re-emitted as `data: …` SSE frames. TTFT and inter-token latency are recorded per worker.

## Project layout

```
.
├── cmd/gateway/          # entrypoint: HTTP server, graceful shutdown
├── internal/
│   ├── config/           # env-based configuration
│   ├── gateway/          # routing, prefix cache, janitor, HTTP handlers
│   └── metrics/          # Prometheus metric definitions
├── deploy/prometheus.yml # Prometheus scrape config for docker compose
├── Dockerfile            # multi-stage build → distroless image
├── docker-compose.yml    # gateway + 2× Ollama + Prometheus
└── Makefile
```

## Quick start

### Run locally

Requires Go 1.27+ and two Ollama instances on ports `11434` and `11435`:

```bash
OLLAMA_HOST=127.0.0.1:11434 ollama serve &
OLLAMA_HOST=127.0.0.1:11435 ollama serve &

make run            # or: go run ./cmd/gateway
```

### Run with Docker Compose

```bash
make up             # builds the gateway and starts gateway, 2× Ollama, Prometheus

# pull a model into the shared volume (once)
docker compose exec ollama-1 ollama pull llama3.2
```

| Service    | URL                     |
|------------|-------------------------|
| Gateway    | http://localhost:8080   |
| Prometheus | http://localhost:9090   |
| Ollama 1   | http://localhost:11434  |
| Ollama 2   | http://localhost:11435  |

### Send a request

```bash
curl -N http://localhost:8080/api/chat -d '{
  "model": "llama3.2",
  "messages": [{"role": "user", "content": "Why is the sky blue?"}]
}'
```

Send the same request again: the log shows `cache=hit` and it goes to the same worker.

## Configuration

All settings come from environment variables:

| Variable           | Default                                                              | Description                                               |
|--------------------|----------------------------------------------------------------------|-----------------------------------------------------------|
| `LISTEN_ADDR`      | `:8080`                                                              | Address the gateway listens on                            |
| `WORKERS`          | `worker-1=http://127.0.0.1:11434,worker-2=http://127.0.0.1:11435`    | Comma-separated `id=url` list of workers                  |
| `CACHE_TTL`        | `10m`                                                                | How long an unused prefix stays pinned to a worker        |
| `JANITOR_INTERVAL` | `30s`                                                                | How often expired prefixes are evicted                    |
| `MAX_ACTIVE_CONNS` | `5`                                                                  | In-flight requests above which a pinned worker is skipped |
| `PREFIX_BYTES`     | `120`                                                                | Number of leading body bytes used for the prefix hash     |

Durations use Go syntax (`90s`, `5m`, `1h`).

## API

| Method | Path        | Description                                       |
|--------|-------------|---------------------------------------------------|
| POST   | `/api/chat` | Proxies to the worker's `/api/chat` and streams SSE |
| GET    | `/healthz`  | Liveness check, returns `ok`                      |
| GET    | `/metrics`  | Prometheus metrics                                |

## Metrics

| Metric                               | Type      | Labels                   | Description                          |
|--------------------------------------|-----------|--------------------------|--------------------------------------|
| `llm_ttft_seconds`                   | histogram | `worker`, `cache_status` | Time to first token                  |
| `llm_itl_seconds`                    | histogram | `worker`                 | Inter-token latency                  |
| `llm_gateway_cache_evictions_total`  | counter   |                          | Prefix entries evicted by TTL        |
| `llm_gateway_active_cached_prefixes` | gauge     |                          | Prefix entries currently in memory   |

Example PromQL, p95 TTFT for hits vs misses:

```promql
histogram_quantile(0.95, sum by (le, cache_status) (rate(llm_ttft_seconds_bucket[5m])))
```

## Development

```bash
make test           # go test -race ./...
make vet            # go vet ./...
make fmt            # gofmt
make build          # → bin/gateway
make docker-build   # → llm-gateway:latest
```
