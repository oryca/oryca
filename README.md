# ORYCA

**The friendly, open-source API gateway built specifically for geospatial services.**

Put ORYCA in front of your spatial servers (OGC API Features, STAC, Tiles, Styles, SensorThings, or standard REST/JSON APIs) and it instantly handles:
- **Authentication:** Issues and validates developer API keys.
- **Rate Limiting:** Protects upstream servers with tier-based rate limit quotas.
- **Response Rewriting (Link Rewriter):** Translates upstream links so clients stay routed through the gateway.
- **Developer Portal:** A ready-to-use Next.js web portal where users can register, manage API keys, and test endpoints.

---

## Key Features

- **Knows the OGC shapes:** Pick a service type and ORYCA suggests the paths that kind of server usually exposes (`/conformance`, `/collections`), so you are not typing them by hand.
- **Dynamic Link Rewrites:** Ready-to-use presets rewrite links in geospatial JSON and XML responses (e.g. WMTS Capabilities) on the fly.
- **Interactive Developer Console:** Offers a **"Try It"** testing tool right in the browser, showing raw rate-limit headers (`RateLimit-Limit`, `RateLimit-Remaining`).
- **Telemetry Dashboard:** Visual charts and searchable request logs. Each person sees their own traffic, administrators see everyone's.
- **Self-Service Access:** Developers sign up, retrieve keys, and review documents independently.

### What "knows the OGC shapes" does not mean

ORYCA **does not implement** these standards. It sits in front of servers that do, and understands their shapes well enough to suggest paths and rewrite links.

| Standard | Version the hints follow | Note |
| :--- | :--- | :--- |
| OGC API - Features | Part 1: Core 1.0 | Also published as ISO 19168-1 |
| OGC API - Tiles | Part 1: Core 1.0 | |
| OGC API - Styles | Part 1: Core | Still a **draft**, so the hints may change with it |
| SensorThings API | 1.0, 1.1 | Advertises conformance on the landing page, not at `/conformance` |
| STAC API | 1.0 | A community specification, **not** an OGC standard |

Each service you register records the version its own upstream implements, which is the number that actually matters to a client.

---

## How it Works

ORYCA consists of two Go microservices and a Next.js web portal:

```
                  ┌────────────────────────────────────────┐
                  │            Next.js Portal              │
                  │   (Dashboard, Keys, Admin Controls)    │
                  └───────┬────────────────────────┬───────┘
                          │ browser calls both     │
                          ▼                        ▼
             ┌───────────────────┐  polls   ┌────────────────────────┐
   clients ─►│   ORYCA Gateway   │ ───────► │  ORYCA Control Plane   │
             │  (Traffic Proxy)  │ ◄─────── │   (Management API)     │
             └─────────┬─────────┘  events  └───────────┬────────────┘
                       │                                │ MongoDB
                       │        ┌──────────────┐        ▼
                       └───────►│    Redis     │◄──  ┌──────────────────┐
                    routes,     └──────────────┘     │ Users, Keys,     │
                    cache,        shared by both     │ Config, Logs     │
                    rate limit                       └──────────────────┘
```

1. **ORYCA Gateway:** High-performance proxy handling traffic routing, authentication checks, and rate-limiting. Its configuration comes from the control plane and lives in Redis, so it keeps serving while the control plane restarts. It never opens a database connection of its own.
2. **ORYCA Control Plane:** Management backend coordinates admin configs, users list, packages, and stores logs in MongoDB. It uses Redis too, to push changes and to read the request logs the gateway writes.
3. **Developer Portal:** Frontend dashboard for developers and administrators.

The two Go services never import each other. They talk over HTTP and Redis, written down in [sync-contract.md](docs/sync-contract.md).

---

## Quick Start

Start MongoDB, Redis, the gateway, control plane, and portal using Docker Compose:

### 1. Clone & Spin Up
```sh
git clone https://github.com/oryca/oryca.git
cd oryca
cp .env.example .env
docker compose up -d --build
```

That builds the three services from source, which takes a few minutes the first time. To skip the build and use the published images instead:
```sh
docker compose -f docker-compose.images.yml up -d
```

### 2. Find Your Admin Credentials
An administrator account is created on the first start. Run this command to fetch your generated password:
```sh
docker compose logs control-plane | grep -A2 "ROOT ACCOUNT"
```

### 3. Access Services
- **Developer Portal:** [http://localhost:3000](http://localhost:3000) (Log in with `admin@localhost` and the generated password)
- **Control Plane API:** [http://localhost:9001/control-plane/api/v1/health](http://localhost:9001/control-plane/api/v1/health)
- **Gateway Server:** [http://localhost:9002/gateway/api/health](http://localhost:9002/gateway/api/health)

---

## Configuration

`.env` at the root configures the whole stack. Every value has a working default except:

- **`ORYCA_INTERNAL_SECRET`** — the two Go services share it. Change it before anything is reachable from a network you do not control. Rotating it? Put the old value in `ORYCA_INTERNAL_SECRET_PREV` so calls in flight keep working.
- **`ORYCA_API_REDIS_DB` and `ORYCA_GW_REDIS_DB`** — these must match. Pub/sub ignores the database number, so a mismatch looks like it works until routes stop arriving.

Starting data (settings, email templates, the first package) is YAML in `control-plane/seed/`. It loads once, against an empty database. After that the database wins: change things in the portal and a restart will not undo your work. Administrator credentials are deliberately not in those files, they come from `ORYCA_API_ROOT_EMAIL` and `ORYCA_API_ROOT_PASSWORD`.

**Two settings behave differently.** `ORYCA_PORTAL_API_URL` and `ORYCA_PORTAL_GATEWAY_URL` are the addresses the portal calls **from the visitor's browser**, so your users have to be able to reach them. Next.js writes them into the page bundle while the image is built, so serving the stack from your own domain means rebuilding the portal:
```sh
docker compose build portal && docker compose up -d
```

---

## Repository Layout

- [`portal/`](portal): Next.js developer dashboard and administrator console.
- [`gateway/`](gateway): High-speed proxy built in Go.
- [`control-plane/`](control-plane): Management API built in Go.
- [`cmd/`](cmd): Entry points, three lines each. The servers themselves live in `gateway/app` and `control-plane/app`, so another project can embed one instead of rebuilding its setup. That is also why nothing is hidden behind `internal/`.
- [`docs/`](docs): How the pieces agree with each other.
  - [`response-transforms.md`](docs/response-transforms.md): Custom rewrite rules and parameters.
  - [`sync-contract.md`](docs/sync-contract.md): Synchronization interface protocol between Gateway and Control Plane.

---

## Local Development & Tests

One Go module, so everything runs from the root:

```sh
# Build and run tests
go build ./...
go test ./...

# Start control plane
go run ./cmd/oryca-control-plane

# Start gateway
go run ./cmd/oryca-gateway
```

`boundary_test.go` fails the build if one binary ever imports the other.

Once the stack is running, run the automated integration test to verify the complete setup:
```sh
./tools/smoke-test.sh
```

It walks the whole path a person takes: sign in, publish a service, issue a key, call it through the gateway, and check it reaches the dashboard.

---

## Contributing

Issues and questions are welcome, see [CONTRIBUTING.md](CONTRIBUTING.md). Found a security problem? [SECURITY.md](SECURITY.md).

## License

Licensed under the [Apache 2.0 License](LICENSE).
