# The gateway ↔ control plane contract

The gateway never imports control-plane code and never opens a MongoDB
connection. Everything it knows about services, keys and users arrives over the
two channels described here, and `boundary_test.go` in the repository root fails
the build if that separation is ever broken by an import.

Both sides therefore have to agree on these payloads by hand. When you change one
side, change the other in the same commit.

## 1. HTTP — the source of truth

The control plane exposes a small read-only surface under `/internal/*`, guarded
by the `X-Internal-Key` header. The secret is `ORYCA_INTERNAL_SECRET` on the
control plane and `ORYCA_GW_INTERNAL_SECRET` on the gateway; they must match.
`ORYCA_INTERNAL_SECRET_PREV` accepts the previous value while a rotation is
rolling out.

| Endpoint | Read by the gateway | Cadence |
|---|---|---|
| `GET /internal/services` | routing table: base paths, resource paths, per-package rate limits | every 60s (`ORYCA_GW_SERVICE_POLL_INTERVAL`) |
| `GET /internal/sources` | upstream targets each resource path forwards to | with services |
| `GET /internal/api-keys` | active keys and their owners | every 30s (`ORYCA_GW_APIKEY_POLL_INTERVAL`) |
| `GET /internal/response-transforms` | response rewrite rules | with services |
| `GET /internal/users/{id}` | one user's authorization snapshot, on cache miss | on demand |
| `GET /.well-known/jwks.json` | public key that signs session tokens | at start, then every 5 min |

Every list endpoint answers with an `ETag`. The gateway sends
`If-None-Match` and treats `304` as "nothing changed" — polling is therefore
cheap enough to stay the safety net even when the push channel is healthy.

## 2. Redis — the push channel

Polling alone would leave up to a full interval between an administrator's change
and the gateway acting on it. To close that gap the control plane publishes an
event on a Redis pub/sub channel, and the gateway applies it immediately.

- Channel: `oryca:sync-events` — `ORYCA_CP_SYNC_CHANNEL` and
  `ORYCA_GW_SYNC_CHANNEL`, which must match. Empty disables the push and leaves
  polling to do the work.
- Both services must also point at the same Redis **database number**
  (`ORYCA_API_REDIS_DB` and `ORYCA_GW_REDIS_DB`). Pub/sub is not namespaced per
  database, so a mismatch here looks like it works while the shared keys and the
  log stream silently go to different places.

The envelope:

```json
{ "type": "apikey", "version": 1786551735509, "payload": { } }
```

`version` is `UnixMilli` of the change on the control-plane side. The gateway
keeps the highest version it has seen per entity and drops anything older, so an
out-of-order delivery can never overwrite newer data.

| `type` | `payload` | What the gateway does |
|---|---|---|
| `reload_services` | none | re-pulls services, sources and transforms over HTTP — the collections are large, so the event only says "look again" |
| `apikey` | one API key | writes it straight into the key cache |
| `user` | one user's authorization snapshot | writes it into the freshness cache |
| `user_invalidate` | `{"userIds":[…]}` | evicts those users and re-fetches them; used by bulk operations so one admin action is one event, not one per user |

Delivery is at-most-once by design: a gateway that is restarting misses whatever
is published meanwhile, and picks the change up on its next poll. Nothing in the
gateway may depend on an event arriving.

## 3. Redis — the log stream

The traffic flows the other way for request logs.

- Stream: `stream:usage-log`, written by the gateway with `XADD`, capped at
  roughly 500,000 entries.
- Consumer group: `cp-log-consumer`, read by the control plane, which batches up
  to 100 entries or one second and writes them to the `access_logs` collection.

The control plane acknowledges a batch only after MongoDB has accepted it, so a
crash mid-write leaves the entries pending and another instance reclaims them.
That makes the pipeline at-least-once: a duplicated log line is possible, a lost
one is not.

One log entry, as published:

```json
{
  "time": "2026-08-12T16:09:28.641Z",
  "level": "INFO",
  "message": "proxy",
  "hostname": "…", "service": "oryca-gateway",
  "traceId": "…", "userId": "…", "apiKeyId": "…", "serviceId": "…",
  "request":  { "host": "…", "ip": "…", "method": "GET", "path": "/gateway/api/resources/demo/get" },
  "response": { "statusCode": 200, "size": 9593, "duration": 1186 }
}
```

`userId` and `apiKeyId` are empty for calls to a public service, which is why the
dashboard shows those requests only to administrators — there is no owner to
attribute them to.

The shape lives in `gateway/logger/log_model.go` and is parsed in
`control-plane/consumer/log_consumer.go`. Those two files are the contract.
