# Provider compatibility

How closely does Mimic's simulation match each real provider? Every
provider README has a **Compatibility** section in the standard format
below. This page defines that format and summarizes it.

Compatibility has two axes:

- **Contract compatibility**: does the API *look* like the real
  provider? This covers routes, request and response shapes, status codes
  and auth headers.
- **Behavioral compatibility**: does it *behave* like the real provider?
  This covers lifecycles, timing, async processing, webhooks, validation
  rules and error conditions.

A provider can score high on contract and still simplify behavior on
purpose. The notes say where it does, and why, so tests don't assume
fidelity that isn't there.

## Summary

| Provider | Contract | Behavior | Async / webhooks | Scenarios |
|---|---|---|---|---|
| [Zenvia](../internal/providers/zenvia/README.md#compatibility) | High: send-message and subscription endpoints for SMS/WhatsApp/Email | Simplified: one status path for every channel; no real carrier | Status hops via scheduler; `MESSAGE_STATUS` webhooks via subscriptions | 7 |
| [IntegraICP](../internal/providers/integraicp/README.md#compatibility) | High: auth / credential / signature v3 | Simplified: auto-authentication; signatures computed, not CA-backed | None (synchronous, per official API) | 1 (fault-only) |
| [SNCR](../internal/providers/sncr/README.md#compatibility) | High: login, token exchange, both numeração endpoints | High: daily/monthly quotas, CNPJ check, identity matching | None (synchronous) | — |
| [BRy SCAD](../internal/providers/bryscad/README.md#compatibility) | High: all 54 operations of OpenAPI v2.12.1 registered | Partial: collection lifecycle simulated; most operations return deterministic stubs | Completion webhook via `/concluir` or a scenario | 3 |
| [BRy Medical](../internal/providers/brymedical/README.md#compatibility) | Partial: the two calls golgimed's adapter makes | Partial: token uses and expiry enforced; signing simulated | None | — |
| [CRM (CFM)](../internal/providers/crm/README.md#compatibility) | High: SOAP 1.1 `Consultar`/`Validar`, official WSDL served | Fixture-based lookups | None | — |
| [OpenAPI adapter](../specs/README.md#compatibility) | As good as the dropped-in spec (examples/schemas, `$ref`, validation) | Generic: static responses or single-level CRUD | None | — |

All providers support Mimic's cross-cutting features: fault injection
(`PUT /admin/faults`), simulations with seeds, profiles, the simulation
clock, the timeline, correlation IDs and replay. These are Mimic
features, not provider behavior, and are documented in the root README.

## Format

Each provider README's `## Compatibility` section uses these headings.
Omit the ones that don't apply, and keep each to a few bullets.

```markdown
## Compatibility

**Contract** — <High | Partial | Low>: one line.
**Behavior** — <High | Simplified | Partial>: one line.

- **Supported:** resources/operations covered (or "see Endpoint coverage").
- **Request validation:** what is checked, what is accepted unchecked.
- **Responses & errors:** documented shapes reproduced; invented ones flagged.
- **Lifecycle & async:** states, transitions, timing (scenarios that select other paths).
- **Webhooks:** events, delivery, signing.
- **Fault injection:** routes/keys to use.
- **Known differences:** real provider vs Mimic, and why.
- **Unsupported:** not simulated at all.
```

Keep notes next to the provider and update them in the same PR as any
behavior change. They are documentation only; nothing in code models
compatibility.
