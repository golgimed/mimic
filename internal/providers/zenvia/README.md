# Zenvia

Simulates [Zenvia](https://zenvia.github.io/zenvia-openapi-spec/v2/) — SMS, WhatsApp, and Email messaging, with subscription-based delivery-status webhooks.

Routes are mounted under `/zenvia`.

## SMS

```bash
curl http://localhost:3000/zenvia/channels/sms/messages \
  -H "X-API-TOKEN: any-value" -H "Content-Type: application/json" \
  -d '{"from":"sender-id","to":"5511999999999","contents":[{"type":"text","text":"Hello"}]}'
```

## WhatsApp

```bash
curl http://localhost:3000/zenvia/channels/whatsapp/messages \
  -H "X-API-TOKEN: any-value" -H "Content-Type: application/json" \
  -d '{"from":"sender-id","to":"5511999999999","contents":[{"type":"text","text":"Hello"}]}'
```

Content types: `text`, `template`, `file`. See the [Zenvia API reference](https://zenvia.github.io/zenvia-openapi-spec/v2/) for full payload options — buttons/lists/products/flows/location/contacts content types aren't implemented here.

## Email

```bash
curl http://localhost:3000/zenvia/channels/email/messages \
  -H "X-API-TOKEN: any-value" -H "Content-Type: application/json" \
  -d '{"from":"sender@example.com","to":"recipient@example.com","contents":[{"type":"email","subject":"Hi","html":"<b>Hello</b>"}]}'
```

## Delivery-status webhooks (SMS/WhatsApp/Email)

Zenvia delivers status updates via a **subscription** you create once, not a per-message callback. Subscribe a webhook URL to a channel, then watch it receive `MESSAGE_STATUS` events as each message advances internally (`ACCEPTED → SENT → DELIVERED`):

```bash
curl http://localhost:3000/zenvia/subscriptions \
  -H "X-API-TOKEN: any-value" -H "Content-Type: application/json" \
  -d '{"eventType":"MESSAGE_STATUS","webhook":{"url":"https://your-app/webhooks/zenvia"},"criteria":{"channel":"sms"}}'
```

Delay between status transitions is controlled by `ZENVIA_STATUS_DELAY_MS` (default `2000`). Scenarios in [`scenarios.yaml`](scenarios.yaml) select other documented paths, for example `message-not-delivered` (`ACCEPTED → SENT → NOT_DELIVERED`, with `description`/`causes` on the event). See the root README's "Scenarios" section. Each hop is an explicit `internal/shared/lifecycle` transition run by the job scheduler (job kind `lifecycle:zenvia:message`), so pending hops are visible in the `jobs` table.

## Known limitations

- The internal status transition (`ACCEPTED → SENT → DELIVERED`) is the same for every channel; the real API's exact codes vary slightly by channel (e.g. WhatsApp also has `READ`).
- Webhook signing/verification (e.g. an HMAC header) is not implemented.

## Compatibility

Format: [docs/compatibility.md](../../../docs/compatibility.md).

**Contract**: High. The send-message endpoints for SMS, WhatsApp and Email and the subscription CRUD follow the Zenvia v2 OpenAPI (`docs/vendor/zenvia-openapi-v2.json`).
**Behavior**: Simplified. Every channel follows the same status path, and there is no real carrier.

- **Supported:** `POST /channels/{sms,whatsapp,email}/messages` and `/subscriptions` (create, list, get, delete).
- **Request validation:** `X-API-TOKEN` presence only, plus the required `from`, `to` and `contents` for each channel. A failure returns Mimic's `VALIDATION_ERROR` body.
- **Lifecycle & async:**
  - Messages start in Mimic's internal `ACCEPTED` state and move `SENT → DELIVERED`, one hop every `ZENVIA_STATUS_DELAY_MS`, via the job scheduler.
  - Scenarios select other documented paths: `REJECTED`, `NOT_DELIVERED`, `READ`.
- **Webhooks:**
  - `MESSAGE_STATUS` events go to matching subscriptions, filtered by channel and direction.
  - `description` and `causes` are included only when a scenario provides them.
  - Payloads are not signed.
- **Fault injection:** provider `zenvia`.
  - Route paths above for request faults.
  - Route `webhook` for `webhook_dropped` and `webhook_invalid`.
- **Known differences:**
  - Real status codes and timing vary by channel and carrier. Mimic uses one configurable delay.
  - Real `READ` depends on the channel. In Mimic it appears only in the `message-read` scenario.
  - Message and event ids are random UUIDs.
- **Unsupported:**
  - Content types other than `text`, `template`, `file` and `email`.
  - Inbound messages (`MESSAGE` events).
  - Webhook signing.
  - Other Zenvia products (e.g. voice, RCS).
