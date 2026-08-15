# Configuration

Everything is environment variables. There is no config file to write.

Running with the compose files, the ten settings in `.env` are all you touch, and
the compose file translates them into whatever each service expects. Only one has
no default, and compose refuses to start without it.

The rest of this page is for running the binaries yourself, or for tuning.

---

## The ten in `.env`

| Setting | Default | What it does |
|---|---|---|
| `ORYCA_INTERNAL_SECRET` | none | Shared by the two services. The gateway sends it on every `/internal/*` call, and whoever holds it can read every service and API key you have. There is no default: generate one with `openssl rand -hex 32`. |
| `ORYCA_INTERNAL_SECRET_PREV` | empty | The previous secret, accepted alongside the current one. Set it while rotating, remove it after. |
| `ORYCA_API_ROOT_EMAIL` | empty | The first administrator, created on the first start only. `.env.example` ships `admin@localhost`; leaving it truly empty skips creating a root account, and the log says so. |
| `ORYCA_API_ROOT_PASSWORD` | empty | Leave empty and one is generated and printed to the log once. |
| `ORYCA_PORTAL_PORT` | `3000` | Host port for the portal. |
| `ORYCA_GATEWAY_PORT` | `9002` | Host port for the gateway. |
| `ORYCA_CONTROL_PLANE_PORT` | `9001` | Host port for the control plane. |
| `ORYCA_API_LOG_RETENTION_DAYS` | `14` | How long request logs are kept. Applied on start, including to an existing collection. |
| `ORYCA_PORTAL_API_URL` | `http://localhost:9001/control-plane/api/v1` | Where the **visitor's browser** reaches the control plane. |
| `ORYCA_PORTAL_GATEWAY_URL` | `http://localhost:9002/gateway/api` | Where the **visitor's browser** reaches the gateway. |

The last two are the ones people get wrong. They are addresses your users' browsers
resolve, so a container name like `http://control-plane:9001` will not work. Change
them and restart the portal. No rebuild is needed, since the values are read when a
page is served.

---

## Gateway

Only the first four normally need setting. It holds no database connection of its
own; everything it knows comes from the control plane and Redis.

| Setting | Default | |
|---|---|---|
| `ORYCA_GW_HOST` | `0.0.0.0` | Listen address |
| `ORYCA_GW_PORT` | `9002` | Listen port |
| `ORYCA_GW_CP_BASE_URL` | none | Where the control plane answers. Also where the key that signs portal sessions is fetched from. |
| `ORYCA_GW_INTERNAL_SECRET` | none | Must equal `ORYCA_INTERNAL_SECRET` |
| `ORYCA_GW_PUBLIC_URL` | empty | The gateway's own public address, used to expand `{{oryca_gateway_url}}`. Guessed from the request when empty, which a load balancer can get wrong. |
| `ORYCA_GW_ALLOW_ORIGIN` | `*` | CORS origin |
| `ORYCA_GW_MAX_REQUEST_BODY` | `500M` | Largest request accepted |

**Redis.** `ORYCA_GW_REDIS_ADDRESS`, `ORYCA_GW_REDIS_PASSWORD`,
`ORYCA_GW_REDIS_DB` (default `7`), plus `_POOL_SIZE` (100), `_MIN_IDLE_CONNS`
(10), `_DIAL_TIMEOUT` (5s), `_READ_TIMEOUT` and `_WRITE_TIMEOUT` (3s).

> The gateway's default database is `7` and the control plane's is `0`. They have
> to match, and the compose files pin both to `0`. Pub/sub ignores the database
> number, so a mismatch looks healthy while no routing data ever arrives.

The response cache can live somewhere else entirely, via
`ORYCA_GW_CACHE_REDIS_ADDRESS`, `_PASSWORD` and `_DB`; each falls back to the
main Redis settings. `ORYCA_GW_CACHE_MEMORY_MB` (256) sizes the in-process tier
in front of it, and `ORYCA_GW_CACHE_DEFAULT_TTL` (300s) applies when an upstream
sends no `Cache-Control`.

