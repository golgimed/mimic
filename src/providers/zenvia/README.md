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

Delay between status transitions is controlled by `ZENVIA_STATUS_DELAY_MS` (default `2000`).

## Known limitations

- The internal status transition (`ACCEPTED → SENT → DELIVERED`) is the same for every channel; the real API's exact codes vary slightly by channel (e.g. WhatsApp also has `READ`).
- Webhook signing/verification (e.g. an HMAC header) is not implemented.
