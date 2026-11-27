# Configuration

Revunk reads `config.toml` when it starts. The file lives under `$XDG_CONFIG_HOME/revunk/`, or under `~/.config/revunk/` when `XDG_CONFIG_HOME` is unset or relative. A missing file keeps the defaults.

## Precedence

A flag wins over the config file, and the config file wins over the built-in default. `-base` takes the config `base`, and `-leader` takes the config `leader`.

## Defaults

The file below sets every parameter to its default value, so it works as a reference. Copy it to your config file and change only the lines you need.

```toml
# Branch to compare against, the same as -base. Unset by default, which uses origin's default branch, else main.
# base = "main"

# Key that starts leader sequences, the same as -leader.
leader = "space"

# Key bindings, each set to its default keys. An empty list disables a binding.
[keys]
quit = ["q"]
force-quit = ["ctrl+c"]
help = ["?"]
next-focus = ["tab"]
prev-focus = ["shift+tab"]
focus-diff = ["l"]
down = ["j", "down"]
up = ["k", "up"]
top = ["g g"]
bottom = ["G"]
half-down = ["ctrl+d"]
half-up = ["ctrl+u"]
page-down = ["pgdown"]
page-up = ["pgup", "b"]
next-file = ["f", "] f"]
prev-file = ["F", "[ f"]
next-change = ["h", "}"]
prev-change = ["H", "{"]
open = ["enter"]
select = ["v", "V"]
cancel = ["esc"]
comment = ["c"]
file-comment = ["C"]
delete = ["d d"]
delete-all = ["D D"]
viewed = ["-"]
fold = ["z a"]
comments = ["t"]
edit = ["e"]
explorer = ["space e"]
send = ["S"]
copy = ["y"]
save = ["ctrl+s"]
newline = ["enter"]
next-kind = ["tab"]
prev-kind = ["shift+tab"]
search = ["/"]
next-match = ["n", "ctrl+n"]
prev-match = ["N", "ctrl+p"]
delete-char = ["backspace"]
open-file = ["g f"]
agent-tab = ["1"]
pulls-tab = ["2"]
next-filter = ["s"]
edit-pr = ["g e"]
open-diff = ["g d"]
open-url = ["g u"]
create-pr = ["g c"]
actions = ["A"]
commit-info = ["K"]
pick-down = ["down", "ctrl+n"]
pick-up = ["up", "ctrl+p"]
toggle = ["space"]
reply = ["r"]
react = ["+"]
resolve = ["x"]
normal-mode = ["esc"]
leave-insert = ["j"]
insert = ["i"]
append = ["a"]
insert-start = ["I"]
append-end = ["A"]
open-below = ["o"]
open-above = ["O"]
char-left = ["h", "left"]
char-right = ["l", "right"]
word-next = ["w"]
word-prev = ["b"]
line-start = ["0"]
line-end = ["$"]
delete-under = ["x"]
"ops.inner-change" = ["c i w"]
"ops.word-change" = ["c w"]
"ops.inner-delete" = ["d i w"]
"ops.word-delete" = ["d w"]
"ops.change-to-end" = ["c $"]
"ops.delete-to-end" = ["d $"]
window-left = ["ctrl+w h"]
window-down = ["ctrl+w j"]
window-up = ["ctrl+w k"]
window-right = ["ctrl+w l"]
```

## Parameters

Each row is one setting. `Keybinding` is the field name in `keyMap` in `internal/tui/keys.go`, and `-` marks a setting that is not a key binding.

