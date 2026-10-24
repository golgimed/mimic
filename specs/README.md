# specs/

Drop OpenAPI 3.x spec files (`.yaml`, `.yml`, `.json`) here to have Mimic
serve them as mock providers.

`example.yaml` is a small CRUD-shaped spec mounted by default — copy its
structure as a starting point for your own specs.

- Each spec is mounted under a route prefix derived from `info.title`
  (slugified). Set `x-mimic-name` at the document root to override the
  derived prefix if it collides with another spec or a hand-written
  provider.
- `serve` uses this directory by default when neither `-spec-dir` nor
  `-spec` is passed and the directory exists. Override with
  `-spec-dir <path>` or repeated `-spec <glob>` flags.
- Loaded once at boot; each spec's version (`info.version`) and content
  checksum are visible on the dashboard under the `openapi` provider.
- Specs are parsed with [kin-openapi](https://github.com/getkin/kin-openapi).
  Local `$ref`s into `#/components/...` (schemas, parameters, request
  bodies, responses) are resolved, including nested and recursive schema
  references. External `$ref`s (another file or a URL) are rejected: a
  spec must be self-contained.
- Schema-generated bodies merge `allOf` members and use the first
  `oneOf`/`anyOf` alternative; a recursive schema stops (as `null` or an
  empty array) at its first repetition.
- A spec that can't be parsed, or has an unresolvable `$ref`, fails the
  boot with an error naming the file. OpenAPI conformance problems that
  don't prevent serving (e.g. a response without `description`) are only
  logged as warnings.

## Request validation

Every spec-mocked request is checked against its operation's contract:
path/query/header parameters (presence, basic type, enum) and the request
body (presence, content type, JSON structure, required properties, types,
enums — `$ref`s resolved). Security requirements are not checked, since
spec routes don't enforce auth.

- `MIMIC_OPENAPI_VALIDATION=permissive` (default) — invalid requests are
  still served; the response carries an `X-Mimic-Validation-Warning`
  header summarizing the violations.
- `MIMIC_OPENAPI_VALIDATION=strict` — invalid requests get `400` before any
  simulated behavior (persistence, generated bodies) runs:

  ```json
  {"error": {"code": "VALIDATION_ERROR", "issues": [
    {"in": "body", "name": "signer.email", "reason": "property \"email\" is missing"},
    {"in": "query", "name": "mode", "reason": "value is not one of the allowed values [\"sync\",\"async\"]"}
  ]}}
  ```

  This envelope is Mimic's own, not the real provider's error format.
- Set `x-mimic-validation: strict` (or `permissive`) at a spec's document
  root to override the global mode for that spec.
- Fault injection (`x-mimic-behavior`, `/admin/faults`) is applied before
  validation, as a gateway-level failure would be.
- Body content types without a JSON-compatible decoder (e.g. XML) are
  reported as violations; prefer `permissive` for such specs.

## Persistence

By default, spec-mocked routes are stateless: every response is either the
spec's literal example or a schema-derived stub, computed once at boot so
repeated requests to the same route always get the same body.

Set `MIMIC_OPENAPI_PERSIST=true` (env var, default `false`) to turn on real
CRUD persistence for CRUD-shaped routes across every loaded spec, or set
`x-mimic-persist: true`/`false` at a spec's document root to override the
global default for just that spec.

A route is CRUD-shaped when its path follows plain REST convention:

| Path shape | Method | Operation |
|---|---|---|
| collection, e.g. `/pets` | GET | list |
| collection, e.g. `/pets` | POST | create |
| item, e.g. `/pets/{id}` | GET | read |
| item, e.g. `/pets/{id}` | PUT/PATCH | update |
| item, e.g. `/pets/{id}` | DELETE | delete |

The resource type is the path's first segment. **Only single-level
resources are supported** — a collection with zero path parameters, or an
item path with exactly one, trailing path parameter. Nested resources
(e.g. `/pets/{id}/vaccinations`) or multiple path parameters aren't
CRUD-inferred and keep the static example/stub behavior regardless of the
persist setting.

When persistence is on for a route:

- `POST` persists the request body as a new resource. Whatever the client
  sends is stored verbatim; any property declared in the response schema
  but missing from the request (e.g. a server-assigned `status` or
  `createdAt`) is filled with a realistic fake value. The resource's `id` is
  whatever the client supplied under `"id"`, or a freshly generated one
  (this is always the JSON field name, regardless of what the spec calls
  the path parameter). Generated ids and fake values draw from the
  request's simulation seed, so the same requests under the same seed
  create the same resources (see the root README's "Simulations and
  seeds").
- `GET` (list) always returns a raw JSON array of stored resources —
  it does not attempt to reproduce a declared response envelope (e.g.
  `{"pets": [...]}`).
- `GET`/`PUT`/`PATCH`/`DELETE` (item) 404 if the resource doesn't exist —
  nothing is auto-seeded.

Persisted resources live in SQLite alongside every other provider's state,
so they're covered by the existing flush/reset behavior.

This directory is version-controlled — commit specs here so environments
serve the same contract.

## Compatibility

Format: [docs/compatibility.md](../docs/compatibility.md).

**Contract**: As faithful as the spec. Paths, methods and response examples or schemas come from the spec, and local `$ref`s are resolved. Request validation is optional (`MIMIC_OPENAPI_VALIDATION`).
**Behavior**: Generic. Spec routes return either static example or schema bodies or single-level CRUD persistence. There is no provider-specific logic.

- **Lifecycle & async, webhooks:** none. Specs don't describe behavior.
- **Fault injection:** provider `openapi`, the route path as mounted (with prefix), or `x-mimic-behavior`.
- **Unsupported:**
  - Security schemes (auth isn't enforced).
  - Callbacks and webhooks declared in specs.
  - Nested CRUD resources.
  - External `$ref`s.
