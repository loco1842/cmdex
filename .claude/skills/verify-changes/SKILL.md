---
name: verify-changes
description: "Run the project's full build/lint/test verification suite after making code changes and before committing. Trigger when asked to verify, check, validate, or confirm a change works, or before creating a commit."
---

## Verify changes (commamer)

Run this after making backend (Go) or frontend (TypeScript/React) changes, before committing.

1. Ensure the frontend dist placeholder exists (some targets need it): `mkdir -p frontend/dist`.
2. Compile/typecheck: `make check` (runs `go build ./...` and `cd frontend && pnpm tsc --noEmit`). For a Go-only quick check you can run `go build ./...` and `go vet ./...` directly.
3. Lint: `make lint` (runs `golangci-lint run` for Go and `cd frontend && pnpm lint` for the frontend). This is a hard gate in CI (not advisory) — it must exit 0.
4. If lint or formatting issues are found, auto-fix with `make fmt` (`golangci-lint fmt` + `cd frontend && pnpm lint:fix`), or `make lint-fix` to also auto-fix lint violations, then re-run `make lint` to confirm clean.
5. Test: `make test` (runs `go test ./...`, then `cd frontend && pnpm test` for Vitest unit tests, then `cd frontend && pnpm test:e2e` for Playwright). For a faster Go-only pass use `go test ./...`.
6. After the suite passes, run `git status` and check for stray/regenerated artifacts that should or shouldn't be committed (e.g. `frontend/dist/.gitkeep`, `frontend/bindings/**` from `wails3 generate bindings`, stale build outputs) — stage or restore as appropriate.
7. Only proceed to commit once build, lint, and tests all pass cleanly.

Before relying on the exact commands above, confirm the current Makefile targets (`cat Makefile`) and `frontend/package.json` scripts, since these have changed over time (e.g. lint moved from `gofmt`/`golint` to `golangci-lint`, and enforcement moved from advisory `|| true` to strict).

Not needed for tiny doc-only or comment-only edits with no code impact.
