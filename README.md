# GolgiMed Provider Simulator

A lightweight simulator for third-party providers used by GolgiMed — for local development, integration testing, and CI, when you don't have (or don't want to depend on) real sandbox access.

It emulates provider HTTP contracts, state transitions, and webhooks with predictable, configurable behavior. It is **not** a production system, and it does not try to reproduce every detail of the real providers — see [CLAUDE.md](CLAUDE.md) for the project's design philosophy.

Currently simulated:

- **[Zenvia](https://zenvia.github.io/zenvia-openapi-spec/v2/)** — SMS, WhatsApp, and Email messaging, with subscription-based delivery-status webhooks.
- **[IntegraICP](https://developers.integraicp.com.br/api-reference/icp/v3/index.html)** — Brazilian digital signature / ICP-Brasil certificate flows.

## Requirements

- Node.js 22+
- Docker + Docker Compose (optional, for containerized runs)

## Getting started

```bash
npm install
npm run dev
```

The server listens on `http://localhost:3000` (`PORT` env var to change it). State is a local SQLite file at `db/simulator.sqlite`; delete it to reset.

Or with Docker:

```bash
docker compose up --build
```

## Usage

Every endpoint mirrors its real provider's relative path and payload shape as closely as reasonably possible, so a client built against the real API works against the simulator with only its base URL changed.

### SMS

```bash
curl http://localhost:3000/zenvia/channels/sms/messages \
  -H "X-API-TOKEN: any-value" -H "Content-Type: application/json" \
  -d '{"from":"sender-id","to":"5511999999999","contents":[{"type":"text","text":"Hello"}]}'
```

### WhatsApp

```bash
curl http://localhost:3000/zenvia/channels/whatsapp/messages \
  -H "X-API-TOKEN: any-value" -H "Content-Type: application/json" \
  -d '{"from":"sender-id","to":"5511999999999","contents":[{"type":"text","text":"Hello"}]}'
```

Content types: `text`, `template`, `file`. See the [Zenvia API reference](https://zenvia.github.io/zenvia-openapi-spec/v2/) for full payload options — buttons/lists/products/flows/location/contacts content types aren't implemented here.

### Email

```bash
curl http://localhost:3000/zenvia/channels/email/messages \
  -H "X-API-TOKEN: any-value" -H "Content-Type: application/json" \
  -d '{"from":"sender@example.com","to":"recipient@example.com","contents":[{"type":"email","subject":"Hi","html":"<b>Hello</b>"}]}'
```

### Delivery-status webhooks (SMS/WhatsApp/Email)

Zenvia delivers status updates via a **subscription** you create once, not a per-message callback. Subscribe a webhook URL to a channel, then watch it receive `MESSAGE_STATUS` events as each message advances internally (`ACCEPTED → SENT → DELIVERED`):

```bash
curl http://localhost:3000/zenvia/subscriptions \
  -H "X-API-TOKEN: any-value" -H "Content-Type: application/json" \
  -d '{"eventType":"MESSAGE_STATUS","webhook":{"url":"https://your-app/webhooks/zenvia"},"criteria":{"channel":"sms"}}'
```

### Digital signature (IntegraICP)

Three-step flow — see the [IntegraICP API reference](https://developers.integraicp.com.br/api-reference/icp/v3/index.html) for the full picture (PKCE, clearances, certificates):

```bash
# 1. Authenticate (autostart=true skips the real human-in-the-loop provider login
#    and immediately redirects to callback_uri with a credentialId)
curl -i "http://localhost:3000/integraicp/c/my-channel/icp/v3/authentications?secret_data=<code_challenge>&callback_uri=https://your-app/callback&autostart=true"

# 2. Fetch the credential (secret_data here is the code_verifier for the challenge above)
curl "http://localhost:3000/integraicp/c/my-channel/icp/v3/credentials/<credentialId>?secret_data=<code_verifier>"

# 3. Sign content (contentDigest is base64(sha256(content)))
curl http://localhost:3000/integraicp/c/my-channel/icp/v3/signatures \
  -H "Content-Type: application/json" \
  -d '{"credentialId":"<credentialId>","secretData":"<code_verifier>","requests":[{"contentDigest":"<base64-sha256>"}]}'
```

## Dashboard

`GET /dashboard` — a single page listing everything the simulator has processed (messages, credentials), newest first. Click a row for its raw payload and webhook delivery log.

## Fault injection

Simulate failures without touching code:

```bash
curl -X PUT http://localhost:3000/admin/faults -H "Content-Type: application/json" \
  -d '{"provider":"zenvia","routePattern":"/zenvia/channels/sms/messages","faultKind":"http_status","faultValue":"503"}'
```

Kinds: `delay_ms`, `http_status`, `timeout`, `invalid_payload`, `webhook_dropped`, `webhook_invalid`. Omit `routePattern` to apply to every route for a provider; add `"times": N` to auto-clear after N uses. `GET /admin/faults` lists active faults, `DELETE /admin/faults/:id` clears one.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `3000` | HTTP port |
| `DB_PATH` | `db/simulator.sqlite` | SQLite file path |
| `LOG_LEVEL` | `info` | Fastify logger level |
| `DEFAULT_DELAY_MS` | `0` | Baseline simulated processing latency |
| `ZENVIA_STATUS_DELAY_MS` | `2000` | Delay per Zenvia status transition |
| `SCHEDULER_INTERVAL_MS` | `1000` | Background job poll interval |

## Testing

```bash
npm run typecheck
npm test
```

Integration tests spin up the Fastify app in-process against an in-memory SQLite database (Vitest, `tests/`).

## Known limitations

Contracts are taken from each provider's public documentation (cached under [`docs/vendor/`](docs/vendor/)), not verified sandbox traffic, since sandbox access wasn't available while building this:

- IntegraICP's real auth requires a human choosing a provider and logging in; the simulator auto-authenticates instead. The non-autostart "list clearances" response always returns one fake entry.
- Zenvia's internal status transition (`ACCEPTED → SENT → DELIVERED`) is the same for every channel; the real API's exact codes vary slightly by channel (e.g. WhatsApp also has `READ`).
- Webhook signing/verification (e.g. an HMAC header), rate-limit (429) behavior, and exact error-body shapes for edge cases are best-effort.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
