# Revunk

Revunk is a pull request review tool with a Bubble Tea terminal interface. It reviews the commits on the current git branch, such as changes made by a coding agent, as one continuous diff with inline comments. The review logic lives apart from the terminal UI so a future web or mobile client can reuse the same review service.

## Requirements

- Go 1.26 or later
- `git`
- `claude` (Claude Code), and optionally [herdr](https://herdr.dev) or `tmux`, only to send comments with `S`

## Commands

| Task        | Command                                     |
| ----------- | ------------------------------------------- |
| Build       | `go build -o bin/revunk ./cmd/revunk`       |
| Run the TUI | `go run ./cmd/revunk` or `./bin/revunk tui` |
| Format      | `gofmt -w .`                                |
| Vet         | `go vet ./...`                              |
| Test        | `go test ./...`                             |

## Agent tab

Run Revunk from inside a git repository. The **Agent** tab compares the current branch with its base branch (`base...HEAD`).

- **commits** lists `all commits` and each commit on the branch. Selecting one shows only that commit's diff.
- **files** lists the changed files grouped by directory. Selecting a file scrolls the diff to it.
- **continuous diff** shows every file one after another, each under a header with its status, line counts, and viewed state. Added and removed lines are tinted, both line numbers are shown, and code is syntax highlighted.
- Comments are drawn inline under the line, range, or file header they belong to. They support Markdown, and a ` ```suggestion ` block is shown as a suggested change against the commented lines.
- A file marked viewed collapses and stays viewed until its diff changes.

Comments and viewed state are saved per branch in `.git/revunk.json`.

`S` sends every comment on the branch to the Claude Code or opencode session open in this repository, and `y` copies the same text to the clipboard. The text lists each comment with its file, lines, commit, and the commented code. `S` picks the first Claude Code or opencode session whose directory is inside the repository, skipping the pane Revunk runs in, and tries these in order.

1. A `herdr` pane listed by `herdr agent list`, prompted with `herdr agent prompt`.
2. A `tmux` pane running `claude` or `opencode`, pasted into with `tmux paste-buffer` and submitted with `Enter`.
3. With no session open, a new Claude Code session that gets the comments as its first message, in a new `herdr` pane when Revunk runs in `herdr`, a new `tmux` window when it runs in `tmux`, and otherwise in the background with `claude --bg` (list it with `claude agents`).

### Keys

| Key                    | Action                                                 |
| ---------------------- | ------------------------------------------------------ |
| `tab`, `shift+tab`     | Cycle focus between commits, files, and diff           |
| `h`, `l`               | Focus the files or the diff                            |
| `j`/`↓`, `k`/`↑`       | Move down or up                                        |
| `gg`, `G`              | Go to the top or bottom of the diff                    |
| `ctrl+d`, `ctrl+u`     | Move half a page down or up                            |
| `pgdn`/`f`, `pgup`/`b` | Move a page down or up                                 |
| `]f`, `[f`             | Next or previous file                                  |
| `}`, `{`               | Next or previous hunk                                  |
| `v`, `V`               | Start or stop selecting a range of lines               |
| `c`                    | Comment on the current line, the range, or file header |
| `C`                    | Comment on the whole file                              |
| `dd`                   | Delete the comment under the cursor                    |
| `r`                    | Toggle the file's viewed state                         |
| `S`                    | Send every comment to Claude Code or opencode          |
| `y`                    | Copy every comment to the clipboard                    |
| `?`                    | Show every key                                         |
| `esc`                  | Stop selecting or close the help                       |
| `q`, `ctrl+c`          | Quit                                                   |

### Comment editor

| Key                | Action                                                             |
| ------------------ | ------------------------------------------------------------------ |
| `ctrl+s`           | Save the comment                                                   |
| `enter`            | Insert a newline                                                   |
| `tab`, `shift+tab` | Cycle the type (comment, issue, suggestion, nit, question, praise) |
| `ctrl+g`           | Insert a ` ```suggestion ` block with the commented lines          |
| `esc`              | Cancel                                                             |

## CLI

```
revunk [-base <branch>] [tui]   Launch the terminal review interface (default)
```

`-base` sets the branch to compare against. It defaults to the remote's default branch (`origin/HEAD`), or `main` when there is none.

## Project layout

| Package                 | Responsibility                                                                                                                                  |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/revunk`            | CLI entry point and dependency wiring                                                                                                           |
| `internal/review`       | Review domain types, diff line parsing, the `Source` and `Store` boundaries, and the `Service` that all clients use. It has no UI dependencies. |
| `internal/provider/git` | Local git provider implementing `review.Source` for the commits on the current branch. Code hosts such as GitHub go in sibling packages.        |
| `internal/agent`        | Sends prompts to a Claude Code or opencode session through `herdr` or `tmux`, or starts a new Claude Code session, implementing `review.Agent`  |
| `internal/store`        | JSON file store implementing `review.Store`                                                                                                     |
| `internal/tui`          | Bubble Tea presentation layer, with `chroma` for syntax highlighting and `glamour` for Markdown comments                                        |
