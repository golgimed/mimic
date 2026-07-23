# CLAUDE.md

# Mimic

Mimic is an engineering tool used to simulate third-party providers during local development, integration testing and CI. Like the D&D monster, it looks like the real thing (the provider's API contract) but does something else underneath (deterministic, controllable fake behavior).

The goal is to emulate external services with predictable behavior while keeping the implementation as simple as possible.

## Philosophy

The simulator is **not** a production system.

Always prefer:

- simplicity
- readability
- deterministic behavior
- fast iteration

Never sacrifice simplicity for unnecessary abstractions.

When in doubt, choose the smaller solution.

---

# Goals

The simulator should:

- emulate external provider APIs
- emulate provider state transitions
- emulate asynchronous processing
- emulate webhooks
- allow fault injection
- support local development
- support automated integration tests

The simulator should **not**:

- reproduce every provider implementation detail
- implement unnecessary business rules
- become a generic API Gateway
- become a production-ready platform

---

# Supported Providers

Current providers:

| Provider | Purpose | Official Documentation |
|-----------|---------|------------------------|
| IntegraICP | Digital Signature | https://developers.integraicp.com.br/api-reference/icp/v3/index.html |
| Zenvia | SMS, WhatsApp, Email | https://zenvia.github.io/zenvia-openapi-spec/v2/ |

Future providers should be added incrementally.

**Rules**

- Official provider documentation is the source of truth.
- Never invent request or response contracts.
- Always prefer compatibility over approximation.
- If provider behavior is undocumented, document the assumption and keep the implementation isolated so it can be replaced when access to the official sandbox becomes available.

---

# Architecture

Keep the architecture simple.

```
src/
    providers/
        integraicp/
        zenvia/
    shared/
        auth/
        scheduler/
        storage/
        webhooks/
        admin/
    server.ts
```

Providers own their own routes, handlers and state.

Shared components should only exist when reused by at least two providers.

---

# Technology Stack

- TypeScript
- Node.js 22+
- Fastify
- Zod
- SQLite (better-sqlite3)
- Docker

Avoid frameworks unless they significantly simplify development.

Always use context7

---

# Development Principles

Always:

- implement the smallest increment
- keep providers isolated
- keep endpoints compatible with official documentation
- preserve request/response contracts
- simulate asynchronous behavior
- make failures reproducible

Never:

- overengineer
- create plugin systems
- introduce Clean Architecture
- introduce DDD
- create repositories/services unless clearly justified

Simple modules are preferred.

---

# State

The simulator owns state.

Examples:

Messages

```
QUEUED

↓

SENT

↓

DELIVERED
```

Signature Requests

```
PENDING

↓

SIGNED
```

or

```
PENDING

↓

REJECTED
```

or

```
PENDING

↓

EXPIRED
```

State transitions should be deterministic.

---

# Fault Injection

Every provider should support configurable failures.

Examples:

- artificial delay
- timeout
- HTTP 500
- HTTP 429
- HTTP 503
- invalid webhook
- dropped webhook

Fault injection must be configurable without changing code.

---

# Dashboard

A lightweight dashboard is encouraged.

Purpose:

- inspect state
- inspect requests
- inspect scheduled jobs
- inspect webhooks
- trigger failures

Do not build an enterprise admin panel.

---

# Workflow

For every task:

1. Read the provider documentation.
2. Identify affected provider.
3. Implement the smallest increment.
4. Validate compatibility.
5. Update documentation.

---

# Documentation

Official provider documentation is the source of truth.

Never invent request or response contracts.

If behavior is unknown:

- document the assumption
- keep implementation isolated
- make it easy to replace later

---

# Testing

Prefer integration tests over unit tests.

Test:

- happy path
- invalid requests
- state transitions
- webhooks
- failure scenarios

---

# Definition of Done

A task is complete only when:

- implementation is complete
- tests pass
- documentation is updated