package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Base   string              `toml:"base"`
	Leader string              `toml:"leader"`
	Theme  string              `toml:"theme"`
	Agent  string              `toml:"agent"`
	Keys   map[string][]string `toml:"keys"`
}

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

func Load(path string) (Config, error) {
	cfg := Config{Leader: "space", Theme: "catppuccin"}
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

var agentLine = regexp.MustCompile(`^\s*agent\s*=`)

// SetAgent saves the default agent by rewriting only its line, so the rest of the file keeps its comments.
func SetAgent(path, name string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var setting bytes.Buffer
	if err := toml.NewEncoder(&setting).Encode(map[string]string{"agent": name}); err != nil {
		return err
	}
	line := strings.TrimSuffix(setting.String(), "\n")
	lines := strings.Split(string(data), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return agentLine.MatchString(l) || strings.HasPrefix(strings.TrimSpace(l), "[") })
	if i >= 0 && agentLine.MatchString(lines[i]) {
		lines[i] = line
	} else {
		lines = slices.Insert(lines, 0, line)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
