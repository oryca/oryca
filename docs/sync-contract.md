# Synchronization Protocol: Gateway ↔ Control Plane

The gateway never imports control-plane code and never connects to MongoDB. It gets routes, API keys and user accounts, and sends its request logs back, over two channels: **HTTP polling** and **Redis**.

```
                   ┌────────────────────────────────────────┐
                   │          ORYCA Control Plane           │
                   └─────┬──────────────┬─────────────┬─────┘
                         ▲              ▼             ▲
                         │              │             │
                     HTTP poll       Pub/Sub       Log Stream
                  (gateway asks)   (CP pushes)   (gateway logs)
                         │              │             │
                   ┌─────┴──────────────┴─────────────┴─────┐
                   │             ORYCA Gateway              │
                   └────────────────────────────────────────┘
```

So if the control plane or MongoDB goes offline, the gateway keeps proxying traffic from its cached routing tables. Nothing new arrives until they are back.

> [!IMPORTANT]
> Both services must point at the **same Redis database**: `ORYCA_API_REDIS_DB` and `ORYCA_GW_REDIS_DB` have to match. Pub/Sub ignores the database number, so a mismatch looks healthy — events arrive, but the cached routing tables never do.

---

## 1. HTTP Polling (The Source of Truth)

The gateway polls the control plane's read-only `/internal/*` routes to synchronize state. These internal requests are secured by matching `X-Internal-Key` headers (configured via `ORYCA_INTERNAL_SECRET`). To rotate that secret without dropping calls, set the old value as `ORYCA_INTERNAL_SECRET_PREV` on the control plane: it accepts both until you remove it.

| Endpoint | Data Synced | Frequency |
| :--- | :--- | :--- |
| `GET /internal/services` | Base paths, routing configurations, and rate-limiting packages | Every 60s |
| `GET /internal/sources` | Mapped upstream target URLs for each routing path | Polled with services |
| `GET /internal/api-keys` | Active developer API keys and their assigned tiers | Every 30s |
| `GET /internal/response-transforms` | Link-rewrite rules and transform definitions | Polled with services |
| `GET /internal/users/{id}` | User credentials, roles, and status on cache misses | On-demand (cache miss) |
| `GET /.well-known/jwks.json` | Public signing key for validating JWT session tokens | On start, then every 5 min |

*Note: All listing endpoints return an `ETag` header. The gateway includes `If-None-Match` in poll requests, allowing the control plane to return a lightweight `304 Not Modified` response if no changes occurred.*

---

## 2. Redis Pub/Sub (Real-Time Push Events)

To prevent delays between an administrator making changes in the portal and the gateway applying them, the control plane publishes real-time events over a Redis Pub/Sub channel (`oryca:sync-events`).

Pub/Sub is **at-most-once**: an event published while the gateway is restarting is gone. That is on purpose, the 60-second poll above is what makes it safe. Push is the shortcut, polling is the guarantee.

### Event Payload Envelope
```json
{
  "type": "apikey",
  "version": 1786551735509,
  "payload": { ... }
}
```
*The `version` field tracks change timestamps. The gateway ignores incoming payloads older than its currently cached version to guarantee out-of-order messages do not corrupt newer states.*

### Event Types
- **`reload_services`:** Instructs the gateway to immediately trigger an HTTP reload of services, sources, and transforms.
- **`apikey`:** Pushes a created, edited, or deleted API key directly to the gateway cache.
- **`user`:** Updates a single user snapshot in the gateway's cache.
- **`user_invalidate`:** Bulk invalidates a list of users, prompting the gateway to re-authenticate them on their next request.

---

## 3. Redis Log Stream (Usage Logs)

The gateway records every request it proxies and sends them to the control plane, which is where the dashboard reads them from:

1. **Upload:** The gateway appends traffic entries to the Redis log stream (`stream:usage-log`) using `XADD`.
2. **Process:** The control plane reads entries via consumer group `cp-log-consumer` in batches of up to 100 or every 1 second, writing them to MongoDB (`access_logs` collection).
3. **Acknowledge:** The control plane sends an acknowledgment (`XACK`) only after MongoDB confirms saving the logs, guaranteeing **at-least-once delivery** of all access logs.

### Log Entry Schema
```json
{
  "time": "2026-08-12T16:09:28.641Z",
  "level": "INFO",
  "message": "proxy",
  "hostname": "gateway-1",
  "service": "oryca-gateway",
  "traceId": "9a12b...",
  "userId": "6d82e...",
  "apiKeyId": "5f11a...",
  "serviceId": "7c34b...",
  "request": {
    "host": "localhost",
    "ip": "127.0.0.1",
    "method": "GET",
    "path": "/gateway/api/resources/my-service/collections"
  },
  "response": {
    "statusCode": 200,
    "size": 9593,
    "duration": 1186
  }
}
```
