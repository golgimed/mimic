# Contributing

Thanks for considering a contribution. Before anything else, read
[CLAUDE.md](CLAUDE.md) — it's the project's design philosophy and covers what
Mimic is (and deliberately isn't). Changes that go against it (new
frameworks, generic abstractions, reproducing provider business rules that
aren't needed for testing) are unlikely to be accepted regardless of code
quality.

## Setup

Requires Go 1.26+.

```bash
go build ./...
go vet ./...
gofmt -l .        # should print nothing
go test -race ./...
```

## Workflow

1. Read the relevant provider's documentation (or official spec) before
   touching its handlers — never invent request/response contracts.
2. Identify the smallest change that solves the problem. Prefer editing
   existing files over adding new abstractions.
3. Add or update tests. Prefer integration tests (`tests/`, via
   `internal/testutil.App`) over unit tests — see existing `tests/*/*.go`
   for the pattern.
4. Run `gofmt -w .` before committing — CI checks formatting.
5. Update the relevant README (top-level or provider-specific) if behavior
   changed.

## Adding a new provider

See [README.md](README.md#adding-a-new-provider) — briefly: create
`internal/providers/<name>/` with routes, handlers, state, and a `README.md`,
then register it in `internal/providers/providers.go`. No other file should
need to change.

## Releases

Tags follow SemVer (`vMAJOR.MINOR.PATCH`). Maintainers cut a release by
tagging `main` and pushing the tag — `git tag vX.Y.Z && git push origin
vX.Y.Z` — which triggers `.github/workflows/release.yml` to cross-compile
binaries, publish a GitHub Release with checksums, and push the container
image to GHCR.

## Pull requests

Keep PRs scoped to one change. Explain the *why* in the description, not
just the *what* — the diff already shows what changed. If behavior is
undocumented in the provider's official docs, say so explicitly and keep the
implementation isolated so it's easy to replace later.
