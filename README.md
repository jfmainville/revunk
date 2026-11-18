# Revunk

Revunk is a pull request review tool with a Bubble Tea terminal interface. It reviews the commits on the current git branch, such as changes made by a coding agent, as one continuous diff with inline comments, and manages your GitHub pull requests, from their details and conversation to a full code review. The review logic lives apart from the terminal UI so a future web or mobile client can reuse the same review service.

## Requirements

- Go 1.26 or later
- `git`
- [herdr](https://herdr.dev) with Claude Code or opencode, only to send comments with `S`
- The [GitHub CLI](https://cli.github.com), `gh`, signed in with `gh auth login`, only for the **Pull Request** tab

## Install

Each [GitHub release](https://github.com/jfmainville/revunk/releases) has `revunk` for Linux and macOS on `amd64` and `arm64`, as `.tar.gz` archives and `.deb` packages for Linux. Install a `.deb` with `sudo apt install ./revunk_<version>_linux_<arch>.deb`.

## Commands

| Task        | Command                                     |
| ----------- | ------------------------------------------- |
| Build       | `go build -o bin/revunk ./cmd/revunk`       |
| Run the TUI | `go run ./cmd/revunk` or `./bin/revunk tui` |
| Format      | `gofmt -w .`                                |
| Vet         | `go vet ./...`                              |
| Test        | `go test ./...`                             |

## Status bar

The bar under the panes shows the mode on the left and messages on the right, each after an icon, `✗` for an error, `⚠` for a warning, `✓` for a result, or a spinner while loading or sending. A message clears after 5 seconds, or at the next key, and a spinner stays until the work is done. On the Agent tab the bar also shows the branch, the search, and the comment and viewed counts.

## Agent tab

Run Revunk from inside a git repository. The **Agent** tab compares the current branch with its base branch, from their merge base to the working tree, so commits, staged and unstaged edits, and untracked files all show. Untracked files that are binary or larger than 1 MB are listed without a diff.

- **commits** lists `all changes` and each commit on the branch. Selecting a commit shows only that commit's diff. The pane takes at most 10 rows, or a third of the height when that is smaller, and scrolls to keep the selected commit in view.
- **files** lists the changed files grouped by directory, each with its added and removed line counts, and the totals in the pane title. Reviewed files move to a **Reviewed** section at the bottom, listed by full path. Marking a file reviewed from this pane selects the next file still to review. Selecting a file scrolls the diff to it, and moving through the diff keeps the file under the cursor selected, marked with a yellow bar.
- **continuous diff** shows every file one after another, each under a header with its status, line counts, and viewed state. Added and removed lines are tinted, each line shows its number in the current file, or in the old file for removed lines, and code is syntax highlighted. Like on GitHub, each removed line is paired in order with an added line of the same change, and the words that differ between them get a stronger tint, unless the lines share less than half their text. The diff shows as soon as it loads and its colors follow a moment later, since highlighting a large diff takes longer than reading it. Lines wider than the pane wrap onto the next rows without changing the file.
- Comments are drawn inline under the line, range, or file header they belong to. They support Markdown, and a ` ```suggestion ` block is shown as a suggested change against the commented lines.
- **comments**, hidden until `t` is pressed, lists every comment on the branch with its file, lines, commit, and state. `enter` jumps to the selected comment, switching to its commit view when needed, `x` resolves or reopens it, and `dd` deletes it. Resolved comments show a `✓` and the `resolved` state.
- A file marked viewed collapses and stays viewed until its diff changes. The reload notices the change, on the Agent tab and in a pull request's review diff, and expands the file again as not viewed.

Comments and viewed state are saved per branch in `.git/revunk.json`. When Revunk starts, it drops the comments and viewed state of branches that no longer exist locally, or whose upstream is gone after a `git fetch --prune`. The current branch, even a detached `HEAD`, is always kept.

`S` sends the comments not sent yet to the Claude Code or opencode session open in this repository, and `y` copies the same text to the clipboard. The text lists each comment with its ID, file, lines, commit, the commented code, and the replies so far, and asks the agent to answer each one with `revunk reply <id> "<text>"`. `S` uses `herdr agent list` to pick the first Claude Code or opencode pane whose directory is inside the repository, skipping the pane Revunk runs in. With no such pane, it starts Claude Code in a new pane next to Revunk. It submits the text with `herdr agent prompt` and marks the comments sent. Comments stay drafts when sending fails.

`gf` hands the pane to `$VISUAL`, else `$EDITOR`, else `nvim`, run as `<editor> +<line> <file>` from the repository root, and Revunk comes back and refreshes when the editor exits. On a removed line it opens the nearest line that still exists.

Revunk reloads commits, the diff, viewed state, and comments every 2 seconds, keeping the cursor on the same line. New commits and file edits therefore show up on their own, and agent replies appear under their comment, which moves from `draft` to `sent` to `replied`. Once the agent has replied, `r` opens an editor under the comment to answer it. Your reply shows as `↳ your reply` and turns the comment back into a `draft`, so the next `S` sends it again with the whole thread. A comment stays until you delete it with `dd` or `DD`. `x` resolves the comment under the cursor instead: it leaves the diff and is no longer sent to the agent, but stays in the comments pane, where `x` reopens it. Comments on a file that no longer exists in the working tree are deleted automatically, except comments on the removed lines or the whole file when the branch deletes that file. Replies work with any agent that can run shell commands, since the prompt gives the full path of the running `revunk` binary.

### Keys

| Key                              | Action                                                                       |
| -------------------------------- | ---------------------------------------------------------------------------- |
| `tab`, `shift+tab`               | Cycle focus between the visible panes                                        |
| `l`                              | Focus the diff                                                               |
| `ctrl+w` then `h`, `j`, `k`, `l` | Focus the pane to the left, below, above, or right, like vim windows         |
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
| `dd`                             | Delete the comment under the cursor                                          |
| `x`                              | Resolve or reopen the comment under the cursor                               |
| `r`                              | Reply to the agent's answer on the comment under the cursor                  |
| `t`                              | Show or hide the comments pane                                               |
| `e`                              | Show the files and commits panes and focus the files, or hide them           |
| `DD`                             | Delete every comment on the branch                                           |
| `-`                              | Toggle the file's viewed state                                               |
| `S`                              | Send unsent comments to the agent                                            |
| `y`                              | Copy unsent comments to the clipboard                                        |
| `gf`                             | Open the file at the cursor's line in `$VISUAL`, `$EDITOR`, or `nvim`        |
| `gd`                             | Review the branch's pull request on the **Pull Request** tab                 |
| `gu`                             | Review a pull request from its GitHub URL on the **Pull Request** tab        |
| `1`, `2`                         | Show the **Agent** or **Pull Request** tab, also by clicking its name        |
| `?`                              | Show every key, the bar at the bottom lists only the essential ones          |
| `esc`                            | Stop selecting, clear the search, or close the help, a modal, or a window    |
| `q`, `ctrl+c`                    | Quit                                                                         |

Clicking a comment in the comments pane jumps to it like `enter`. While mouse support is on, most terminals select text with `shift` held down.

### Comment editor

| Key                | Action                                                                                                                                                                                                                 |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ctrl+s`           | Save the comment                                                                                                                                                                                                       |
| `enter`            | Insert a newline                                                                                                                                                                                                       |
| `tab`, `shift+tab` | Cycle the type (comment, note, issue, suggestion), each drawn in its own color, with issues in red. Picking suggestion adds the commented lines as a ` ```suggestion ` block to edit, and leaving it removes the block |
| `esc`, `jj`        | Switch to normal mode, where `esc` cancels                                                                                                                                                                             |

The comment text works like vim, on the **Agent** tab and in the pull request review diff, and its Markdown is highlighted as you type: headings, lists, bold, italic, code, links, and quotes. It starts in insert mode, `esc` or `jj` switches to normal mode, where `h` `j` `k` `l`, `w` `b`, `0` `$`, `gg` `G`, `ctrl+d` `ctrl+u`, `i` `a` `I` `A` `o` `O`, `x`, and `dd` work, and `esc` in normal mode cancels. A text longer than its box scrolls to follow the cursor. The cursor is a bar in insert mode and a block in normal mode, in every comment box and modal, and the one line fields of the modals, always typing, show a bar.

Every modal and editor saves with `ctrl+s` only and cancels or closes with `esc` only, so `q` and `enter` are never a shortcut for either.

## Pull Request tab

Press `2` to open the **Pull Request** tab, and `1` to go back to the **Agent** tab. It lists the open pull requests assigned to you on GitHub, across every repository, loaded in the background once the branch loads so the tab opens with them, and `s` switches the filter to the ones where your review is requested, the ones you authored, and the ones that involve you. Each pull request takes two lines. The first shows a mark for its latest checks, `✓` passed, `●` running, `✗` failed, or `◌` for a draft, then its number, its title, cut with `…` when it doesn't fit, its added and removed lines, its author, and how long ago it was opened and last updated, each in a column under a header so the rows line up. The second line shows the repository. The list takes 60% of the width and the details sidebar the rest.

The sidebar on the right shows the selected pull request, and `enter` focuses it. `tab` and `shift+tab` move between its tabs:

- **Overview** shows the title, the state in an accent chip, line counts, then assignees, reviewers with their review state, labels, the checks of the latest commit, and the description rendered as Markdown, each separated by a blank line.
- **Activity** lists the comments, replies, and reviews in the order they happened.
- **Comments** shows the conversation, the reviews with a message, and the review threads on code, with their replies and reactions. `j` and `k` or a click select a comment, marked with its author in the accent color while the sidebar has focus, for `r` to reply or `+` to react. New comments are added from the actions modal.
- **Commits** and **Files** list the pull request's commits and changed files.

Every change is sent to GitHub through `gh`, then the pull request reloads. The list and the selected pull request also reload every 10 seconds while the tab is open, so new checks, comments, and reviews show up on their own.

`gd` opens the pull request's diff in the same continuous diff as the **Agent** tab, with the commits and files panes, search, viewed state, and comments. Comments there are GitHub review comments: `c`, `C`, and a ` ```suggestion ` block add them to your pending review, they show with their author, replies, and `pending`, `resolved`, or published state, and `dd` deletes one. The review diff reloads every 15 seconds, and `esc` closes it once nothing is selected or searched. Viewed files are saved locally, like on the **Agent** tab. A note or an issue is posted as a regular comment since GitHub has no such types.

`A` opens the actions of the selected pull request, also from the review diff. With no pull request selected it lists only the ones that need none. It opens in its search, where typing filters them and `↓` `↑` or `ctrl+n` `ctrl+p` move. `esc` leaves the search for the list, where `j` `k` move and `/` or `i` goes back to the search, and `esc` there closes it. `enter` runs the selected action. Each action has a one cell icon in the accent color:

| Action                               | Does                                                                                                                                                                                                       |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ✎ Edit the title and description     | Same as `ge`                                                                                                                                                                                               |
| ± Review the diff                    | Same as `gd`, not listed in the review diff                                                                                                                                                                |
| ↗ Review a pull request from its URL | Same as `gu`, not listed in the review diff                                                                                                                                                                |
| + Create a pull request              | Same as `gc`. Pick the branch, the current one by default, and the target branch, the default branch by default, then write the title and description in the same boxes as `ge`. The branch must be pushed |
| ❝ Comment                            | Comment on the pull request                                                                                                                                                                                |
| ✓ Submit a review                    | Comment, approve, or request changes, picked in a dropdown that `enter` opens. Like on GitHub, a comment or request for changes without a message needs inline comments in the review                      |
| ⎇ Merge                              | Merge after confirming the method and the commit message                                                                                                                                                   |
| ✗ Close without merging              | Close after confirming, with an optional comment                                                                                                                                                           |
| ◉ Assignees, ◎ Reviewers, ◆ Labels   | Pick in a searchable list of the repository's users or labels                                                                                                                                              |
| ✦ Request a review from Copilot      | Ask Copilot for a review, like `gh pr edit --add-reviewer @copilot`                                                                                                                                        |

`ge` opens a large modal with two vim boxes, a one line `title` and the Markdown `description` under it, and `tab` moves between them, keeping the vim mode. It starts on the title in normal mode. An `into` dropdown above it changes the target branch, starting on the current one, and `shift+tab` reaches it. The branches load in the background, so the modal opens right away.

Merging asks to confirm in a modal that lists only the merge methods the repository allows, with the commit message GitHub would use in the same title and description boxes as `ge`, starting in normal mode. The description box is smaller than the one of `ge` and scrolls when the message is longer. A rebase keeps each commit's message, so it has none. The modal warns when the branch conflicts with its base, is behind it, has failing checks, or is blocked by branch protection. GitHub still decides, so a blocked merge, missing permission, or a branch that got new commits since it loaded shows GitHub's reason in the status line and leaves the pull request open. A successful merge reloads the pull request to show its merged state. Draft, closed, and merged pull requests don't open the modal.

`gu`, on either tab, opens a modal that takes a pull request URL such as `https://github.com/owner/repo/pull/123` and opens its review diff, so a pull request outside the list needs no searching. `ctrl+s` opens it. A URL that is not a `github.com` pull request keeps the modal open with the reason, and a repository or pull request you can't reach, or a `gh` sign in problem, shows the `gh` error in the status line.

| Key                    | Action                                                                      |
| ---------------------- | --------------------------------------------------------------------------- |
| `j`/`↓`, `k`/`↑`       | Move through the list, or the selected comment or the text in the sidebar   |
| `gg`, `G`              | Go to the top or bottom                                                     |
| `ctrl+d`, `ctrl+u`     | Scroll the sidebar half a page                                              |
| `enter`, `esc`         | Focus the sidebar, or go back to the list                                   |
| `ctrl+w` then `l`, `h` | Focus the sidebar or the list                                               |
| `tab`, `shift+tab`     | Next or previous sidebar tab                                                |
| `s`                    | Next filter: assigned, review requested, authored, involved                 |
| `A`                    | Search and run the pull request's actions, also from the review diff        |
| `ge`                   | Edit the title and the Markdown description, also from the review diff      |
| `r`                    | Reply to the selected comment, quoting it when it is not in a review thread |
| `+`                    | React to the selected comment, picking a reaction you gave removes it       |
| `x`                    | Resolve or reopen the selected review thread                                |
| `gd`                   | Review the diff with GitHub review comments                                 |
| `gu`                   | Review the diff of a pull request from its GitHub URL                       |
| `gc`                   | Create a pull request                                                       |
| Click, mouse wheel     | Select a pull request or sidebar tab, or focus and scroll the sidebar       |

The modals save with `ctrl+s` and cancel with `esc`, like the comment editor. Pasting works in every modal field and in the comment editor. `tab` moves between their fields, and `j` and `k` pick in a list. Their Markdown text works like vim, the same as the comment editor.

The assignee and reviewer pickers list the users who can be assigned in the repository, and the label picker its labels, with the current ones selected first. Like the actions, they open in their search, and `esc` leaves it for the list where `j` `k` move. `space` selects or clears, and `ctrl+s` saves. The author is left out of the reviewers.

## CLI

```
revunk [-base <branch>] [tui]   Launch the terminal review interface (default)
revunk -cleanup                 Delete .git/revunk.json and exit
revunk -version                 Print the version and exit
```

Coding agents answer comments with `revunk reply <id> <text>`, which the prompt sent by `S` explains, so it is left out of `--help`.

`-base` sets the branch to compare against. It defaults to the remote's default branch (`origin/HEAD`), or `main` when there is none.

`-cleanup` deletes `.git/revunk.json`, removing every saved comment and viewed file for all branches and pull requests, then exits without opening the interface.

`-version` prints the release version, such as `revunk 1.4.0`, or `revunk dev` for a binary built from source.

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
