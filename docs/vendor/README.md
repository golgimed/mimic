# Vendor Specs

Cached copies of official provider API specs, used as the source of truth for request/response contracts (per `CLAUDE.md`: never invent contracts).

- `zenvia-openapi-v2.json` — built OpenAPI 3 spec from `zenvia/zenvia-openapi-spec`, `gh-pages` branch, `v2/openapi.json`. Fetched 2026-07-23.

Re-fetch when a provider contract changes or before implementing a new endpoint, to confirm the cached copy is still current.
