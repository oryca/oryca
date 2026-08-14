# Portal

The web interface for ORYCA. One app for everyone: people who consume the APIs
and the people who publish them, with the menu limited by role.

## Running it

The portal needs the rest of the stack. From the repository root:

```sh
docker compose up -d
```

Then open http://localhost:3000.

To work on the portal itself, run the backend in Docker and the portal on your
machine:

```sh
docker compose up -d control-plane gateway mongo redis
cd portal && npm install && npm run dev
```

## How it talks to the backend

Every call is made from the browser, never from the Next.js server. That keeps
one address correct for both: the one your users type. Two settings carry it,
and Next.js writes them into the page bundle while building, so changing either
means rebuilding:

| Setting | Default |
|---|---|
| `NEXT_PUBLIC_API_URL` | `http://localhost:9001/control-plane/api/v1` |
| `NEXT_PUBLIC_GATEWAY_URL` | `http://localhost:9002/gateway/api` |

In Docker they come from `ORYCA_PORTAL_API_URL` and `ORYCA_PORTAL_GATEWAY_URL`
in the root `.env`, passed in as build arguments.
