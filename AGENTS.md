# AGENTS.md

Revunk is a pull request review tool written in Go with a Bubble Tea terminal interface.

## Commands

- Build: `go build -o bin/revunk ./cmd/revunk`
- Run: `go run ./cmd/revunk`
- Format: `gofmt -w .`
- Vet: `go vet ./...`
- Test: `go test ./...`
- Coverage: `go test -cover ./...`

Format, vet, and test must all pass before every commit. CI runs the same checks through `.github/workflows/validation.yml`, which calls `.github/workflows/validation-go.yml`. Pushes to `main` run `.github/workflows/release.yml`, which versions releases from conventional commits with `semantic-release` (see `docs/releases.md`).

## Architecture

- `internal/review` holds the domain types, the `Source`, `Store`, `Host`, and `Agent` interfaces, and `Service`. It never imports UI, provider, or storage packages.
- `internal/provider/<name>` implements `review.Source` for one change source: `git` for the local branch, and `github` for pull requests through `gh`, which also implements `review.Host` and the `review.Store` comments of a pull request's review.
- `internal/agent` implements `review.Agent` through `herdr` and `tuios`, and `agent.New` picks one at startup from `TUIOS_PANE_ID`.
- `internal/store` implements `review.Store`.
- `internal/tui` is the only package that imports Charm libraries (`bubbletea`, `bubbles`, `lipgloss`).
- Every key binding lives in `defaultKeys()` in `internal/tui/keys.go`, with the help screen and footer hints. Match keys with `is(key, m.keys.X)`, never by comparing key strings.
- `cmd/revunk` only parses arguments and wires dependencies together.

## Simplicity

Choose the simplest solution that fully meets the requirement.

- Write plain, direct code. No abstractions, interfaces, config, or generalization until there is a real second use.
- Prefer the standard library. Add a dependency only when it replaces substantial code and is popular and maintained.
- Reuse and extend existing types and functions before writing new ones.
- Delete dead code, unused exports, and stale files in the area you touch. Inline one-off helpers.
- Add no features, flags, or placeholder commands before they are needed.
- Write no comments unless the code can't be clear on its own. Never leave commented-out code.

## Tests

- Every function gets a unit test in the same change.
- Tests check behavior and must fail when it breaks. Don't write tests that restate the implementation or only assert that code runs.
- Cover error paths and edge cases (empty input, boundaries, failing dependencies), not just the happy path.
- Use small hand-written fakes for `review.Source` and `review.Store`. No mocking libraries.
- Test the TUI by sending messages to `Model.Update` and checking the model state or rendered output.
- Only code that needs a real terminal, such as `main()` and starting the Bubble Tea program, may go untested.

## Docs

- `README.md` covers what Revunk is, its requirements, commands, keys, CLI usage, and project layout. Don't update it on every change. Edit it only when the user asks, or when a change makes one of its lines false, and then change only that line.
- Put longer technical docs (architecture, design decisions) in `docs/`, short and focused on what the code can't show.
- Document only what exists. No roadmaps, placeholders, or "not implemented yet" notes.
- Format Markdown with `npx prettier --write`.
