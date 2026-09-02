# Portal

The web interface for ORYCA. One app for everyone, people who consume the APIs
and the people who publish them, with the menu limited by role.

## Running it

The portal needs the rest of the stack. From the repository root.

```sh
docker compose up -d
```

Then open http://localhost:3000.

To work on the portal itself, run the backend in Docker and the portal on your
machine.

```sh
docker compose up -d control-plane gateway mongo redis
cd portal && npm install && npm run dev
```

## How it talks to the backend

Every call is made from the browser, never from the Next.js server. That keeps
one address correct for both, the one your users type.

Those addresses are read when a page is served, not when the image is built, so
one image works on any domain. `app/layout.tsx` puts them into the page as
`window.__ORYCA_CONFIG__`, and `lib/api.ts` reads them from there.

| Setting | Default |
|---|---|
| `ORYCA_PORTAL_API_URL` | `http://localhost:9001/control-plane/api/v1` |
| `ORYCA_PORTAL_GATEWAY_URL` | `http://localhost:9002/gateway/api` |

In Docker both come from the root `.env`. For `npm run dev`, `NEXT_PUBLIC_API_URL`
and `NEXT_PUBLIC_GATEWAY_URL` still work as a fallback.
