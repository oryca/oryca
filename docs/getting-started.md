# Publish your first API

You have the stack running and you are logged into the portal as the
administrator. This walks through putting one API behind the gateway and calling
it with a key.

It takes four things, in this order.

| | What it is |
|---|---|
| **Upstream source** | One address the gateway forwards to. Yours, or someone else's. |
| **Service** | The route you publish. Its paths point at sources. |
| **Package** | A rate limit, plus the list of service paths it may reach. |
| **API key** | What a developer sends. It works because their package grants the path. |

---

## 1. Publish the service

**Manage Services** → **Publish Service**.

Fill in a name, and a base path such as `/weather`. That base path is where your
API will live on the gateway, so a request looks like this.

```
https://your-gateway/gateway/api/resources/weather/<path>
```

Leave the type as **General** for an ordinary REST or JSON API. Pick an OGC type
only if the upstream really implements that standard. ORYCA uses it to suggest
paths and to offer the right link-rewrite preset.

Under **Resource Paths**, each row is one route you are publishing. For each row,
pick where it goes from the dropdown; if you have no upstream yet, choose
**+ New upstream** and add it right there.

### The one thing worth getting right

**An upstream source URL is the whole destination address, not a prefix.** The
resource path is only the name the gateway publishes it under. The two are
matched up row by row, and nothing is appended.

So to publish an upstream's `/observations` endpoint as `/latest`, the row reads.

| Resource path | Upstream source URL |
|---|---|
| `/latest` | `https://api.upstream.example/v2/observations` |

Not `https://api.upstream.example/v2`, which would forward to the wrong place.

Two things do get filled in.

- **`{param}` placeholders.** A resource path of `/collections/{collectionId}`
  with a source URL of `https://upstream.example/collections/{collectionId}`
  substitutes whatever the caller asked for.
- **A trailing `*`.** A resource path of `/tiles/*` appends the rest of the
  request onto the end of the source URL, so one row covers a whole subtree.

### When the upstream does not have the endpoint

Sometimes a server is missing a path its standard expects. Choose **Answer with a
fixed body** instead of a URL, and the gateway replies with what you write.
Nothing is forwarded.

That is genuinely useful for a landing page of links. Be careful with
`/conformance`, because that document declares which conformance classes the server
implements, so writing one here says that on its behalf. List only what it really
does, or clients will call features that are not there.

Save. If the service is an OGC type, ORYCA offers to apply its link-rewrite
preset. Say yes, unless you plan to write the rules yourself. See
[response transforms](response-transforms.md) for what those rules do.

---

## 2. Let a package reach it

A key only opens the doors its package lists.

**Packages & Users** → pick a package (a fresh install has one) → **Link
Service**. Choose the service, tick the paths, and set the rate limit.

Anyone who signs up gets the default package, so this one step is usually all it
takes for every future developer.

---

## 3. Get a key and call it

**Services & Keys** → **Create new key**. The key is shown once, so copy it now.

Then open **Sandbox**. Pick the service, the path and the key, and send. You get
the status, how long it took, the body, the headers with the rate-limit ones
pulled out, and the same call written as `curl`.

From a terminal it looks like this.

```sh
curl "http://localhost:9002/gateway/api/resources/weather/latest" \
  -H "X-API-Key: <your-key>"
```

Watch `RateLimit-Remaining` come down as you repeat it, and `429` when it runs
out.

---

## When it does not work

**401.** No key, or the key is wrong. Check the `X-API-Key` header.

**404 with `"detail":"Route not found"`.** The gateway has no such route.
Either the base path or the resource path does not match what you called, the
service is disabled, or the caller's package does not grant this path, so back to
step 2. Changes reach the gateway within a second; if it has been longer than a
minute, check that both services point at the same Redis database (see
[the sync contract](sync-contract.md)).

**405.** The route exists, but not for that method. Add it to the row's methods.

**404 with something else in the body.** That 404 came from your upstream and
was passed through. The route is fine; the source URL is pointing at the wrong
place. Re-read step 1.

**502.** The gateway could not reach the upstream at all. Wrong host, or it is
down. If the upstream runs in the same compose file, use its service name, not
`localhost`.

---

## What to look at next

- [Response transforms](response-transforms.md) covers rewriting a response on its
  way back, and the presets that do it for you.
- [Gateway and control plane](sync-contract.md) records what the two services promise
  each other, and what to check when a change does not arrive.
- `tools/smoke-test.sh` is this whole walkthrough as a script, if you would rather
  read it in one piece.
