---
name: bump-dependencies
description: Bump Go module and frontend pnpm dependencies on a fresh branch from main, regenerate Wails bindings, and verify with build, lint, and tests. Use when the user asks to bump, upgrade, or update dependencies, packages, Wails, or supersede Dependabot PRs.
---

# Bump Dependencies

When asked to bump versions, follow these steps in order. Work on a new branch based on `main`, never on the current feature branch.

## 1. Create branch from main

```bash
git fetch origin main
git checkout -b chore/bump-<what>-and-frontend-deps origin/main
```

## 2. Discover latest versions

- Go: `go list -m -versions <module>` (e.g. `github.com/wailsapp/wails/v3`)
- Frontend: `npm view <pkg> version` for a specific package, `pnpm outdated` (run in `frontend/`) for the full picture
- Note the currently pinned versions in `go.mod` and `frontend/package.json` first

## 3. Bump Go modules

```bash
go get <module>@<version>
go mod tidy
```

Keep the Wails Go module (`github.com/wailsapp/wails/v3`) and the frontend `@wailsio/runtime` on matching versions.

## 4. Bump frontend packages

```bash
pnpm update --latest
```

Run in `frontend/`. Afterwards inspect `git diff frontend/package.json`:

- **Confirm major bumps with the user before keeping them** (e.g. a TypeScript major bump can break `tsc` on previously valid code). Revert offenders with `pnpm add [-D] <pkg>@<previous-range>`.
- Leave intentionally pinned packages untouched.

## 5. Regenerate bindings

```bash
wails3 generate bindings
```

Never hand-edit `frontend/bindings/` — it is generated output. If the command produces no diff, there was no API change and there is nothing to commit for bindings.

## 6. Verify

Use the repo's documented checks (see `AGENTS.md`):

- `go build ./...` and `go vet ./...`
- `go test ./...`
- In `frontend/`: `pnpm tsc --noEmit`, `pnpm lint`, `pnpm test`

If a check fails after the bump, isolate the cause: check out `main` in a temporary worktree (`git worktree add /tmp/<name> origin/main`), confirm the check passes there, then pin or revert the offending package on the bump branch. Remove the worktree when done (`git worktree remove --force <path>`).

## 7. Commit, push, PR (only when explicitly requested)

- Stage only the intended files (`go.mod`, `go.sum`, `frontend/package.json`, `frontend/pnpm-lock.yaml`, plus regenerated bindings if any).
- Commit with a conventional message, e.g. `chore: bump wails v3 to beta.16 and update frontend dependencies`.
- Push with `git push -u origin <branch>` and open the PR with `gh pr create --base main`.
- If the bump supersedes open Dependabot PRs, reference them in the PR body (`Fixes #<n>`). Note: `Fixes` auto-closes issues, not PRs — superseded PRs may need manual closing after merge.

## Checklist

- [ ] Branch created from `origin/main`
- [ ] Go modules bumped, `go mod tidy` clean
- [ ] Frontend packages bumped, `package.json` diff reviewed, majors confirmed
- [ ] `@wailsio/runtime` matches the Wails Go module version
- [ ] Bindings regenerated (or confirmed no-op)
- [ ] `go build`, `go vet`, `go test` pass
- [ ] `tsc --noEmit`, `lint`, `test` pass in `frontend/`
