# ORYCA

[![CI](https://github.com/oryca/oryca/actions/workflows/ci.yml/badge.svg)](https://github.com/oryca/oryca/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/oryca/oryca?sort=semver)](https://github.com/oryca/oryca/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/oryca/oryca)](go.mod)
[![License](https://img.shields.io/github/license/oryca/oryca)](LICENSE)

**An open-source API gateway, at home with geospatial APIs.**

[Quick Start](#quick-start) · [Configuration](#configuration) · [Docs](docs) · [Contributing](CONTRIBUTING.md)

Put ORYCA in front of any HTTP API — your own REST and JSON service, or an OGC API Features, STAC, Tiles, Styles or SensorThings server — and it handles four things you would otherwise build yourself:

- **API keys.** Issues them, checks them on every request.
- **Rate limits.** Per package, per path, so an upstream server is not the thing that fails first.
- **Response rewriting.** Change a response on its way back: swap an address, hide an upstream's own key, add a header. This is what keeps an API that answers with links to itself from sending clients straight past the gateway.
- **Developer portal.** Sign-up, keys, docs, and a way to try a call in the browser.

Any API gets all four. What geospatial servers get on top is a gateway that knows
their shapes: suggested paths per standard, and ready-made rewrite presets — see
[the OGC section](#what-the-ogc-support-is-and-is-not).

---

## Quick Start

You need Docker. Nothing else, no Go and no Node.

### 1. Get the stack running
```sh
curl -fsSL -o docker-compose.yml https://raw.githubusercontent.com/oryca/oryca/main/docker-compose.images.yml
curl -fsSL -o .env https://raw.githubusercontent.com/oryca/oryca/main/.env.example
docker compose up -d
```

That pulls the published images. To build from source instead:
```sh
git clone https://github.com/oryca/oryca.git
cd oryca && cp .env.example .env
docker compose up -d --build
```

### 2. Find your admin password
An administrator account is created on the first start, with a generated password printed once:
```sh
docker compose logs control-plane | grep -A2 "ROOT ACCOUNT"
```

### 3. Open it
- **Developer Portal:** [http://localhost:3000](http://localhost:3000) — log in as `admin@localhost`
- **Control Plane API:** [http://localhost:9001/control-plane/api/v1/health](http://localhost:9001/control-plane/api/v1/health)
- **Gateway:** [http://localhost:9002/gateway/api/health](http://localhost:9002/gateway/api/health)

### 4. Put an API behind it
[Publish your first API](docs/getting-started.md) walks through the four pieces —
upstream, service, package, key — and the one detail that trips everyone up.

---

## Features

**For any API**

- **Try it in the browser.** The portal calls the gateway with your key and shows what comes back, rate-limit headers included (`RateLimit-Limit`, `RateLimit-Remaining`).
- **Request charts and logs.** Volume, response time, status breakdown, and a searchable log. Each person sees their own traffic, administrators see everyone's.
- **Self-service sign-up.** Developers register, get a key, and read the docs without an administrator in the loop.
- **Rewrite rules you write yourself.** JSON by JSONPath, XML by XPath, headers by name — see [response transforms](docs/response-transforms.md).
- **Serve a path yourself.** A route can answer from a fixed body instead of proxying, for the endpoints your upstream does not have.

**For geospatial servers, on top**

- **Path suggestions per standard.** Pick a service type and ORYCA fills in the paths that kind of server usually exposes (`/conformance`, `/collections`), instead of you typing them by hand.
- **Rewrite presets, including XML.** One click covers JSON responses and capabilities documents, where both the address and the upstream's own API key sit inside attributes.

---

## How it Works

Two Go services and a Next.js portal, on MongoDB and Redis.

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

1. **Gateway** carries the traffic: find the route, check the key, apply the rate limit, serve from cache, rewrite the response. Its configuration comes from the control plane and lives in Redis, so it keeps serving while the control plane restarts. It never opens a database connection of its own.
2. **Control plane** handles management: users, services, packages, API keys, the portal's API, and the dashboard. It owns MongoDB, and uses Redis to push changes and to read the request logs the gateway writes.
3. **Portal** is one web interface for everyone, with the menu limited by role.

The two Go services never import each other. They talk over HTTP and Redis, written down in [sync-contract.md](docs/sync-contract.md).

### What the OGC support is, and is not

ORYCA does not implement these standards. It sits in front of servers that do, and knows their shapes well enough to suggest paths and rewrite links.

| Standard | Version the hints follow | Note |
| :--- | :--- | :--- |
| OGC API - Features | Part 1: Core 1.0 | Also published as ISO 19168-1 |
| OGC API - Tiles | Part 1: Core 1.0 | |
| OGC API - Styles | Part 1: Core | Still a draft, so the hints may change with it |
| SensorThings API | 1.0, 1.1 | Advertises conformance on the landing page, not at `/conformance` |
| STAC API | 1.0 | A community specification, not an OGC standard |

Each service you register records the version its own upstream implements, which is the number that matters to a client.

---

## Configuration

`.env` at the root configures the whole stack, and every value has a working default. [The configuration reference](docs/configuration.md) lists all of them. The one to change before anything is reachable from a network you do not control is **`ORYCA_INTERNAL_SECRET`**, which the two Go services share. Rotating it? Put the old value in `ORYCA_INTERNAL_SECRET_PREV` first, and the control plane accepts both until you remove it.

Running the binaries yourself, without compose? Then `ORYCA_API_REDIS_DB` and `ORYCA_GW_REDIS_DB` have to match, and their built-in defaults do not (0 and 7). Pub/sub ignores the database number, so a mismatch looks like it works until routes stop arriving. The compose files pin both to 0 for you.

Starting data (settings, email templates, the first package) is YAML in `control-plane/seed/`. It loads once, against an empty database. After that the database wins: change things in the portal and a restart will not undo your work. Administrator credentials are deliberately not in those files, they come from `ORYCA_API_ROOT_EMAIL` and `ORYCA_API_ROOT_PASSWORD`.

One pair is easy to get wrong. `ORYCA_PORTAL_API_URL` and `ORYCA_PORTAL_GATEWAY_URL` are the addresses the portal calls **from the visitor's browser**, so they have to be reachable by your users — a container name like `http://control-plane:9001` will not work. Serving the stack from your own domain means setting both, then:
```sh
docker compose up -d portal
```

---

## Repository Layout

- [`portal/`](portal): the web interface, for developers and administrators alike.
- [`gateway/`](gateway): the traffic side, in Go.
- [`control-plane/`](control-plane): the management side, in Go.
- [`cmd/`](cmd): entry points, three lines each. The servers themselves live in `gateway/app` and `control-plane/app`, so another project can embed one instead of rebuilding its setup. That is also why nothing is hidden behind `internal/`.
- [`tools/`](tools): `smoke-test.sh`, which checks a running stack end to end.
- [`docs/`](docs): how to use it, and how the pieces agree with each other.
  - [`getting-started.md`](docs/getting-started.md): publish your first API, from upstream to a working key.
  - [`configuration.md`](docs/configuration.md): every environment variable, and what is not one.
  - [`response-transforms.md`](docs/response-transforms.md): rewriting a response on its way back, and the presets that do it for you.
  - [`sync-contract.md`](docs/sync-contract.md): what the gateway and the control plane promise each other.

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

With the stack up, `tools/smoke-test.sh` walks the whole path a person takes: sign in, publish a service, issue a key, call it through the gateway, and check it reaches the dashboard.

```sh
./tools/smoke-test.sh
```

---

## Contributing

Issues and questions are welcome, see [CONTRIBUTING.md](CONTRIBUTING.md) and our [Code of Conduct](CODE_OF_CONDUCT.md). Found a security problem? [SECURITY.md](SECURITY.md).

## License

Licensed under the [Apache 2.0 License](LICENSE).
