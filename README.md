# ORYCA

The open-source gateway for geospatial APIs.

Put ORYCA in front of your spatial services and it handles the parts you would
otherwise build yourself: authentication, rate limiting, caching, and response
rewriting. It comes with a developer portal, so people can sign up, get an API
key, and read your docs on their own.

It knows the OGC API shapes (Features, Tiles, Styles, STAC, SensorThings), and
plain REST and static JSON work too.

### What "knows" means

ORYCA does not implement these standards. It sits in front of servers that do,
and understands their shapes well enough to do two things: suggest the paths a
service of that kind usually exposes, and rewrite the links in its answers so
clients keep talking to the gateway instead of wandering off to the server
behind it.

| Standard | Version we follow | Note |
|---|---|---|
| OGC API - Features | Part 1: Core 1.0 | Also published as ISO 19168-1 |
| OGC API - Tiles | Part 1: Core 1.0 | |
| OGC API - Styles | Part 1: Core | Still a draft, so our path hints may change with it |
| SensorThings API | 1.0, 1.1 | Conformance is advertised on the landing page, not at /conformance |
| STAC API | 1.0 | A community specification, not an OGC standard |

Each service you register records the version its own upstream implements, which
is the number that actually matters to a client.

> **Status: under construction.** The gateway and control plane work today. The
> portal is not built yet, a placeholder page stands in for it.

## Quickstart

You need Docker.

```sh
git clone https://github.com/oryca/oryca.git
cd oryca
cp .env.example .env
docker compose up
```

That starts MongoDB, Redis, the control plane, the gateway, and the portal.

An admin account is created on first start. If you left
`ORYCA_API_ROOT_PASSWORD` empty, the password is generated and printed once:

```sh
docker compose logs control-plane | grep -A2 "ROOT ACCOUNT"
```

| Service | URL |
|---|---|
| Portal | http://localhost:3000 |
| Gateway | http://localhost:9002/gateway/api/health |
| Control plane | http://localhost:9001/control-plane/api/v1/health |

## How it fits together

Two Go services and a web portal, on MongoDB and Redis.

```
portal  ──►  control-plane  ◄──►  gateway  ──►  your upstream services
                   │                  │
                MongoDB            Redis
```

**gateway** handles the traffic. Every request goes through it: find the route,
check the API key or token, apply rate limits, serve from cache, rewrite the
response. Its config comes from the control plane and lives in Redis, so it
keeps serving even while the control plane restarts.

**control-plane** handles the management. Users, services, packages, API keys,
the portal's API, and the dashboard.

**portal** is one web interface for everyone, with the menu limited by role.

### Layout

```
cmd/oryca-gateway/         entry point, three lines
cmd/oryca-control-plane/   entry point, three lines
gateway/                   the traffic side
control-plane/             the management side
portal/                    the web interface
docs/sync-contract.md      what the two services promise each other
```

The servers live in `gateway/app` and `control-plane/app`, so another project can
embed one instead of rebuilding its setup. That is also why nothing sits behind
`internal/`.

## Configuration

`.env` at the root configures everything. Every value has a working default
except the secret the two services share.

Starting data (settings, email templates, the first package) sits in
`control-plane/seed/` as YAML. It loads once, against an empty database. After
that the database wins: change things in the portal and a restart will not undo
your work.

Admin credentials are not in those files. They come from `ORYCA_API_ROOT_EMAIL`
and `ORYCA_API_ROOT_PASSWORD`.

Two settings behave differently from the rest: `ORYCA_PORTAL_API_URL` and
`ORYCA_PORTAL_GATEWAY_URL`. The portal calls both services from the visitor's
browser, so they have to be addresses your users can reach, and Next.js writes
them into the page bundle while the image is built. Serving the stack from a real
domain therefore means rebuilding the portal:

```sh
docker compose build portal && docker compose up -d
```

## Development

One Go module, so everything runs from the root:

```sh
go build ./...
go test ./...

go run ./cmd/oryca-control-plane
go run ./cmd/oryca-gateway
```

The two binaries never import each other. They talk over HTTP and Redis, see
[docs/sync-contract.md](docs/sync-contract.md), which is what lets the gateway
run without any database credentials. `boundary_test.go` fails the build if that
ever changes.

With the stack up, `tools/smoke-test.sh` walks the whole path: sign in, publish a
service, issue a key, call it through the gateway, and check it reaches the
dashboard.

## Docs

- [Response transforms](docs/response-transforms.md) — rewriting an answer on its
  way back, and the presets that do it for you
- [The gateway ↔ control plane contract](docs/sync-contract.md) — what the two
  services promise each other

## Contributing

Issues and questions are welcome, see [CONTRIBUTING.md](CONTRIBUTING.md). Found a
security problem? [SECURITY.md](SECURITY.md).

## License

[Apache 2.0](LICENSE)
