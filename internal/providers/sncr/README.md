# SNCR

Simulates the **Sistema Nacional de Controle de Receituários (SNCR)** API — ANVISA's official
prescription-*numbering* service for controlled substances (Notification A/B/B2/RR/RT, Receita de Controle
Especial, Receita Sujeita a Retenção).

Routes are mounted under `/sncr`.

**SNCR is a numbering allocation service, not a document-submission service.** GolgiMed requests a block of
official prescription numbers up front and assigns them to prescriptions locally — there is no "submit a
prescription, get it accepted/rejected" operation anywhere in this API.

This is a real, ANVISA-published API contract (Manual da API SNCR, 1ª edição, and Instruções de apoio ao
processo de integração v1.0) — the request/response shapes and business rules below mirror it directly, they
are not invented.

## Auth flow simplification

The real SNCR API authenticates each prescriber through their own Gov.br account via Keycloak — a full
browser redirect chain (`login → Keycloak → Gov.br → Keycloak callback → session_token → access_token`) that
requires a human logging in. The simulator can't drive a real Gov.br login, so — the same way IntegraICP's
`autostart=true` skips its own human-in-the-loop broker login — `GET /sncr/api/v1/auth/login` completes that
whole chain immediately and redirects straight to `client_url?session_id=...`, as if a real professional had
just finished authenticating.

**Simulator-only convenience**: since there's no real login to say *which* prescriber authenticated, the
login call accepts `conselho`, `uf`, and `documento` as extra query params (defaulting to `CRM`/`SP`/`000000`
if omitted) to pick the simulated identity — the same idea as IntegraICP's `subjectKey`/`subjectType` params.
Real SNCR clients never send these; they come from whichever Gov.br account the professional logged in with.

Everything downstream of that — the one-time `session_token` (30s TTL, single-use), the `access_token`
exchange, and Bearer-authenticated numeracoes calls — follows the real API's shape exactly.

## Example flow

```bash
# 1. "Login" (skips the real Gov.br redirect chain; conselho/uf/documento are simulator-only)
curl -i "http://localhost:3000/sncr/api/v1/auth/login?client_url=https://minha-app.com.br/callback&conselho=CRM&uf=RJ&documento=123456"
# -> 302 Location: https://minha-app.com.br/callback?session_id=<session_id>

# 2. Exchange the one-time session_id for a Bearer access_token
curl "http://localhost:3000/sncr/api/v1/auth/token?session_id=<session_id>"
# -> {"access_token": "sncr_...", "token_type": "Bearer"}

# 3a. Request notification numbers (Notification A/B/B2/RR/RT)
curl -X POST http://localhost:3000/sncr/api/v1/numeracoes/notificacao-receita \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"receita":"NRA","conselho":"CRM","uf":"RJ","documento":"123456","quantidade":25}'
# -> 201 {"numeroReceita": ["2411.1-64.0000001", ...], "saldoReceitas": 25, "mensagem": "..."}

# 3b. Request a special-control/retention range (RCE/RET)
curl -X POST http://localhost:3000/sncr/api/v1/numeracoes/receita-especial-retencao \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"conselho":"CRM","tipo":"RCE","documento":"123456","uf":"RJ","cnpj":"11222333000181"}'
# -> 201 {"inicio":"2411.6-64.0000001","fim":"2411.6-64.0001000","quantidade":1000,"mensagem":"..."}
```

## Business rules enforced

- Both numeracoes endpoints require `Authorization: Bearer <access_token>`; the token's bound
  conselho/uf/documento must match the request body's, or the call fails with **404** — mirroring the real
  API's "authenticated professional doesn't match the requested prescriber" rule (not a 401/403 — this is the
  documented status code).
- `notificacao-receita`: `quantidade` must be 10–50. Numbers are discrete (an array), balance is per
  receita-type/prescriber/**day**, capped at 50. Requesting more than what's left in the day returns **400**;
  requesting when the day's balance is already exhausted returns **204** with no body.
- `receita-especial-retencao`: always allocates exactly 1000 numbers as an `inicio`/`fim` range. `cnpj` must
  pass the standard Brazilian check-digit algorithm. Limit is **3 requests/month and 3000 numbers/month,
  combined across RCE+RET** for a given prescriber — exceeding either returns 400.
- `client_url` on the login call must be an http(s) URL on a `.br` domain, per the real API's documented
  callback domain whitelist.

## Known limitations / judgment calls

- **Number format** (`"2411.1-00.0000001"`-shaped strings): the manual shows the *shape* of a prescription
  number but never documents what its internal segments mean. The simulator folds in the receita/tipo and UF
  deterministically so numbers look plausible and don't collide, but this encoding is not a claim about the
  real one — consumers should treat the whole string as opaque (this matches how GolgiMed is expected to use
  it: persist as-is, never decode).
- **"Exceeding remaining daily stock" (`notificacao-receita`)**: the manual's endpoint description says a
  request for more than what's left returns only the available numbers ("no truncation" language is absent,
  but the tone reads that way), while its error table separately lists a 400 "Limite diário de 50 receitas
  atingido" case. This simulator treats *any* request for more than the current remaining balance as the 400
  path (never silently truncating the requested `quantidade`), and reserves 204 for "balance is already at
  zero." If live homologation testing shows the real API silently truncates instead, this should change.
- **Error message text** for the 401 (missing/invalid Bearer) and 404 (identity mismatch) cases isn't given
  verbatim in the manual — only the status codes are documented. The simulator uses placeholder Portuguese
  messages; treat the status codes as authoritative, not the exact wording.
- **`state` validation on `/auth/login`**: the real API HMAC-signs and validates a `state` parameter as part
  of CSRF protection through the Keycloak hop. Since the simulator skips that hop entirely, `state` is
  accepted and echoed back on the redirect but never validated — there's no CSRF surface to protect in a
  same-process fake.

## Fault injection

Standard Mimic fault injection applies via `PUT /admin/faults` (provider `sncr`, one of the four route paths
above) — useful for exercising GolgiMed's retry/error handling for the numeracoes calls without a real ANVISA
outage.
