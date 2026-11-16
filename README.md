# Revunk

Revunk is a pull request review tool with a Bubble Tea terminal interface. It reviews the commits on the current git branch, such as changes made by a coding agent, as one continuous diff with inline comments. The review logic lives apart from the terminal UI so a future web or mobile client can reuse the same review service.

## Requirements

- Go 1.26 or later
- `git`
- [herdr](https://herdr.dev) with Claude Code or opencode, only to send comments with `S`

## Commands

| Task        | Command                                     |
| ----------- | ------------------------------------------- |
| Build       | `go build -o bin/revunk ./cmd/revunk`       |
| Run the TUI | `go run ./cmd/revunk` or `./bin/revunk tui` |
| Format      | `gofmt -w .`                                |
| Vet         | `go vet ./...`                              |
| Test        | `go test ./...`                             |

## Agent tab

Run Revunk from inside a git repository. The **Agent** tab compares the current branch with its base branch, from their merge base to the working tree, so commits, staged and unstaged edits, and untracked files all show. Untracked files that are binary or larger than 1 MB are listed without a diff.

- **commits** lists `all changes` and each commit on the branch. Selecting a commit shows only that commit's diff.
- **files** lists the changed files grouped by directory. Selecting a file scrolls the diff to it.
- **continuous diff** shows every file one after another, each under a header with its status, line counts, and viewed state. Added and removed lines are tinted, both line numbers are shown, and code is syntax highlighted.
- Comments are drawn inline under the line, range, or file header they belong to. They support Markdown, and a ` ```suggestion ` block is shown as a suggested change against the commented lines.
- A file marked viewed collapses and stays viewed until its diff changes.

Comments and viewed state are saved per branch in `.git/revunk.json`.

`S` sends the comments not sent yet to the Claude Code or opencode session open in this repository, and `y` copies the same text to the clipboard. The text lists each comment with its ID, file, lines, commit, and the commented code, and asks the agent to answer each one with `revunk reply <id> "<text>"`. `S` uses `herdr agent list` to pick the first Claude Code or opencode pane whose directory is inside the repository, skipping the pane Revunk runs in. With no such pane, it starts Claude Code in a new pane next to Revunk. It submits the text with `herdr agent prompt` and marks the comments sent. Comments stay drafts when sending fails.

Revunk reloads commits, the diff, viewed state, and comments every 2 seconds, keeping the cursor on the same line. New commits and file edits therefore show up on their own, and agent replies appear under their comment, which moves from `draft` to `sent` to `replied`. A comment stays until you resolve it with `dd` or `DD`. Replies work with any agent that can run shell commands, since the prompt gives the full path of the running `revunk` binary.

### Keys

| Key                      | Action                                                 |
| ------------------------ | ------------------------------------------------------ |
| `tab`, `shift+tab`       | Cycle focus between commits, files, and diff           |
| `l`                      | Focus the diff                                         |
| `j`/`↓`, `k`/`↑`         | Move down or up                                        |
| `gg`, `G`                | Go to the top or bottom of the diff                    |
| `ctrl+d`, `ctrl+u`       | Move half a page down or up                            |
| `pgdn`, `pgup`/`b`       | Move a page down or up                                 |
| `f`, `F` (or `]f`, `[f`) | Next or previous file                                  |
| `h`, `H` (or `}`, `{`)   | Next or previous hunk                                  |
| `v`, `V`                 | Start or stop selecting a range of lines               |
| `c`                      | Comment on the current line, the range, or file header |
| `C`                      | Comment on the whole file                              |
| `dd`                     | Delete or resolve the comment under the cursor         |
| `DD`                     | Delete every comment on the branch                     |
| `r`                      | Toggle the file's viewed state                         |
| `S`                      | Send unsent comments to the agent                      |
| `y`                      | Copy unsent comments to the clipboard                  |
| `?`                      | Show every key                                         |
| `esc`                    | Stop selecting or close the help                       |
| `q`, `ctrl+c`            | Quit                                                   |

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
revunk reply <id> <text>        Reply to a review comment, used by the coding agent
```

`-base` sets the branch to compare against. It defaults to the remote's default branch (`origin/HEAD`), or `main` when there is none.

## Project layout

| Package                 | Responsibility                                                                                                                                  |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/revunk`            | CLI entry point and dependency wiring                                                                                                           |
| `internal/review`       | Review domain types, diff line parsing, the `Source` and `Store` boundaries, and the `Service` that all clients use. It has no UI dependencies. |
| `internal/provider/git` | Local git provider implementing `review.Source` for the commits on the current branch. Code hosts such as GitHub go in sibling packages.        |
| `internal/agent`        | Sends prompts to a Claude Code or opencode session in a `herdr` pane, or starts one, implementing `review.Agent`                                |
| `internal/store`        | JSON file store implementing `review.Store`                                                                                                     |
| `internal/tui`          | Bubble Tea presentation layer, with `chroma` for syntax highlighting and `glamour` for Markdown comments                                        |
