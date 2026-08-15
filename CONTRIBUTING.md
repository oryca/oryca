# Contributing

Thanks for looking. The project has just been opened, and we are still settling
how it will take contributions.

## Right now

**Issues and questions are welcome.** Bug reports, questions about how something
works, and ideas for what it should do next are the most useful thing you can
send today. A question is a fine reason to open an issue.

**Pull requests are not open yet, but they will be soon.** We would rather say so
than leave one waiting. When the process is ready, this file will say how it
works.

Found a security problem? Please read [SECURITY.md](SECURITY.md) first.

## Running it yourself

```sh
cp .env.example .env
docker compose up -d
./tools/smoke-test.sh
```

The smoke test signs in, publishes a service, calls it through the gateway, and
checks it shows up on the dashboard. If it passes, your setup is fine.

## Reading the code

```sh
go build ./...
go test ./...
gofmt -l .
```

There is one rule to know. The gateway and the control plane never import each other. They
talk over HTTP and Redis, described in [docs/sync-contract.md](docs/sync-contract.md).
`boundary_test.go` fails the build if that changes.