| Parameter                | Keybinding        | Default                      | Description                                                                                                          |
| ------------------------ | ----------------- | ---------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `base`                   | -                 | Unset, so the default branch | Branch to compare against, the same as `-base`.                                                                      |
| `leader`                 | -                 | `space`                      | Key that starts leader sequences, the same as `-leader`.                                                             |
| `keys.quit`              | `Quit`            | `q`                          | Quit Revunk.                                                                                                         |
| `keys.force-quit`        | `ForceQuit`       | `ctrl+c`                     | Quit Revunk immediately.                                                                                             |
| `keys.help`              | `Help`            | `?`                          | Show the help screen.                                                                                                |
| `keys.next-focus`        | `NextFocus`       | `tab`                        | Cycle focus to the next pane or tab.                                                                                 |
| `keys.prev-focus`        | `PrevFocus`       | `shift+tab`                  | Cycle focus to the previous pane or tab.                                                                             |
| `keys.focus-diff`        | `FocusDiff`       | `l`                          | Focus the diff pane.                                                                                                 |
| `keys.down`              | `Down`            | `j`, `down`                  | Move down one line or item.                                                                                          |
| `keys.up`                | `Up`              | `k`, `up`                    | Move up one line or item.                                                                                            |
| `keys.top`               | `Top`             | `g g`                        | Go to the top of the pane.                                                                                           |
| `keys.bottom`            | `Bottom`          | `G`                          | Go to the bottom of the pane.                                                                                        |
| `keys.half-down`         | `HalfDown`        | `ctrl+d`                     | Scroll down half a page.                                                                                             |
| `keys.half-up`           | `HalfUp`          | `ctrl+u`                     | Scroll up half a page.                                                                                               |
| `keys.page-down`         | `PageDown`        | `pgdown`                     | Scroll down a page.                                                                                                  |
| `keys.page-up`           | `PageUp`          | `pgup`, `b`                  | Scroll up a page.                                                                                                    |
| `keys.next-file`         | `NextFile`        | `f`, `] f`                   | Go to the next file in the diff.                                                                                     |
| `keys.prev-file`         | `PrevFile`        | `F`, `[ f`                   | Go to the previous file in the diff.                                                                                 |
| `keys.next-change`       | `NextChange`      | `h`, `}`                     | Go to the next block of changes.                                                                                     |
| `keys.prev-change`       | `PrevChange`      | `H`, `{`                     | Go to the previous block of changes.                                                                                 |
| `keys.open`              | `Open`            | `enter`                      | Open the selected item.                                                                                              |
| `keys.select`            | `Select`          | `v`, `V`                     | Start a visual range selection for a comment.                                                                        |
| `keys.cancel`            | `Cancel`          | `esc`                        | Cancel the current selection, search, or view.                                                                       |
| `keys.comment`           | `Comment`         | `c`                          | Comment on the line, range, or file header under the cursor.                                                         |
| `keys.file-comment`      | `FileComment`     | `C`                          | Comment on the whole file.                                                                                           |
| `keys.delete`            | `Delete`          | `d d`                        | Delete the comment under the cursor.                                                                                 |
| `keys.delete-all`        | `DeleteAll`       | `D D`                        | Delete every comment.                                                                                                |
| `keys.viewed`            | `Viewed`          | `-`                          | Mark the file as viewed until it changes.                                                                            |
| `keys.fold`              | `Fold`            | `z a`                        | Expand or collapse the file under the cursor.                                                                        |
| `keys.comments`          | `Comments`        | `t`                          | Show or hide the comments pane.                                                                                      |
| `keys.edit`              | `Edit`            | `e`                          | Edit the comment under the cursor, when it is yours.                                                                 |
| `keys.explorer`          | `Explorer`        | `space e`                    | Show or hide the commits and files panes. Its default follows `leader`, so change `leader` rather than this setting. |
| `keys.send`              | `Send`            | `S`                          | Send unsent comments to the agent.                                                                                   |
| `keys.copy`              | `Copy`            | `y`                          | Copy unsent comments to the clipboard.                                                                               |
| `keys.save`              | `Save`            | `ctrl+s`                     | Save in the comment editor or a modal.                                                                               |
| `keys.newline`           | `Newline`         | `enter`                      | Insert a newline in the comment editor.                                                                              |
| `keys.next-kind`         | `NextKind`        | `tab`                        | Cycle the comment type in the editor.                                                                                |
| `keys.prev-kind`         | `PrevKind`        | `shift+tab`                  | Cycle the comment type backwards in the editor.                                                                      |
| `keys.search`            | `Search`          | `/`                          | Search every file in the diff.                                                                                       |
| `keys.next-match`        | `NextMatch`       | `n`, `ctrl+n`                | Go to the next search match.                                                                                         |
| `keys.prev-match`        | `PrevMatch`       | `N`, `ctrl+p`                | Go to the previous search match.                                                                                     |
| `keys.delete-char`       | `DeleteChar`      | `backspace`                  | Delete the character before the cursor in a text field.                                                              |
| `keys.open-file`         | `OpenFile`        | `g f`                        | Open the file at the cursor in `$EDITOR`.                                                                            |
| `keys.agent-tab`         | `AgentTab`        | `1`                          | Switch to the Agent tab.                                                                                             |
| `keys.pulls-tab`         | `PullsTab`        | `2`                          | Switch to the Pull Request tab.                                                                                      |
| `keys.next-filter`       | `NextFilter`      | `s`                          | Cycle the pull request filter.                                                                                       |
| `keys.edit-pr`           | `EditPR`          | `g e`                        | Edit the pull request title and description, or the reviewed pull request in a review diff.                          |
| `keys.open-diff`         | `OpenDiff`        | `g d`                        | Review the diff of the branch's pull request.                                                                        |
| `keys.open-url`          | `OpenURL`         | `g u`                        | Review a pull request from its GitHub URL.                                                                           |
| `keys.create-pr`         | `CreatePR`        | `g c`                        | Create a pull request.                                                                                               |
| `keys.actions`           | `Actions`         | `A`                          | Search the pull request actions.                                                                                     |
| `keys.commit-info`       | `CommitInfo`      | `K`                          | Show the details of the selected commit.                                                                             |
| `keys.pick-down`         | `PickDown`        | `down`, `ctrl+n`             | Move down in a picker or the actions list.                                                                           |
| `keys.pick-up`           | `PickUp`          | `up`, `ctrl+p`               | Move up in a picker or the actions list.                                                                             |
| `keys.toggle`            | `Toggle`          | `space`                      | Select or clear the item in a picker.                                                                                |
| `keys.reply`             | `Reply`           | `r`                          | Reply to the selected comment.                                                                                       |
| `keys.react`             | `React`           | `+`                          | React to the selected comment.                                                                                       |
| `keys.resolve`           | `Resolve`         | `x`                          | Resolve or reopen the selected comment thread.                                                                       |
| `keys.normal-mode`       | `NormalMode`      | `esc`                        | Leave insert mode for normal mode in vim-style text.                                                                 |
| `keys.leave-insert`      | `LeaveInsert`     | `j`                          | Leave insert mode when pressed twice in a row, since one press is typed.                                             |
| `keys.insert`            | `Insert`          | `i`                          | Insert before the cursor.                                                                                            |
| `keys.append`            | `Append`          | `a`                          | Insert after the cursor.                                                                                             |
| `keys.insert-start`      | `InsertStart`     | `I`                          | Insert at the start of the line.                                                                                     |
| `keys.append-end`        | `AppendEnd`       | `A`                          | Insert at the end of the line.                                                                                       |
| `keys.open-below`        | `OpenBelow`       | `o`                          | Open a new line below and insert.                                                                                    |
| `keys.open-above`        | `OpenAbove`       | `O`                          | Open a new line above and insert.                                                                                    |
| `keys.char-left`         | `CharLeft`        | `h`, `left`                  | Move the cursor one character left.                                                                                  |
| `keys.char-right`        | `CharRight`       | `l`, `right`                 | Move the cursor one character right.                                                                                 |
| `keys.word-next`         | `WordNext`        | `w`                          | Move to the start of the next word.                                                                                  |
| `keys.word-prev`         | `WordPrev`        | `b`                          | Move to the start of the previous word.                                                                              |
| `keys.line-start`        | `LineStart`       | `0`                          | Move to the start of the line.                                                                                       |
| `keys.line-end`          | `LineEnd`         | `$`                          | Move to the end of the line.                                                                                         |
| `keys.delete-under`      | `DeleteUnder`     | `x`                          | Delete the character under the cursor.                                                                               |
| `keys.ops.inner-change`  | `Ops.InnerChange` | `c i w`                      | Change the word under the cursor.                                                                                    |
| `keys.ops.word-change`   | `Ops.WordChange`  | `c w`                        | Change up to the next word.                                                                                          |
| `keys.ops.inner-delete`  | `Ops.InnerDelete` | `d i w`                      | Delete the word under the cursor.                                                                                    |
| `keys.ops.word-delete`   | `Ops.WordDelete`  | `d w`                        | Delete up to the next word.                                                                                          |
| `keys.ops.change-to-end` | `Ops.ChangeToEnd` | `c $`                        | Change up to the end of the line.                                                                                    |
| `keys.ops.delete-to-end` | `Ops.DeleteToEnd` | `d $`                        | Delete up to the end of the line.                                                                                    |
| `keys.window-left`       | `WindowLeft`      | `ctrl+w h`                   | Focus the pane to the left.                                                                                          |
| `keys.window-down`       | `WindowDown`      | `ctrl+w j`                   | Focus the pane below.                                                                                                |
| `keys.window-up`         | `WindowUp`        | `ctrl+w k`                   | Focus the pane above.                                                                                                |
| `keys.window-right`      | `WindowRight`     | `ctrl+w l`                   | Focus the pane to the right.                                                                                         |

## Key bindings

Each key of `[keys]` names a key binding. Its name is the kebab-case form of the field name, so `NextFocus` becomes `next-focus` and `OpenURL` becomes `open-url`. Operator bindings sit under `ops`, such as `ops.inner-change`, and the dot needs quotes.

The keys of a binding replace its defaults, so `quit = ["q", "ctrl+q"]` keeps `q` and adds `ctrl+q`. An empty list disables the binding, and the help screen and footer hide it.

## Errors

Revunk stops with an error naming the file when the config is invalid. This covers a syntax error, an unknown setting, a value of the wrong type, an empty `base` or `leader`, and an empty key string. `revunk` and `revunk reply` both check the file. An unknown key binding name stops `revunk` when the interface starts, and `revunk reply` does not apply key bindings.

## Compatibility

Without a config file, Revunk behaves as before. Nothing has to migrate, and every existing flag keeps working.
