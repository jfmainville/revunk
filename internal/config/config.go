package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds the settings of config.toml. An empty Base means the default branch.
type Config struct {
	Base   string              `toml:"base"`
	Leader string              `toml:"leader"`
	Keys   map[string][]string `toml:"keys"`
}

// Path returns the config file under $XDG_CONFIG_HOME, or under ~/.config when that is unset or relative.
func Path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(dir) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "revunk", "config.toml"), nil
}

// Load reads the config at path. A missing file gives the defaults.
func Load(path string) (Config, error) {
	cfg := Config{Leader: "space"}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("%s: unknown setting %s", path, undecoded[0])
	}
	if md.IsDefined("base") && strings.TrimSpace(cfg.Base) == "" {
		return cfg, fmt.Errorf("%s: base must not be empty", path)
	}
	if strings.TrimSpace(cfg.Leader) == "" {
		return cfg, fmt.Errorf("%s: leader must not be empty", path)
	}
	for name, keys := range cfg.Keys {
		if slices.Contains(keys, "") {
			return cfg, fmt.Errorf("%s: keys.%s has an empty key", path, name)
		}
	}
	return cfg, nil
}
