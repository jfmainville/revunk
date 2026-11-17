package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Agent struct {
	Dir string
}

var names = map[string]string{"claude": "Claude Code", "opencode": "opencode"}

func (a Agent) Prompt(ctx context.Context, text string) (string, error) {
	pane, name, err := a.session(ctx)
	if err != nil {
		return "", err
	}
	if pane == "" {
		if os.Getenv("HERDR_PANE_ID") == "" {
			return "", fmt.Errorf("no Claude Code or opencode session open in %s", a.Dir)
		}
		if pane, err = a.start(ctx); err != nil {
			return "", err
		}
		name = "a new Claude Code pane"
	}
	_, err = run(ctx, "agent", "prompt", pane, text)
	return name, err
}

func (a Agent) session(ctx context.Context) (string, string, error) {
	out, err := run(ctx, "agent", "list")
	if err != nil {
		return "", "", err
	}
	var list struct {
		Result struct {
			Agents []struct {
				Agent  string `json:"agent"`
				Cwd    string `json:"cwd"`
				PaneID string `json:"pane_id"`
			} `json:"agents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return "", "", fmt.Errorf("read herdr agents: %w", err)
	}
	for _, ag := range list.Result.Agents {
		inRepo := ag.Cwd == a.Dir || strings.HasPrefix(ag.Cwd, a.Dir+"/")
		if name, ok := names[ag.Agent]; ok && inRepo && ag.PaneID != os.Getenv("HERDR_PANE_ID") {
			return ag.PaneID, name, nil
		}
	}
	return "", "", nil
}

func (a Agent) start(ctx context.Context) (string, error) {
	out, err := run(ctx, "pane", "split", "--current", "--direction", "right", "--cwd", a.Dir, "--no-focus")
	if err != nil {
		return "", err
	}
	var split struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &split) != nil || split.Result.Pane.PaneID == "" {
		return "", errors.New("herdr pane split returned no pane")
	}
	pane := split.Result.Pane.PaneID
	_, err = run(ctx, "agent", "start", "revunk-"+strings.ReplaceAll(pane, ":", "-"), "--kind", "claude", "--pane", pane)
	return pane, err
}

func FocusPane(ctx context.Context, direction string) error {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return nil
	}
	_, err := run(ctx, "pane", "focus", "--direction", direction, "--pane", pane)
	return err
}

func run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "herdr", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("herdr %s %s: %w %s", args[0], args[1], err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
