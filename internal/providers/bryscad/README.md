# BRy SCAD

Simulates the [BRy SCAD API REST v1](https://cloud.bry.com.br/scad-api/) for
creating and managing signature collections. Routes are mounted under
`/bry-scad` to keep Mimic providers isolated.

## Endpoint coverage

All **54 operations** in BRy SCAD OpenAPI v2.12.1 are registered: webhooks
(v1/v2), collection creation and management, groups, tags, signature images
and locations, signing sessions, collection documents and downloads, and the
pending-participants report. Every route is mounted below `/bry-scad`.

Every route requires an `Authorization: Bearer <token>` header. Mimic checks
that it is present but does not validate it against BRy Cloud.

Collections begin as `PENDENTE`; cancel and reject move them to `CANCELADO`
and `REJEITADA`, and the Mimic-only `POST /coletas/{chave}/concluir` helper
to `CONCLUIDO`. These are the only allowed transitions (an explicit
`internal/shared/lifecycle` machine): acting on a collection that already
left `PENDENTE` returns `409` with `chaveI18n: "coleta_situacao_invalida"`.
That status/key is a Mimic assumption — BRy's documentation doesn't specify
the response for a finished collection.

In a simulation running one of the [`scenarios.yaml`](scenarios.yaml)
scenarios (`collection-completed`, `collection-rejected`), a new collection
follows that path on its own after the scenario's delay. Completion fires the
same webhook as `/concluir`. Creation payloads are retained internally and collection
responses use the documented `listarColeta` and `sucesso` response envelopes.
The remaining OpenAPI operations return their documented top-level object,
array, binary, or success-envelope response type with deterministic empty or
successful state where the provider’s server-side rules are not needed for
local integration testing.

## Fault injection

Use `PUT /admin/faults` with provider `bry-scad` and any implemented route path.
