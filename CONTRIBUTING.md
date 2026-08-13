# Contributing

Thanks for looking. The project is young, so the rules are short.

## Run it

```sh
cp .env.example .env
docker compose up -d
./tools/smoke-test.sh
```

The smoke test signs in, publishes a service, calls it through the gateway, and
checks it shows up on the dashboard. If it passes, your setup is fine.

## Before you open a pull request

```sh
go build ./...
go test ./...
gofmt -l .
```

One rule to know: the gateway and the control plane never import each other. They
talk over HTTP and Redis, described in [docs/sync-contract.md](docs/sync-contract.md).
`boundary_test.go` fails the build if that changes.

## Commit messages

Start with what the commit does:

```
[Add] dashboard summary endpoint
[Fix] rate limit counter reset
[Update] bump go-redis to v9.22
```

## Not sure about something?

Open an issue and ask. A question is a fine reason to open one.

Found a security problem? Please read [SECURITY.md](SECURITY.md) first.
