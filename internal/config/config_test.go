package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, err := Path()
	if err != nil || got != filepath.Join("/xdg", "revunk", "config.toml") {
		t.Fatalf("Path() = %q, %v", got, err)
	}
}

func TestPathFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, xdg := range []string{"", "relative/path"} {
		t.Setenv("XDG_CONFIG_HOME", xdg)
		got, err := Path()
		if want := filepath.Join(home, ".config", "revunk", "config.toml"); err != nil || got != want {
			t.Errorf("XDG_CONFIG_HOME=%q: Path() = %q, %v, want %q", xdg, got, err, want)
		}
	}
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Leader != "space" || cfg.Theme != "catppuccin" || cfg.Base != "" || cfg.Keys != nil {
		t.Fatalf("Load() = %+v, want defaults", cfg)
	}
}

func TestLoadReadsSettings(t *testing.T) {
	path := writeConfig(t, `
base = "develop"
leader = ","
theme = "dracula"
agent = "Codex"

[keys]
quit = ["q", "ctrl+q"]
next-focus = []
"ops.inner-change" = ["c i p"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := map[string][]string{
		"quit":             {"q", "ctrl+q"},
		"next-focus":       {},
		"ops.inner-change": {"c i p"},
	}
	if cfg.Base != "develop" || cfg.Leader != "," || cfg.Theme != "dracula" || cfg.Agent != "Codex" || !reflect.DeepEqual(cfg.Keys, want) {
		t.Fatalf("Load() = %+v", cfg)
	}
}

func TestLoadKeepsDefaultsForAbsentSettings(t *testing.T) {
	cfg, err := Load(writeConfig(t, "[keys]\nquit = [\"x\"]\n"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Leader != "space" || cfg.Base != "" {
		t.Fatalf("Load() = %+v, want default leader and empty base", cfg)
	}
}

func TestLoadRejectsInvalidFile(t *testing.T) {
	cases := map[string]struct {
		content, want string
	}{
		"syntax error":     {"base = ", "config.toml"},
		"unknown setting":  {"bsae = \"main\"\n", "unknown setting bsae"},
		"wrong type":       {"leader = 1\n", "config.toml"},
		"empty base":       {"base = \"  \"\n", "base must not be empty"},
		"empty leader":     {"leader = \"\"\n", "leader must not be empty"},
		"empty key string": {"[keys]\nquit = [\"\"]\n", "keys.quit has an empty key"},
		"keys not a list":  {"[keys]\nquit = \"q\"\n", "config.toml"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, tc.content)
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), path) {
				t.Fatalf("Load() error = %v, want it to mention %q and %s", err, tc.want, path)
			}
		})
	}
}

func TestSetAgentCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "revunk", "config.toml")
	if err := SetAgent(path, "Claude Code"); err != nil {
		t.Fatalf("SetAgent() error = %v", err)
	}
	if cfg, err := Load(path); err != nil || cfg.Agent != "Claude Code" {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
}

func TestSetAgentKeepsTheRestOfTheFile(t *testing.T) {
	cases := map[string]struct {
		content, want string
	}{
		"inserted before tables": {
			"# my config\ntheme = \"nord\"\n\n[keys]\nquit = [\"x\"]\n",
			"agent = \"Say \\\"hi\\\"\"\n# my config\ntheme = \"nord\"\n\n[keys]\nquit = [\"x\"]\n",
		},
		"replaced in place": {
			"theme = \"nord\"\n  agent = \"Codex\" # old\n[keys]\n",
			"theme = \"nord\"\nagent = \"Say \\\"hi\\\"\"\n[keys]\n",
		},
		"table key left alone": {
			"[keys]\nagent = [\"g a\"]\n",
			"agent = \"Say \\\"hi\\\"\"\n[keys]\nagent = [\"g a\"]\n",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, tc.content)
			if err := SetAgent(path, `Say "hi"`); err != nil {
				t.Fatalf("SetAgent() error = %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != tc.want {
				t.Fatalf("file = %q, %v, want %q", got, err, tc.want)
			}
			if cfg, err := Load(path); err != nil || cfg.Agent != `Say "hi"` {
				t.Fatalf("Load() = %+v, %v", cfg, err)
			}
		})
	}
}

func TestSetAgentReportsErrors(t *testing.T) {
	if err := SetAgent(t.TempDir(), "Codex"); err == nil {
		t.Fatal("SetAgent() on a directory should fail to read")
	}
	file := writeConfig(t, "")
	if err := SetAgent(filepath.Join(file, "config.toml"), "Codex"); err == nil {
		t.Fatal("SetAgent() under a file should fail")
	}
}
