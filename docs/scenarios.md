# Scenario library

Mimic's built-in scenarios capture behavior its providers already
simulate, so integration tests can select realistic paths by name instead
of wiring states and faults by hand. The live catalog, with lifecycle,
transitions, webhooks, faults and requirements, is at
`GET /admin/scenarios`. Each scenario is defined in its provider's
`internal/providers/<provider>/scenarios.yaml`.

## Using a scenario

```bash
# 1. Create a simulation running the scenario (seed and profile optional)
curl -X POST http://localhost:3000/admin/simulations \
  -d '{"scenario": "message-not-delivered", "profile": "fast", "seed": 847291}'
# → {"id": "sim_…", "seed": 847291, "scenario": "message-not-delivered", "profile": "fast", …}

# 2. Send requests in it
curl -X POST http://localhost:3000/zenvia/channels/sms/messages \
  -H "X-API-TOKEN: t" -H "X-Mimic-Simulation: sim_…" -H "Content-Type: application/json" \
  -d '{"from":"acct","to":"5511999999999","contents":[{"type":"text","text":"hi"}]}'

# 3. Inspect / reproduce
curl "http://localhost:3000/admin/timeline?simulation_id=sim_…"
curl -X POST http://localhost:3000/admin/simulations/sim_…/replay
```

The pieces compose. The **scenario** decides what happens. The
**profile** decides how fast and how reliably it happens (`fast`,
`realistic`, `chaos`, see the root README). The **seed** makes every
random decision reproducible. The **clock** (`MIMIC_CLOCK=virtual` plus
`POST /admin/clock/advance`) fast-forwards delays. A scenario's faults
apply only to requests in its simulation.

## Catalog

### zenvia

Statuses are Zenvia's documented `MESSAGE_STATUS` codes. A transition with
no explicit delay uses `ZENVIA_STATUS_DELAY_MS`.

| Scenario | Lifecycle | Webhooks | Notes |
|---|---|---|---|
| `message-delivered` | ACCEPTED → SENT → DELIVERED | MESSAGE_STATUS ×2 | Mimic's default path, made explicit |
| `message-not-delivered` | ACCEPTED → SENT → NOT_DELIVERED | MESSAGE_STATUS ×2 | NOT_DELIVERED event carries `description` and `causes` |
| `message-rejected` | ACCEPTED → REJECTED | MESSAGE_STATUS ×1 | REJECTED event carries `description` and `causes` |
| `message-read` | ACCEPTED → SENT → DELIVERED → READ | MESSAGE_STATUS ×3 | READ depends on the channel in the real API |
| `webhook-delivery-failed` | default | none delivered | every MESSAGE_STATUS webhook is dropped and logged as `dropped` |
| `request-rate-limited` | default | default | SMS sends limited to 2/min → `429` + `Retry-After` |
| `zenvia-unavailable` | n/a | n/a | every zenvia request → `503` |

### bry-scad

A collection created in the simulation follows the path on its own.
No `/concluir`, `/cancelar` or `/rejeitar` call is needed.

| Scenario | Lifecycle | Webhooks | Requires |
|---|---|---|---|
| `collection-completed` | PENDENTE → CONCLUIDO (30s) | completion webhook | `BRY_SCAD_WEBHOOK_URL` |
| `collection-rejected` | PENDENTE → REJEITADA (30s) | none | |
| `collection-cancelled` | PENDENTE → CANCELADO (30s) | none | |

### integraicp

IntegraICP is simulated statelessly, so its scenarios only inject faults.

| Scenario | Effect |
|---|---|
| `integraicp-unavailable` | every integraicp request → `503` |

## Adding a scenario

Add an entry to the provider's `scenarios.yaml`:

```yaml
- name: unique-name            # global across providers
  resource: message            # the resource type whose lifecycle it drives
  description: What it simulates, in one sentence.
  initial: ACCEPTED
  transitions:
    - to: SENT                 # must be an allowed transition of the provider's lifecycle
      after: 5s                # optional; default = provider's delay
      meta: {reason: …}        # optional provider-specific details
  webhooks: [MESSAGE_STATUS]   # descriptive
  faults:                      # optional, same terms as PUT /admin/faults
    - {kind: http_status, value: "503", route: /zenvia/subscriptions}
  requires: [SOME_ENV_VAR]     # optional, descriptive
```

The provider must actually implement the scenario's behavior; don't
invent states or contracts it doesn't have. Steps are validated at
startup against the provider's lifecycle machine. Every scenario needs a
test in `tests/scenario/library_test.go`; `TestEveryScenarioIsTested`
fails otherwise.
