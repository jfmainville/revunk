# Configuration

Revunk reads `config.toml` when it starts. The file lives under `$XDG_CONFIG_HOME/revunk/`, or under `~/.config/revunk/` when `XDG_CONFIG_HOME` is unset or relative. A missing file keeps the defaults.

## Precedence

A flag wins over the config file, and the config file wins over the built-in default. `-base` takes the config `base`, and `-leader` takes the config `leader`.

## Sample

```toml
# Branch to compare against, the same as -base.
base = "main"

# Key that starts leader sequences, the same as -leader.
leader = "space"

[keys]
# Replace the default keys of an action. An empty list disables it.
# quit = ["q", "ctrl+q"]
# explorer = ["ctrl+e"]
# Operators are nested under ops, and the dot needs quotes.
# "ops.inner-change" = ["c i p"]
```

## Key bindings

Each key of `[keys]` names a key binding. Its name is the field name in `keyMap` in `internal/tui/keys.go`, in kebab case, so `NextFocus` becomes `next-focus` and `OpenURL` becomes `open-url`. Operator bindings sit under `ops`, such as `ops.inner-change`.

The keys of a binding replace its defaults, so `quit = ["q", "ctrl+q"]` keeps `q` and adds `ctrl+q`. The help screen and footer show the configured keys.

## Errors

Revunk stops with an error naming the file when the config is invalid. This covers a syntax error, an unknown setting, a value of the wrong type, an empty `base` or `leader`, and an empty key string. `revunk` and `revunk reply` both check the file. An unknown key binding name stops `revunk` when the interface starts, and `revunk reply` does not apply key bindings.

## Compatibility

Without a config file, Revunk behaves as before. Nothing has to migrate, and every existing flag keeps working.
