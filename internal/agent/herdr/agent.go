package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Agent struct {
	Dir string
}

func (a Agent) Prompt(ctx context.Context, text string) error {
	out, err := run(ctx, "agent", "list")
	if err != nil {
		return err
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
		return fmt.Errorf("read herdr agents: %w", err)
	}
	for _, ag := range list.Result.Agents {
		inRepo := ag.Cwd == a.Dir || strings.HasPrefix(ag.Cwd, a.Dir+"/")
		if ag.Agent == "claude" && inRepo && ag.PaneID != os.Getenv("HERDR_PANE_ID") {
			_, err := run(ctx, "agent", "prompt", ag.PaneID, text)
			return err
		}
	}
	return fmt.Errorf("no Claude Code session open in %s", a.Dir)
}

func run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "herdr", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("herdr agent %s: %w %s", args[1], err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
