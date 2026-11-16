# Revunk

Revunk is a pull request review tool with a Bubble Tea terminal interface. The review logic lives apart from the terminal UI so a future web or mobile client can reuse the same review service.

## Requirements

- Go 1.26 or later

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
| `q`, `ctrl+c`          | Quit                             |

## CLI

```
revunk [tui]   Launch the terminal review interface (default)
revunk serve   Start the review API server
```

## Project layout

| Package           | Responsibility                                                                                                               |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `cmd/revunk`      | CLI entry point and dependency wiring                                                                                        |
| `internal/review` | Review domain types, the `Source` and `Store` boundaries, and the `Service` that all clients use. It has no UI dependencies. |
| `internal/github` | GitHub integration boundary implementing `review.Source`                                                                     |
| `internal/store`  | Persistence boundary implementing `review.Store`                                                                             |
| `internal/tui`    | Bubble Tea presentation layer                                                                                                |

## Current limitations

This is the bootstrap version, so a few parts are still placeholders:

- `revunk serve` is not implemented yet. It exits with an error.
- `internal/github` returns fixed sample files instead of calling the GitHub API.
- `internal/store` keeps viewed state in memory, so it is lost when the program exits.
