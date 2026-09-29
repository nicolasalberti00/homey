# MCP

homey speaks the Model Context Protocol over **Streamable HTTP** at `/mcp`, so
an MCP host drives the inventory through tools instead of raw HTTP.

- Endpoint: `POST /mcp` (JSON-RPC); `GET /mcp` opens the server-sent event
  stream of a session.
- On by default; switch it off with `HOMEY_MCP_ENABLED=false` or `--mcp=false`.
- The same bearer tokens as the REST API: every request carries
  `Authorization: Bearer …`. Without one the answer is `401` with a
  `WWW-Authenticate` challenge.

## Tools

Reading — any valid token:

| Tool | What it answers |
| --- | --- |
| `search_inventory` | where something is, by name, alias, tag or description: every candidate carries its location path, quantity and tags |
| `get_item` | everything stored about one item, by id |
| `list_location` | what sits directly in a room or a container; without one, the rooms of the home |
| `count_items` | how many items match, optionally filtered by tag or place (a place counts everything inside it, containers included) |

Writing — a token with the `write` scope:

| Tool | What it does |
| --- | --- |
| `add_item` | records a new item in a room or container that already exists, and answers with its id |
| `update_item` | changes the fields it is given — name, description, quantity, tags, aliases, notes — and leaves the others alone |
| `move_item` | puts an item in another room or container, leaving everything else about it alone |
| `delete_item` | removes an item for good, after a confirmation step (below) |

The registry enforces the permission **before** the handler runs: a read-only
token (`--scope read`) sees the whole tool list but is refused on every write
tool, and a request that never said who is calling is refused too. Inputs and
outputs are validated against JSON schemas inferred from the types the tools
are written with, so what a client sees and what the code accepts cannot drift.

## Places are named, not numbered

A location is a string a person would say: `"Garage"` or
`"Garage > Toolbox > Drawer"`, matched ignoring case and accents, with a bare
name accepted when only one place carries it. When a name fits several places,
the tool does not pick one — it answers with a clarification instead:

```json
{
  "clarification": {
    "argument": "destination",
    "name": "Toolbox",
    "question": "More than one place is called \"Toolbox\". Which one did you mean?",
    "candidates": ["Garage > Toolbox", "Cucina > Toolbox"]
  }
}
```

The call changed nothing; repeat it with one of the full paths. The same holds
for items: `search_inventory` returns every candidate with its location path,
so a model asks which one was meant rather than guessing.

## Deleting asks first

`delete_item` never deletes silently:

1. A call with just an `id` answers `confirmation: "pending"`, the item that
   would go, and a short-lived `confirmation_token` — **nothing is removed**.
2. Calling it again with that token in `confirmation_token` carries the
   deletion out and answers `confirmation: "confirmed"`.

The token lasts two minutes and is bound to that one action and that one item:
it cannot be replayed or spent elsewhere. Two bypasses exist, both explicit —
`confirm: true` in the same turn, and a token created with
`--destructive-confirmation=bypass` — and both are recorded in the `events`
table as `confirmation: "bypassed"` together with the tool and the token's
name.

## Connecting a client

Create a token first (see [API tokens](../README.md#api-tokens)); a read-only
one is enough to explore:

```bash
go run ./cmd/server token create --name "cursor" --scope read
```

### Cursor

Cursor talks to a remote server directly and can carry the header. Add it to
`~/.cursor/mcp.json` (global, so the token does not land in version control)
or to `.cursor/mcp.json` in the project:

```json
{
  "mcpServers": {
    "homey": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer YOUR_TOKEN" }
    }
  }
}
```

Restart Cursor (or reload the MCP servers) and the eight tools appear.

### Claude Code

```bash
claude mcp add --transport http \
  --header "Authorization: Bearer YOUR_TOKEN" \
  homey http://127.0.0.1:8080/mcp
```

### Claude Desktop

Claude Desktop's configuration file speaks stdio, so it goes through
[`mcp-remote`](https://github.com/geelen/mcp-remote), a local bridge that
forwards the header (Node required). Open **Settings → Developer → Edit
Config** and add:

```json
{
  "mcpServers": {
    "homey": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "http://127.0.0.1:8080/mcp",
        "--header",
        "Authorization: Bearer YOUR_TOKEN"
      ]
    }
  }
}
```

Restart Claude Desktop completely after saving.

### Claude (claude.ai connectors)

A custom connector is opened from **Anthropic's cloud**, not from your
machine: the URL must be a public HTTPS address (see
[`deployment/docker`](../deployment/docker/README.md) for a reverse proxy with
certificates), and the connector must be able to send the token. Static
request headers are supported for organization connectors in beta; a personal
connector only offers OAuth, which homey does not implement. For a home
server, prefer Claude Desktop with the `mcp-remote` bridge above.

### ChatGPT and Codex

Two different surfaces, two different answers.

**Codex — CLI, IDE extension and the ChatGPT desktop app** — share one
configuration file and a Streamable HTTP server there carries the token
(`~/.codex/config.toml`, or `.codex/config.toml` for a trusted project):

```toml
[mcp_servers.homey]
url = "http://127.0.0.1:8080/mcp"
bearer_token_env_var = "HOMEY_TOKEN"
```

`HOMEY_TOKEN` is read from the environment Codex starts in; the literal form
works too — `http_headers = { "Authorization" = "Bearer YOUR_TOKEN" }`. All
three clients read the same file, so one entry serves them.

**ChatGPT on the web** takes custom connectors (Developer Mode, web only) as
**OAuth or no authentication, and nothing else**: its own documentation says
the client "cannot present custom API keys", and the tool security schemes it
accepts are `noauth` and `oauth2`. A token-protected homey has neither, so
`no authentication` would leave the header out and homey answers `401`.
There is no recipe that works there today — use one of the clients above.

### Ollama

Ollama serves models; it is not an MCP client and has no place to configure a
server. Put a host in front of it that speaks MCP *and* can attach the
header. [Kit](https://github.com/mark3labs/kit) runs Ollama models and takes
stdio servers, so the token rides the same local bridge as above
(`~/.kit.yml`):

```yaml
model: ollama/qwen3:14b
mcpServers:
  homey:
    type: local
    command: ["npx", "-y", "mcp-remote", "http://127.0.0.1:8080/mcp", "--header", "Authorization: Bearer YOUR_TOKEN"]
```

The model needs tool calling (a 14B+ model such as Qwen 3 or Llama 3.3 is the
practical floor). Kit's own `type: remote` cannot attach a static header yet,
which is why the example goes through the bridge.

## Checking it works

```bash
curl -sS -X POST http://127.0.0.1:8080/mcp \
  -H "Authorization: Bearer $HOMEY_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
        "protocolVersion":"2025-06-18","capabilities":{},
        "clientInfo":{"name":"curl","version":"0"}}}'
```

Then `tools/list` returns the eight tools and `tools/call` runs one.

## When it does not work

| Symptom | Cause |
| --- | --- |
| `401` with `WWW-Authenticate` | missing, wrong or revoked token |
| `403` / permission error on a write tool | read-only token, or a request with no caller |
| `404` on `/mcp` | MCP is switched off (`HOMEY_MCP_ENABLED=false`) |
| `429` with `Retry-After` | rate limited: `POST /mcp` counts against `HOMEY_RATE_LIMIT_WRITES`, repeated `401`s against `HOMEY_RATE_LIMIT_AUTH_FAILURES` |
| The client connects but the tools do nothing | the model's tool calling is switched off or too weak |
| A write tool answers with `clarification` | the name fits several places; answer with the full path |
