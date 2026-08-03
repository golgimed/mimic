# Mimic — Architecture & Engineering Review

**Reviewer stance:** principal/staff engineer, adopting long-term maintainership.
**Scope:** full codebase as of `feature/openapi-spec` (uncommitted Go rewrite, TS implementation being removed).
**Date:** 2026-08-02

> **Update 2026-08-02 (release audit):** every P0 and P1 recommendation below
> is implemented, as is nearly all of P2 (probability/latency-distribution
> behavior, rate limiting as a fault kind, the `x-mimic-behavior` vendor
> extension). The one remaining open item is the OpenAPI parser-library
> migration under [Product Vision](#product-vision) — deferred, mitigated by
> the loud unsupported-construct warnings already in place, and tracked in
> the README's Roadmap. Treat the rest of this document as historical record
> of what motivated those changes, not an open TODO list.

---

## Executive Summary

Mimic's Go rewrite is in genuinely good shape. It has almost no accidental complexity: zero custom interfaces, no DI framework, no service/repository layers, a single-connection SQLite store, and a provider model (`registry.Provider`) that is a plain struct of closures rather than an interface hierarchy. This is exactly the shape the project's own CLAUDE.md asks for, and the team has stuck to it.

The two real findings are:

1. **The codebase is ahead of its documentation and slightly ahead of its own consistency.** The OpenAPI adapter is fully built and tested (16/16 passing) but unmentioned in the README. IntegraICP and Zenvia model state at different levels of rigor (one has a real state machine, one doesn't). One env var (`MIMIC_PROVIDERS`) bypasses `Config` entirely. These are all small, fast fixes.
2. **The product vision ("simulate how APIs behave, not just what they return") is not yet architecturally supported.** Fault injection today is deterministic and manually configured per-route (delay/status/timeout/invalid-payload/webhook-drop). There is no probabilistic behavior, no latency distributions, no rate limiting, no per-operation policy attached declaratively to an OpenAPI spec. This is the one area that needs real design work, not cleanup — see [Product Vision](#product-vision).

Nothing in this review argues for a rewrite or a new layer. Every recommendation is either "delete this," "merge these two things," or "add one small declarative feature." That is a good sign for a project this size.

---

## What Should Be Removed

| Item | Why |
|---|---|
| 4 of the 5 migration files (`0002`–`0005`), squashed into `0001` or a single baseline | All are pure additive `CREATE TABLE`, no `ALTER`s, no production data to preserve migration history for. Mimic is a stateless simulator — a fresh `_migrations` baseline is strictly simpler to read than 5 files for one reader to reconstruct the schema from. |
| `lane.RunHealthCheck` / Mimic's own static `/health` handler — pick one | Two disconnected health-check mechanisms currently coexist: Lane's own `HealthState`/readiness gating (imported, unused) and a hand-rolled always-200 `/health` in `core/mux.go`. Keeping both is dead weight; only one is real. |
| Duplicated `writeJSON`/error-envelope helpers in `integraicp/handlers.go` and `zenvia/handlers.go` | Same ~15 lines in two files. Not a big deal today, but it's the first thing that will drift when a third provider is added. |

Nothing else qualifies as dead code — the exploration found no unused functions, no abandoned packages, no orphaned config, no unreachable handlers. That itself is worth noting: this is an unusually clean codebase for its stage.

---

## What Should Be Simplified

**Config discipline.** `internal/registry/registry.go:90` reads `os.Getenv("MIMIC_PROVIDERS")` directly instead of going through `internal/config`. This is the only config leak in the entire app (verified by grepping for `os.Getenv` across `internal/` and `cmd/` — everywhere else is clean). Move it into `Config` as `EnabledProviders []string`, loaded once at startup like everything else. Effort: 15 minutes. Impact: closes the only inconsistency in an otherwise disciplined config story.

**`Config.Load()` validation.** It currently cannot fail — invalid ints/bools/durations silently fall back to defaults with no logging, and `LOG_LEVEL` is parsed separately in `main.go` rather than in `config.go`. This isn't urgent (nothing is exploitable), but a misconfigured `PORT=abc` or `SCHEDULER_INTERVAL_MS=-1` currently fails silently instead of loudly. Recommend: `Load()` returns `(Config, error)`, folds `parseLevel` into the same function, and `main.go` exits with a clear message on error instead of running with silently-wrong defaults.

**IntegraICP's state model.** Zenvia tracks real persisted state (`ACCEPTED → SENT → DELIVERED`) with a scheduler-driven transition table. IntegraICP's handlers return hardcoded status literals (`"AUTHENTICATED"`, `"COMPLETED_WITH_SUCCESS"`) that were never written to `integraicp_credentials` as a tracked field. This isn't wrong per the official IntegraICP contract (some of those statuses may genuinely be synchronous in the real API), but it means the two providers demonstrate inconsistent simulation depth. Worth an explicit decision: either document that IntegraICP's flow is intentionally synchronous (per the real docs), or add a `status` column and drive it the same way Zenvia does, so the pattern is uniform for the next provider author to copy.

---

## Quick Wins

- Fix the `MIMIC_PROVIDERS` env leak (above). ~15 min.
- Add OpenAPI adapter section to top-level `README.md` — feature is fully built and tested but invisible to a new reader; `specs/README.md` already has the content to summarize. ~30 min.
- Squash migrations `0001`–`0005` into one baseline file. ~30 min, zero risk (no real data to migrate).
- Wire `/health` to an actual dependency check (DB ping) instead of a static stub, and either use Lane's `HealthState` or drop the import if it stays unused. ~1 hr.
- Extract `writeJSON`/error-envelope helpers into `internal/shared/httpx`. ~30 min, prevents drift before the next provider copies the duplicated version.

---

## High Impact Refactors

### 1. Declarative runtime-behavior policies (the actual product-vision gap)

This is the one substantial piece of new design work — see [Product Vision](#product-vision) below for the full proposal. Effort: medium (1–2 weeks for a first cut: probability-based fault selection + latency distributions, reusing the existing `fault_config` table and `RequestFaultHook` middleware). Impact: high — this is the difference between "mock server" and "realistic simulation platform," which is the explicit stated goal.

### 2. OpenAPI parser upgrade

The hand-rolled OpenAPI 3.0 YAML decoder (`internal/openapi/loader.go`) has no `$ref`, `components`, or `oneOf/allOf/anyOf` support — already flagged in the code itself as a deliberate, timeboxed gap. This will start failing silently (or loudly) the moment someone points it at a real-world spec with shared component schemas, which is nearly all of them. Recommend the spike the code comment already promises: evaluate `kin-openapi` (mature, widely used, 3.0-focused) vs. `pb33f/libopenapi` (3.1 + JSON Schema 2020-12, more modern). Effort: 3–5 days including route-generation rewiring. Impact: high — this determines whether the OpenAPI adapter can handle real vendor specs instead of only hand-crafted test fixtures.

### 3. Namespace strategy for multi-spec conflicts

Already implemented and working (`internal/openapi/merge.go`: `strict`/`merge`/`priority` modes, plus prefix derivation from `x-mimic-name` or slugified title). The current design is sound — prefix-per-spec is the right call over a flat namespace, since it's the only approach that scales to N unrelated third-party specs without a central conflict registry. No change recommended here; it's called out because the review brief asked for one, and the answer is "the current design already got this right."

---

## Architectural Risks

- **Scheduler is currently single-tenant.** `internal/shared/scheduler` is a generic job queue but only Zenvia registers a handler (`zenvia:advance`). Fine today; if a future provider or the OpenAPI adapter needs async state progression, verify the `Tick`/`JobHandler` dispatch-by-`kind` model still holds under multiple concurrent handler kinds — it should, but hasn't been exercised that way yet.
- **Single DB connection (`SetMaxOpenConns(1)`).** Correct and deliberate for SQLite/WAL correctness today, but it caps throughput under load-testing scenarios (e.g., simulating high-concurrency chaos testing per the product vision). Not a problem now; worth revisiting once runtime-behavior simulation adds concurrent-request scenarios as a first-class use case.
- **No `$ref` support is a landmine for adoption**, not just a missing feature — a new user's first real spec will very likely use `components/schemas` and hit silent degradation (static-fallback stub bodies) rather than a clear error. Recommend adding an explicit "unsupported spec construct" warning/error at load time until the parser upgrade lands, so failure is loud, not silent.

---

## Future Architecture

The existing shape is close to right — this section is about extending it, not replacing it:

```
cmd/mimic/                     # entrypoint, flag parsing, wiring only
internal/
    registry/                  # Provider{Register, ListItems, GetItemDetail} — no interfaces
    core/                      # mux, CORS, /health (should become a real check), /dashboard
    config/                    # single Config struct, one Load() (should return error)
    providers/
        providers.go           # RegisterAll — add new providers here only
        integraicp/, zenvia/   # hand-written, one per real provider
    openapi/                   # spec-driven adapter — loader, route model, merge, CRUD store
    shared/
        httpx/                 # NEW: writeJSON/error envelope, shared by all providers
        behavior/               # NEW: declarative runtime-behavior policies (see Product Vision)
        auth/, faults/, scheduler/, storage/, webhooks/, admin/
db/migrations/                 # one baseline + future real deltas
specs/                         # user-dropped OpenAPI specs
dashboard/                     # embedded static HTML
```

Two additions only: `internal/shared/httpx` (dedup) and `internal/shared/behavior` (the new policy engine). Everything else stays. No new layers, no new interfaces, no plugin system — consistent with the project's own stated philosophy.

---

## Product Vision

> "Mimic should simulate how APIs behave, not just what they return."

**Current state:** fault injection is real and dynamic (`fault_config` table, `PUT /admin/faults`, request-time and webhook-time kinds), but it is *deterministic and manually triggered* — an operator sets "this route returns 500 for the next 3 requests." That's fault injection, not behavior simulation. It's a solid foundation, not a replacement for what's being asked for.

**What's missing, mapped onto the existing fault system rather than a parallel one:**

- **Probability-based faults.** Extend `FaultConfig` with an optional `probability float64` (0–1). `ConsumeMatchingFault` already does exact-match-wins-over-provider-wide resolution — add a coin-flip gate before returning a match. This alone unlocks "5% of requests to `/messages` return 503" without any new subsystem.
- **Latency distributions instead of a fixed delay.** `faults/delay.go` currently sleeps a fixed `time.Duration`. Replace with a small distribution type (`fixed`, `uniform(min,max)`, `normal(mean,stddev)`) — a few dozen lines, no new dependency needed (stdlib `math/rand` suffices).
- **Rate limiting as a policy, not a provider concern.** A per-route token-bucket middleware, configured the same way faults are (via `admin` routes), returning `429` once budget is exhausted — this is genuinely the same shape as existing fault injection and should live next to it, not as a separate feature.
- **Per-operation attachment via OpenAPI vendor extensions.** For spec-driven routes, allow `x-mimic-behavior` in the spec itself (latency range, error rate) as an alternative to configuring it via `/admin/faults` after the fact — this is what makes behavior simulation "declarative" per the vision statement, and reuses the same `x-mimic-*` vendor-extension convention already established for `x-mimic-name` and `x-mimic-persist`.

**What this deliberately does NOT need:** a chaos-engineering framework, a rules DSL, or a policy plugin system. The existing `fault_config` table + `RequestFaultHook` middleware is the right foundation — this is additive fields and one new middleware, not a new architecture.

**Verdict:** the current implementation is compatible with this vision but hasn't built the probabilistic/declarative layer yet. This is expected — it's clearly the next major feature, not a course-correction.

---

## Prioritized Roadmap

### P0 — Do Immediately
- Fix `MIMIC_PROVIDERS` env leak into `Config`.
- Document the OpenAPI adapter in the top-level README.
- Add loud warnings/errors for unsupported OpenAPI constructs (`$ref`, `components`) instead of silent stub fallback.

### P1 — Next Release
- Squash migrations into one baseline.
- Extract shared `httpx` helpers.
- Decide and document IntegraICP's state-tracking approach (synchronous-by-design vs. add real state column).
- Wire `/health` to a real dependency check; remove the unused half of the Lane/hand-rolled health duality.
- `Config.Load()` returns an error; fold `parseLevel` in.

### P2 — Future
- Probability-based + distribution-based fault injection (`behavior` package).
- Rate-limiting middleware reusing the fault-config pattern.
- `x-mimic-behavior` vendor extension for declarative per-operation simulation.
- OpenAPI parser spike + migration to `kin-openapi` or `pb33f/libopenapi` for `$ref`/`components`/3.1 support.

---

## Final Thoughts

This codebase does not need to be argued out of enterprise patterns — it never adopted them in the first place. No interfaces exist that only have one implementation, because no interfaces exist at all outside the one the external `lane` library requires. No repository layer, no service layer, no builder/factory ceremony. That restraint is the single best architectural decision in the project, and it should be actively protected as new features (behavior simulation, richer OpenAPI support) get added — the temptation with a "policy engine" feature especially will be to reach for an interface-per-policy-type design. Resist it; a struct of typed fields plus a switch, matching the existing `FaultConfig` shape, is enough.

The one place where the codebase is *behind* its own ambition is the product vision itself — not because of any wrong decision, but because probabilistic/declarative behavior simulation simply hasn't been built yet. That's a feature to schedule, not a defect to fix.
