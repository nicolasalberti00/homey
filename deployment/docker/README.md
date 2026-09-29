# Deployment examples

Two compose files: the base one at the repository root, plain HTTP, and the
one in this directory, HTTPS through a reverse proxy.

## Base example (HTTP)

From the repository root:

```bash
docker compose up -d
curl -s http://localhost:8080/healthz
```

- Serves the UI, the REST API and `/mcp` on `http://<host>:8080`.
- Data lives in `./data` (SQLite). On Linux hosts make the directory writable
  by uid 1000, the user inside the container:
  `mkdir -p data && sudo chown 1000:1000 data`.
- The first token comes from the container itself:

  ```bash
  docker compose exec homey homey token create --name web --scope read,write
  ```

Use it as it is on a LAN or behind a proxy of your own.

## HTTPS example (Caddy)

homey speaks plain HTTP and expects TLS to end in front of it. Caddy
terminates TLS and obtains the certificates itself.

1. Edit `Caddyfile` and put your domain where `homey.example.com` is. The
   domain's A/AAAA record must point at this host, and ports 80 and 443 must
   be reachable: 80 is used to prove the domain, then both are used for
   renewals (Caddy renews on its own, there is nothing to schedule).

2. Start it:

   ```bash
   cd deployment/docker
   docker compose -f compose.https.yaml up -d
   ```

3. Create a token and open the site:

   ```bash
   docker compose -f compose.https.yaml exec homey homey token create --name web --scope read,write
   ```

   Then open `https://<domain>` and paste the token in **Settings**.

For a first run without a domain, replace the site with `localhost` in the
`Caddyfile`: Caddy issues an internal certificate and the browser warns about
it.

### Notes

- **homey is not published on the host** in this example: only the proxy
  reaches port 8080. Never publish the homey port and the proxy's at the same
  time — that would put an unauthenticated door next to the authenticated one.
- **Rate limits see one address, not one client.** Behind a proxy the peer
  address is the proxy, so `HOMEY_RATE_LIMIT_WRITES` and
  `HOMEY_RATE_LIMIT_AUTH_FAILURES` count every visitor together: a burst of
  mutations, or a few failed token attempts, affects everybody for that
  minute. The defaults are fine for a household; raise them or set them to
  `0` (as this example does) when the proxy does its own limiting.
- **Backup and restore** work the same from inside the container — see the
  [Backup](../../README.md#backup) section of the README.

### Other hosts

The image is a single static binary on Alpine; build it for the target
architecture with buildx:

```bash
docker buildx build --platform linux/arm64 -t homey:arm64 .
```

On a NAS or a Raspberry Pi, point the volume at storage that survives
container recreation — the database is the only state homey has.