**Staying in step.** `ORYCA_GW_SERVICE_POLL_INTERVAL` (60s) and
`ORYCA_GW_APIKEY_POLL_INTERVAL` (30s) are the fallback when a pushed event is
missed; `ORYCA_GW_SYNC_CHANNEL` (`oryca:sync-events`) must match the control
plane's. `ORYCA_GW_USER_CACHE_TTL` (90s) and
`ORYCA_GW_USER_CACHE_REFRESH_CONCURRENCY` (25) govern user lookups.

**Circuit breaker**, per upstream host. `ORYCA_GW_CB_CONSECUTIVE_FAILURES` (5)
to open, `ORYCA_GW_CB_TIMEOUT_SEC` (30) before a retry, `ORYCA_GW_CB_MAX_REQUESTS`
(5) allowed through while half-open, `ORYCA_GW_CB_INTERVAL_SEC` (60) to reset the
count, `ORYCA_GW_CB_IDLE_EVICT_MIN` (30) to forget a host.

**Upstream connections.** `ORYCA_GW_UPSTREAM_DIAL_TIMEOUT` (30s),
`_RESPONSE_HEADER_TIMEOUT` (30s), `_TLS_HANDSHAKE_TIMEOUT` (10s),
`_KEEP_ALIVE` (30s), `_IDLE_CONN_TIMEOUT` (90s), `_MAX_IDLE_CONNS` (1000),
`_MAX_IDLE_CONNS_PER_HOST` (100), `_MAX_CONNS_PER_HOST` (200).

---

## Control plane

| Setting | Default | |
|---|---|---|
| `ORYCA_API_HOST` | none | Public address, used in the links it emails |
| `ORYCA_API_PORT` | `9001` | Listen port |
| `ORYCA_API_DB_URL` | none | MongoDB connection string |
| `ORYCA_API_DB_NAME` | none | Database name |
| `ORYCA_API_REDIS_ADDRESS` | none | Redis address |
| `ORYCA_API_REDIS_DB` | `0` | Must match the gateway's |
| `ORYCA_API_ALLOW_ORIGIN` | `*` | CORS origin |
| `ORYCA_API_BODY_LIMIT` | `10M` | Largest request accepted |
| `ORYCA_API_JWT_PRIVATE_KEY` | `auth-private.key` | Signing key path. Generated on first start if absent. |
| `ORYCA_API_JWT_PUBLIC_KEY` | `auth-public.key` | Public half, served at `/.well-known/jwks.json` |
| `ORYCA_API_SEED_DIR` | built-in | Where the starting YAML lives |
| `ORYCA_API_LOG_CONSUMER_ENABLED` | `true` | Set `false` to stop writing request logs to MongoDB |

**Mongo pool.** `ORYCA_API_DB_MAX_POOL_SIZE` (50), `_MIN_POOL_SIZE` (10),
`_SOCKET_TIMEOUT` (20s).
**Redis.** `_PASSWORD`, `_POOL_SIZE` (50), `_MIN_IDLE_CONNS` (10),
`_DIAL_TIMEOUT` (5s), `_READ_TIMEOUT` and `_WRITE_TIMEOUT` (3s),
`_EXPIRES` (300s, the general cache TTL), `_ROUTING_TTL` (86400s, how long
routing data survives without the control plane).
**HTTP timeouts.** `ORYCA_API_READ_TIMEOUT` (60s), `_READ_HEADER_TIMEOUT` (10s),
`_IDLE_TIMEOUT` (120s).
**Sync.** `ORYCA_CP_SYNC_CHANNEL` (`oryca:sync-events`), which must match the
gateway's.

---

## Both

`LOG_FORMAT` takes `console` for readable output, anything else for JSON.

---

## What is not an environment variable

Settings you change while it runs live in the database, not here. Sign-up on or off,
the terms and privacy text, token lifetimes, email templates, packages and their
rate limits. They start from the YAML in `control-plane/seed/`, which is
read once against an empty database. After that the database wins, so editing in
the portal survives a restart.

Administrator credentials are deliberately not in those files, since the
repository is public. They come from `ORYCA_API_ROOT_EMAIL` and
`ORYCA_API_ROOT_PASSWORD`.
