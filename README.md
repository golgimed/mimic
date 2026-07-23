# Provider Simulator

Emulates third-party providers (IntegraICP, Zenvia) for GolgiMed local development, integration tests, and CI. See [CLAUDE.md](CLAUDE.md) for the project's design philosophy and constraints.

## Running locally

```bash
npm install
npm run dev          # tsx watch, http://localhost:3000
```

Or via Docker:

```bash
docker compose up --build
```

SQLite state persists in `db/simulator.sqlite` (local) or the `simulator-data` volume (Docker). Delete it (or `docker compose down -v`) to reset.

## Testing

```bash
npm run typecheck
npm test              # vitest, in-process Fastify + :memory: SQLite
```

## Dashboard

`GET /dashboard` — a single static page listing everything the simulator has "sent" (Zenvia messages, IntegraICP credentials), newest first. Click a row for its raw payload and webhook delivery log. Polls `GET /admin/items` / `GET /admin/items/:provider/:id`.

## Providers

### Zenvia (`/zenvia/...`)

Routes mirror the real relative paths. Contract cached at [`docs/vendor/zenvia-openapi-v2.json`](docs/vendor/zenvia-openapi-v2.json).

- `POST /zenvia/channels/sms/messages` — content: `text` | `template`.
- `POST /zenvia/channels/whatsapp/messages` — content: `text` | `template` | `file`. Also accepts `idRef`/`contentRef` (reply-to). The real API additionally supports buttons/lists/products/flows/location/contacts content types — out of scope for now, add if GolgiMed needs them.
- `POST /zenvia/channels/email/messages` — content: `email` (`subject`, `html`, `attachments`) | `template`. Also accepts `representative` (`{type, name}`).
- All three require `X-API-TOKEN` and return the message as sent; no status field (matches the real API).
- `POST /zenvia/subscriptions`, `GET /zenvia/subscriptions`, `GET|DELETE /zenvia/subscriptions/:id` — subscribe a webhook URL to `MESSAGE_STATUS` events for a channel.
- Internally, message status advances `ACCEPTED → SENT → DELIVERED` on a timer (`ZENVIA_STATUS_DELAY_MS`, default 2000ms), the same two-hop transition for every channel — a simplification; the real API's exact status codes differ slightly per channel (e.g. WhatsApp also has `READ`). Each hop posts a `MESSAGE_STATUS` event to matching subscriptions.

### IntegraICP (`/integraicp/c/:channelId/icp/v3/...`)

Digital signature flow. Contract cached at [`docs/vendor/integraicp-api-reference-v3.md`](docs/vendor/integraicp-api-reference-v3.md) (a Docsify page — IntegraICP has no OpenAPI spec).

- `GET /authentications` — with `autostart=true`, the simulator **auto-authenticates synchronously** and 302-redirects to `callback_uri` with a fake `credentialId` (the real flow needs a human picking a provider and logging in, which can't be reproduced). Without `autostart`, returns a fake `ClearancesResult` list.
- `GET /credentials/:credentialId` — poll for the (simulated) authenticated credential + fake certificate info.
- `POST /signatures` — synchronous, matching the real API: signs and returns `COMPLETED_WITH_SUCCESS` in the same response.
- PKCE (RFC 7636) is validated end to end: `secret_data` at `/authentications` is a `code_challenge`, `secret_data` at `/credentials` and `/signatures` must be the matching `code_verifier`.

## Fault injection

`PUT /admin/faults` `{ provider, routePattern?, faultKind, faultValue?, times? }` — `routePattern` omitted applies to every route for that provider; `times` omitted means the fault stays active until deleted.

Kinds: `delay_ms`, `http_status`, `timeout`, `invalid_payload` (checked before the real handler runs), `webhook_dropped`, `webhook_invalid` (checked at Zenvia webhook delivery time — use `routePattern: "webhook"`).

`GET /admin/faults` lists active faults, `DELETE /admin/faults/:id` clears one.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `3000` | HTTP port |
| `DB_PATH` | `db/simulator.sqlite` | SQLite file path |
| `MIGRATIONS_DIR` | `db/migrations` | Migration files |
| `LOG_LEVEL` | `info` | Fastify logger level |
| `DEFAULT_DELAY_MS` | `0` | Baseline simulated processing latency |
| `ZENVIA_STATUS_DELAY_MS` | `2000` | Delay per Zenvia status hop |
| `SCHEDULER_INTERVAL_MS` | `1000` | Job poll interval |
| `DASHBOARD_PATH` | `dashboard/index.html` | Dashboard HTML file |

## Assumptions and known gaps

Neither provider's real sandbox was available while building this — contracts were taken from public docs (cached under `docs/vendor/`) rather than verified traffic. Notable gaps, to revisit once sandbox access exists:

- IntegraICP's real auth requires a human choosing a Clearance and logging into an actual trust provider; the simulator skips straight to issuing a credential. The non-autostart "list clearances" path always returns one fake entry.
- Zenvia webhook signing/verification (e.g. an HMAC header) isn't confirmed from the spec.
- Rate-limit (429) behavior and exact error-body shapes for edge cases are best-effort.
- GolgiMed doesn't currently integrate with Zenvia for any channel: WhatsApp is sent via Meta's Cloud API directly, and SMS/Email delivery are unimplemented stubs (Zenvia is only listed as one *candidate* SMS provider in PP-011). The Zenvia WhatsApp/Email channels here were built ahead of real usage, straight from the public spec.

### Wiring GolgiMed to this simulator (not yet done — GolgiMed-side work)

None of the four channels can be pointed at this simulator via env config alone today; each needs a code change in the `openmed` repo:

- **SMS** (`services/openmed/internal/delivery/adapters/sms/sms_deliverer.go`) — `HTTPSMSClient.Send()` is a pure stub, no HTTP client. `BaseURL`/`APIKey` fields already exist (from `SMS_BASE_URL`/`SMS_API_KEY`) but are never read. Needs the actual `POST {BaseURL}/channels/sms/messages` call added, with `X-API-TOKEN: {APIKey}`.
- **Email** (`.../adapters/email/email_deliverer.go`) — closest to done: has an `http.Client` and `BaseURL`/`APIKey` wired via `EMAIL_BASE_URL`/`EMAIL_API_KEY`, but `Send()` still returns a fake ID without calling out. Same fix shape as SMS, targeting `/channels/email/messages`.
- **WhatsApp** (`.../adapters/whatsapp/whatsapp_deliverer.go`) — hardcoded to `graph.facebook.com` with **Meta's payload shape** (`messaging_product`, `type: "template"`), not Zenvia's. Pointing it at this simulator's `/zenvia/channels/whatsapp/messages` wouldn't work even with a base-URL override — the request bodies don't match. Simulating WhatsApp against GolgiMed's real code would need either a Meta-shaped fake (not built — declined for now) or rewriting the deliverer to speak Zenvia (a bigger, deliberate change, since Meta was an ADR-050 decision).
- **Signature** — GolgiMed's `internal/signature` module has a `SignatureProvider` interface and adapters for BirdID/SafeID/GovBR/ClickSign/ICP-Brasil, each with a `BaseURL` env var — but **no IntegraICP adapter exists**. Needs a new `internal/signature/adapters/integraicp/` package implementing `SignatureProvider`, following the BirdID/SafeID adapter as a template, registered in `signature/module.go`.

This repo's endpoints are ready to be called once that wiring exists; the work above is tracked as a to-do, not started.
