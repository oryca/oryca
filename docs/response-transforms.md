# Response transforms

A transform edits an answer on its way back from your upstream server, before it
reaches the caller. The usual reason is links: an API that answers with its own
address sends clients straight past the gateway, losing the API key, the rate
limit and the cache along with it.

If that is all you need, do not read this page. Open the service, apply a preset,
and you are done. Come back when you want something the presets do not cover.

## What a transform is made of

```json
{
  "name": "Keep links inside the gateway",
  "serviceId": "6a7e7e3bd894661639f3e19c",
  "enabled": true,
  "match": { "path": "/*", "methods": ["GET"] },
  "rules": [ ... ]
}
```

A transform belongs to one service. `match` decides which of that service's
requests it applies to, and `rules` say what to change.

### match

| Field | Meaning |
|---|---|
| `path` | A resource path of the service. `/*` is everything, `/collections/*` is everything below that prefix, `/collections` is that path only |
| `methods` | Empty means every method |

Only one transform runs per request: the first that matches, most specific path
first. Two transforms on the same paths is not a way to stack rules — put the
rules in one transform instead.

## Rules

```json
{
  "type": "json",
  "target": "body",
  "action": "replace",
  "path": "$.links[*]",
  "params": { "field": "href", "find": "https://upstream.example", "replace": "{{oryca_gateway_url}}" }
}
```

| Field | Values |
|---|---|
| `type` | `json` or `xml`. Left out, the body's shape decides |
| `target` | `body` or `headers` |
| `action` | `replace`, `add`, `append`, `remove`, `rename` |
| `path` | JSONPath for a body rule. `$.links[*]`, `$.collections[*].links[*]`, `$..*` for everything |
| `xpath` | XPath, for an XML rule |
| `headerName` | Which header, for a header rule |
| `params` | What the action works with, below |
| `conditions` | Run this rule only when the answer looks a certain way, below |

### params

| Field | Used by | Meaning |
|---|---|---|
| `find` / `replace` | `replace` | Plain text replacement inside a string |
| `regex` / `replace` | `replace` | Same, but the match is a regular expression |
| `field` | `replace` | On an object, only touch this field. Left out, every string field of the object is replaced |
| `value` | `add`, `append` | The value to write |
| `from` / `to` | `rename` | Old and new field name |
| `separator` | `append` | What to join with |

`replace` reads the shape of what it matched: a string is edited directly, an
object has its `field` edited (or all its string fields), and anything else is
overwritten with `value`.

### Two placeholders

These are filled in per request, so one rule serves every caller:

| Placeholder | Becomes |
|---|---|
| `{{oryca_gateway_url}}` | The public address of this service through the gateway, e.g. `https://gateway.example.com/gateway/api/resources/my-service` |
| `{{oryca_auth}}` | The caller's own credential as a query parameter: `api_key=…` or `token=…` |

`GET /response-transforms/variables` returns the same list.

### conditions

Conditions decide **whether the rule runs at all**, not which matches it edits.
If any one of the things the path matched satisfies a condition, the rule then
applies to all of them.

```json
{
  "path": "$.links[*]",
  "conditions": [{ "field": "rel", "equals": ["self", "alternate"] }],
  "params": { "field": "href", "find": "https://upstream.example", "replace": "{{oryca_gateway_url}}" }
}
```

Given a links array holding one `self` and one `license`, that rule rewrites
both: the `self` link satisfied the condition, which switched the rule on for the
whole array. Read them as "only bother with this rule when the answer looks like
this", and use the path to say what to edit.

`equals` and `notEquals` both take a list, and values are compared as text.

## What is left alone

- **A body that did not arrive whole.** Very large answers are streamed straight
  through and never buffered, so no rule sees them.
- **Anything that is not text.** Images, tiles and other binary answers are not
  parsed.
- **Answers that are not 2xx.** An error from the upstream is passed on as it is.

## Writing one by hand

Start from a preset and edit it — that is faster than starting from nothing, and
it comes back filled in with the right upstream address:

```sh
curl -X POST "$CP/response-transforms/presets/features-links/apply" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"serviceId":"…"}'
```

Then `PUT /response-transforms/:id` with your changes. Saving tells the gateway
at once, so the next request already sees it.

Check it the way a client would, by reading the answer that comes back through
the gateway rather than the one your upstream sends:

```sh
curl -s "$GW/resources/my-service/collections" | python3 -m json.tool | grep href
```

## When a link keeps pointing at the upstream

Usually correct. A rule only rewrites addresses that start with the upstream
address behind *this* service, so:

- A link to another site — a licence, a repository, a data file on object
  storage — is left alone, which is what you want.
- A link into a part of the upstream server your service does not cover is also
  left alone. Rewriting it would send clients to a path the gateway answers 404
  for. Register that path on the service if you want it proxied.

## Cost

Rules run on every matching request, on a body the gateway has to hold in memory
to parse. `$..*` walks the whole document, so prefer a path that names what you
mean. Regular expressions are compiled once and reused.
