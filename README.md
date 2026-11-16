# Revunk

Revunk is a pull request review tool with a Bubble Tea terminal interface. The review logic lives apart from the terminal UI so a future web or mobile client can reuse the same review service.

## Requirements

- Go 1.26 or later
- [Claude Code](https://claude.com/claude-code) CLI (`claude`) on your `PATH` and logged in, to ask questions about a diff

## Commands

| Task        | Command                                     |
| ----------- | ------------------------------------------- |
| Build       | `go build -o bin/revunk ./cmd/revunk`       |
| Run the TUI | `go run ./cmd/revunk` or `./bin/revunk tui` |
| Format      | `gofmt -w .`                                |
| Vet         | `go vet ./...`                              |
| Test        | `go test ./...`                             |

### TUI keys

| Key                    | Action                           |
| ---------------------- | -------------------------------- |
| `↑`/`k`, `↓`/`j`       | Select the previous or next file |
| `pgup`/`b`, `pgdn`/`f` | Scroll the diff                  |
| `v`                    | Toggle the file's viewed state   |
| `c`                    | Ask Claude about the file's diff |
| `q`, `ctrl+c`          | Quit                             |

### Asking Claude

Press `c` to type a question about the selected file. `enter` sends it, and `esc` cancels. Revunk sends the question, the pull request, and the file's patch to `claude -p`. The answer shows below the file's diff and stays there while the app runs.

## CLI

```
revunk [tui]   Launch the terminal review interface (default)
```

## Project layout

| Package                    | Responsibility                                                                                                                         |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/revunk`               | CLI entry point and dependency wiring                                                                                                  |
| `internal/review`          | Review domain types, the `Source`, `Store`, and `Agent` boundaries, and the `Service` that all clients use. It has no UI dependencies. |
| `internal/provider/github` | GitHub provider implementing `review.Source`. Other hosts such as GitLab go in sibling packages.                                       |
| `internal/agent/claude`    | Claude Code CLI agent implementing `review.Agent`. It runs `claude -p` with the prompt on stdin.                                       |
| `internal/store`           | Persistence boundary implementing `review.Store`                                                                                       |
| `internal/tui`             | Bubble Tea presentation layer                                                                                                          |
