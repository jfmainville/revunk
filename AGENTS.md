# AGENTS.md

Revunk is a pull request review tool written in Go with a Bubble Tea terminal interface.

## Commands

- Build: `go build -o bin/revunk ./cmd/revunk`
- Run: `go run ./cmd/revunk`
- Format: `gofmt -w .`
- Vet: `go vet ./...`
- Test: `go test ./...`
- Coverage: `go test -cover ./...`

Run format, vet, and tests before every commit. All must pass. CI runs the same checks on every pull request through `.github/workflows/validation.yml`, which calls the reusable `.github/workflows/validation-go.yml`.

## Architecture

- `internal/review` holds the domain types, the `Source`, `Store`, and `Agent` interfaces, and `Service`. It must never import UI, provider, or storage packages.
- `internal/provider/<name>` implements `review.Source` for one source of changes (local git now, code hosts such as GitHub later).
- `internal/agent` implements `review.Agent` by reaching a coding agent through `herdr`.
- `internal/store` implements `review.Store`.
- `internal/tui` is the only package allowed to import Charm libraries (`bubbletea`, `bubbles`, `lipgloss`).
- Every key binding lives in `defaultKeys()` in `internal/tui/keys.go`, together with the help screen and footer hints. Match keys with `is(key, m.keys.X)` and never compare key strings anywhere else.
- `cmd/revunk` only parses arguments and wires dependencies together.

## Simplicity

Always choose the simplest solution that fully meets the requirement.

- Write plain, direct code. No abstractions, interfaces, config, or generalization until there is a real second use.
- Prefer the standard library. Add a dependency only when it replaces substantial code and is popular and maintained.
- Reuse existing types and functions before writing new ones. Extend instead of duplicating.
- Inline one-off helpers. Delete dead code, unused exports, and stale files in the area you touch.
- Don't add features, flags, or placeholder commands that aren't needed yet.
- No code comments unless the code can't be made clear on its own. Never leave commented-out code.

## Tests

- Every function gets a unit test, written in the same change as the function.
- Tests must check behavior and fail when the behavior breaks. No tests that only restate the implementation or assert that code runs.
- Cover error paths and edge cases (empty input, boundaries, failing dependencies), not only the happy path.
- Use small hand-written fakes for `review.Source` and `review.Store`. No mocking libraries.
- Test the TUI by sending messages to `Model.Update` and checking the model state or rendered output.
- Only code that needs a real terminal, such as `main()` and starting the Bubble Tea program, may go untested.

## Docs

- `README.md` covers what Revunk is, requirements, commands, keys, CLI usage, and the project layout. Update it in the same change as any behavior, command, or layout change.
- Longer technical docs (architecture, design decisions) go in `docs/`, kept short and focused on what the code can't show.
- Document only what exists. No roadmaps, placeholders, or "not implemented yet" notes.
- Format Markdown with `npx prettier --write`.
