# Response Transforms (Link Rewriter)

A **response transform** changes a response, its headers or its body, on the way back from your upstream server to the client.

The reason it exists is that a geospatial API (OGC API Features, STAC, WMTS) answers with links to itself. A client that follows those links leaves the gateway, and with it the API key check and the rate limit. Rewriting the links keeps clients coming back.

> [!TIP]
> **Start from a preset.** The portal carries ready-made rulesets under Transforms & Presets, from single-action starters that suit any API to full link rewriters per standard. Applying one writes a transform switched off, so read what it wrote, adjust it, then enable it. Nothing meets traffic until you do.

A preset that rewrites links fills in the upstream address of your service,
minus any query string, so the upstream's own credential never lands in a stored
rule. It also swaps `api_key=...` inside rewritten links for the caller's own
credential. If your upstream names that parameter differently, `apikey` or
`subscription-key` for example, edit the regex to match.

---

## 1. Transform Structure

Each transform is attached to a service and defines which requests it intercepts.

```json
{
  "name": "Geospatial Link Rewriter",
  "serviceId": "6a7e7e3bd894661639f3e19c",
  "enabled": true,
  "match": {
    "path": "/*",
    "methods": ["GET"]
  },
  "rules": [ ... ]
}
```

- **`match.path`** is the request path pattern. `/*` matches all paths, `/collections/*` matches sub-paths.
- **`match.methods`** is a list of HTTP methods (e.g. `["GET"]`). If empty, matches all methods.
- **`rules`** is a list of rules that execute sequentially on the response.

---

## 2. Rewrite Rules

Rules describe exactly what to modify inside headers or body payloads.

```json
{
  "type": "json",
  "target": "body",
  "action": "replace",
  "path": "$.links[*]",
  "params": {
    "field": "href",
    "find": "https://upstream-server.internal",
    "replace": "{{oryca_gateway_url}}"
  }
}
```

### Core Configuration Fields

| Field | Supported Values | Description |
| :--- | :--- | :--- |
| **`type`** | `json` \| `xml` | Document format. If omitted, matching is determined by response content-type. |
| **`target`** | `body` \| `headers` | Interception target. |
| **`action`** | `replace` \| `add` \| `append` \| `remove` \| `rename` | Modification command. |
| **`path`** | JSONPath (e.g. `$.links[*]`) | Target selector inside JSON documents. |
| **`xpath`** | XPath (e.g. `//*[local-name()='ResourceURL']`) | Target selector inside XML documents. |
| **`headerName`**| string | Target header key when `target: "headers"`. |
| **`params`** | object | Configuration parameters for the selected action (see below). |
| **`conditions`**| array of objects | Restrict rule execution depending on response contents (see below). |

---

## 3. Action Parameters (`params`)

The structure of `params` depends on the selected `action`.

| Parameter | Actions | Description |
| :--- | :--- | :--- |
| **`find`** / **`replace`** | `replace` | Plain text substring replacement. |
| **`regex`** / **`replace`** | `replace` | Regular expression replacement. |
| **`field`** | `replace` | Specify a single key to modify inside a matched object. If blank, matches all string values inside the object. |
| **`value`** | `add` \| `append` | The literal value to insert. |
| **`from`** / **`to`** | `rename` | The old and new key name to rename inside an object. |
| **`separator`** | `append` | Separator string used to join appended values. |

---

## 4. Path Placeholders

You can use dynamic placeholders inside rules that expand per request.

- **`{{oryca_gateway_url}}`** expands to the public URL of the service on the gateway (e.g. `https://gateway.oryca.io/gateway/api/resources/my-service`).
- **`{{oryca_auth}}`** expands to the client's credential query parameter (e.g. `api_key=xyz` or `token=abc`) so subsequent requests stay authenticated.

---

## 5. Execution Conditions (`conditions`)

Conditions allow you to enable a rule only if the matched elements satisfy a filter.

```json
{
  "path": "$.links[*]",
  "conditions": [
    { "field": "rel", "equals": ["self", "alternate"] }
  ],
  "params": {
    "field": "href",
    "find": "https://upstream-server.internal",
    "replace": "{{oryca_gateway_url}}"
  }
}
```
*In the example above, the rewrite rule only executes if the link item's `rel` attribute equals either `"self"` or `"alternate"`.*

Use `notEquals` for the opposite (skip the elements listed, act on the rest). Both take a list, and the spelling matters, because an unknown key is ignored, which reads as a rule that fires on everything.

> [!WARNING]
> Conditions are a gate, not a filter. Every condition on a rule has to pass on the element being examined; a rule that matches nothing does nothing, and never reports an error.

---

## 6. XML & Capabilities Docs

When working with XML (e.g. OGC WMTS Capabilities), use `type: "xml"` and specify elements via `xpath`. 

> [!TIP]
> To ignore XML namespace prefixes, query using `local-name()`.
> `//*[local-name()='ResourceURL']`

```json
{
  "type": "xml",
  "target": "body",
  "action": "replace",
  "xpath": "//*[local-name()='ResourceURL']",
  "params": {
    "field": "@template",
    "regex": "api_key=[^&]*",
    "replace": "{{oryca_auth}}"
  }
}
```

Use `@` in the `field` parameter to specify attribute targets.
- **Blank** modifies the text content inside the element.
- **`@href`** modifies the value of the `href` attribute.
- **`@template`** modifies the value of the `template` attribute.

---

## 7. What is Left Alone

Some responses are passed straight through, whatever your rules say. This is deliberate, and it is usually the answer when a transform appears to do nothing.

- **Non-2xx responses.** An error from the upstream reaches the client untouched.
- **Non-text bodies.** Binary payloads such as PNG or MVT tiles are never parsed.
- **Streamed responses.** Anything the gateway forwards without buffering is not rewritten, because rewriting means holding the whole body in memory.

---

## 8. Performance & Memory Cost

- **Buffered, briefly.** A rewritten body is held in memory while the rules run, so the rules should be cheap and the bodies should be documents, not downloads.
- **Selectors cost what they walk.** A deep walk such as `$..*` visits the whole document; `$.links[*]` visits one array. Name the path you mean.
