# Revunk

Revunk is a pull request review tool with a Bubble Tea terminal interface. It reviews the commits on the current git branch, such as changes made by a coding agent, as one continuous diff with inline comments, and manages your GitHub pull requests, from their details and conversation to a full code review. The review logic lives apart from the terminal UI so a future web or mobile client can reuse the same review service.

## Requirements

- Go 1.26 or later
- `git`
- [herdr](https://herdr.dev) with Claude Code or opencode, only to send comments with `S`
- The [GitHub CLI](https://cli.github.com), `gh`, signed in with `gh auth login`, only for the **Pull Request** tab

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
- **files** lists the changed files grouped by directory, each with its added and removed line counts, and the totals in the pane title. Reviewed files move to a **Reviewed** section at the bottom, listed by full path. Marking a file reviewed from this pane selects the next file still to review. Selecting a file scrolls the diff to it, and moving through the diff keeps the file under the cursor selected, marked with a yellow bar.
- **continuous diff** shows every file one after another, each under a header with its status, line counts, and viewed state. Added and removed lines are tinted, each line shows its number in the current file, or in the old file for removed lines, and code is syntax highlighted. Lines wider than the pane wrap onto the next rows without changing the file.
- Comments are drawn inline under the line, range, or file header they belong to. They support Markdown, and a ` ```suggestion ` block is shown as a suggested change against the commented lines.
- **comments**, hidden until `t` is pressed, lists every comment on the branch with its file, lines, commit, and state. `enter` jumps to the selected comment, switching to its commit view when needed, and `dd` deletes it.
- A file marked viewed collapses and stays viewed until its diff changes.

Comments and viewed state are saved per branch in `.git/revunk.json`.

`S` sends the comments not sent yet to the Claude Code or opencode session open in this repository, and `y` copies the same text to the clipboard. The text lists each comment with its ID, file, lines, commit, and the commented code, and asks the agent to answer each one with `revunk reply <id> "<text>"`. `S` uses `herdr agent list` to pick the first Claude Code or opencode pane whose directory is inside the repository, skipping the pane Revunk runs in. With no such pane, it starts Claude Code in a new pane next to Revunk. It submits the text with `herdr agent prompt` and marks the comments sent. Comments stay drafts when sending fails.

`gf` hands the pane to `$VISUAL`, else `$EDITOR`, else `nvim`, run as `<editor> +<line> <file>` from the repository root, and Revunk comes back and refreshes when the editor exits. On a removed line it opens the nearest line that still exists.

Revunk reloads commits, the diff, viewed state, and comments every 2 seconds, keeping the cursor on the same line. New commits and file edits therefore show up on their own, and agent replies appear under their comment, which moves from `draft` to `sent` to `replied`. A comment stays until you resolve it with `dd` or `DD`. Comments on a file that no longer exists in the working tree are deleted automatically, except comments on the removed lines or the whole file when the branch deletes that file. Replies work with any agent that can run shell commands, since the prompt gives the full path of the running `revunk` binary.

### Keys

| Key                              | Action                                                                       |
| -------------------------------- | ---------------------------------------------------------------------------- |
| `tab`, `shift+tab`               | Cycle focus between the visible panes                                        |
| `l`                              | Focus the diff                                                               |
| Click                            | Focus a pane and select the commit, file, line, or comment under the pointer |
| Mouse wheel                      | Focus and scroll the pane under the pointer                                  |
| `j`/`↓`, `k`/`↑`                 | Move down or up                                                              |
| `gg`, `G`                        | Go to the top or bottom of the diff                                          |
| `ctrl+d`, `ctrl+u`               | Move half a page down or up in the diff, focusing it                         |
| `pgdn`, `pgup`/`b`               | Move a page down or up in the diff, focusing it                              |
| `f`, `F` (or `]f`, `[f`)         | Next or previous file                                                        |
| `h`, `H` (or `}`, `{`)           | Next or previous block of added or removed lines                             |
| `/`                              | Search every file in the diff, showing the current match and the total       |
| `n`, `N` (or `ctrl+n`, `ctrl+p`) | Next or previous match, `ctrl+n` and `ctrl+p` also work while typing         |
| `v`, `V`                         | Start or stop selecting a range of lines                                     |
| `c`                              | Comment on the current line, the range, or file header                       |
| `C`                              | Comment on the whole file                                                    |
| `dd`                             | Delete or resolve the comment under the cursor                               |
| `t`                              | Show or hide the comments pane                                               |
| `e`                              | Show or hide the commits and files panes                                     |
| `DD`                             | Delete every comment on the branch                                           |
| `-`                              | Toggle the file's viewed state                                               |
| `S`                              | Send unsent comments to the agent                                            |
| `y`                              | Copy unsent comments to the clipboard                                        |
| `gf`                             | Open the file at the cursor's line in `$VISUAL`, `$EDITOR`, or `nvim`        |
| `ge`                             | Edit the title and description of the branch's pull request                  |
| `gd`                             | Review the branch's pull request on the **Pull Request** tab                 |
| `1`, `2`                         | Show the **Agent** or **Pull Request** tab, also by clicking its name        |
| `?`                              | Show every key                                                               |
| `esc`                            | Stop selecting, clear the search, or close the help                          |
| `q`, `ctrl+c`                    | Quit                                                                         |

Clicking a comment in the comments pane jumps to it like `enter`. While mouse support is on, most terminals select text with `shift` held down.

### Comment editor

| Key                | Action                                                                                                                                                                                      |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ctrl+s`           | Save the comment                                                                                                                                                                            |
| `enter`            | Insert a newline                                                                                                                                                                            |
| `tab`, `shift+tab` | Cycle the type (comment, note, suggestion), each drawn in its own color. Picking suggestion adds the commented lines as a ` ```suggestion ` block to edit, and leaving it removes the block |
| `esc`, `jj`        | Switch to normal mode, where `esc` cancels                                                                                                                                                  |

The comment text works like vim, on the **Agent** tab and in the pull request review diff. It starts in insert mode, `esc` or `jj` switches to normal mode, where `h` `j` `k` `l`, `w` `b`, `0` `$`, `gg` `G`, `i` `a` `I` `A` `o` `O`, `x`, and `dd` work, and `esc` in normal mode cancels.

## Pull Request tab

Press `2` to open the **Pull Request** tab, and `1` to go back to the **Agent** tab. It lists the open pull requests assigned to you on GitHub, across every repository, and `s` switches the filter to the ones where your review is requested, the ones you authored, and the ones that involve you. Each row shows the pull request's number, title, repository, and last update, with a mark for its latest checks, `✓` passed, `●` running, `✗` failed, or `◌` for a draft.

The sidebar on the right shows the selected pull request, and `enter` focuses it. `tab` and `shift+tab` move between its tabs:

- **Overview** shows the title, state, branches, line counts, assignees, reviewers with their review state, labels, the checks of the latest commit, and the description rendered as Markdown.
- **Activity** lists the comments, replies, and reviews in the order they happened.
- **Comments** shows the conversation, the reviews with a message, and the review threads on code, with their replies and reactions. `j` and `k` select a comment.
- **Commits** and **Files** list the pull request's commits and changed files.

Every change is sent to GitHub through `gh`, then the pull request reloads. The list and the selected pull request also reload every 10 seconds while the tab is open, so new checks, comments, and reviews show up on their own.

`gd` opens the pull request's diff in the same continuous diff as the **Agent** tab, with the commits and files panes, search, viewed state, and comments. Comments there are GitHub review comments: `c`, `C`, and a ` ```suggestion ` block add them to your pending review, they show with their author, replies, and `pending`, `resolved`, or published state, and `dd` deletes one. `gs` submits the review, picking comment, approve, or request changes in a dropdown that `enter` opens, with an optional Markdown message. Like on GitHub, a comment or request for changes without a message needs inline comments in the review. The review diff reloads every 15 seconds, and `q` closes it. Viewed files are saved locally, like on the **Agent** tab. A note is posted as a regular comment since GitHub has no notes.

| Key                | Action                                                                      |
| ------------------ | --------------------------------------------------------------------------- |
| `j`/`↓`, `k`/`↑`   | Move through the list, or the selected comment or the text in the sidebar   |
| `gg`, `G`          | Go to the top or bottom                                                     |
| `ctrl+d`, `ctrl+u` | Scroll the sidebar half a page                                              |
| `enter`, `esc`     | Focus the sidebar, or go back to the list                                   |
| `tab`, `shift+tab` | Next or previous sidebar tab                                                |
| `s`                | Next filter: assigned, review requested, authored, involved                 |
| `ge`               | Edit the title and the Markdown description                                 |
| `A`, `R`           | Pick the assignees or reviewers in a searchable list                        |
| `c`                | Comment on the pull request                                                 |
| `r`                | Reply to the selected comment, quoting it when it is not in a review thread |
| `+`                | React to the selected comment, picking a reaction you gave removes it       |
| `x`                | Resolve or reopen the selected review thread                                |
| `gd`               | Review the diff with GitHub review comments                                 |
| `gs`               | Submit the review from the review diff                                      |
| Click, mouse wheel | Select a pull request or sidebar tab, or focus and scroll the sidebar       |

The modals for editing, commenting, and reviewing save with `ctrl+s` and cancel with `esc`, like the comment editor. `tab` moves between their fields, and `j` and `k` pick in a list. Their Markdown text works like vim, the same as the comment editor.

The assignee and reviewer pickers list the users who can be assigned in the repository, with the current ones selected first. Typing searches them, `↓` `↑` or `ctrl+n` `ctrl+p` move, `space` selects or clears, and `ctrl+s` saves. The author is left out of the reviewers.

## CLI

```
revunk [-base <branch>] [tui]   Launch the terminal review interface (default)
```

Coding agents answer comments with `revunk reply <id> <text>`, which the prompt sent by `S` explains, so it is left out of `--help`.

`-base` sets the branch to compare against. It defaults to the remote's default branch (`origin/HEAD`), or `main` when there is none.

## Project layout

| Package                    | Responsibility                                                                                                                                                                     |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/revunk`               | CLI entry point and dependency wiring                                                                                                                                              |
| `internal/review`          | Review and pull request domain types, diff line parsing, the `Source`, `Store`, `Host`, and `Agent` boundaries, and the `Service` that all clients use. It has no UI dependencies. |
| `internal/provider/git`    | Local git provider implementing `review.Source` for the commits on the current branch                                                                                              |
| `internal/provider/github` | GitHub provider through `gh`, implementing `review.Host` for pull requests, and `review.Source` and `review.Store` for a pull request's review diff                                |
| `internal/agent`           | Sends prompts to a Claude Code or opencode session in a `herdr` pane, or starts one, implementing `review.Agent`                                                                   |
| `internal/store`           | JSON file store implementing `review.Store`                                                                                                                                        |
| `internal/tui`             | Bubble Tea presentation layer, with `chroma` for syntax highlighting, `glamour` for Markdown comments, and every default key binding in `keys.go`                                  |

[`docs/pull-requests.md`](docs/pull-requests.md) explains how the **Pull Request** tab works with GitHub.
