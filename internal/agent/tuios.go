package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Tuios struct {
	Dir string
}

var tuiosNames = map[string]string{
	"aider": "Aider", "amp": "Amp", "antigravity": "Antigravity", "claude-code": "Claude Code", "cline": "Cline",
	"codex": "Codex", "copilot": "Copilot", "crush": "Crush", "cursor-agent": "Cursor", "devin": "Devin",
	"droid": "Droid", "gemini-cli": "Gemini", "goose": "Goose", "grok": "Grok", "hermes": "Hermes",
	"kilo": "Kilo", "kimi": "Kimi", "kiro": "Kiro", "letta": "Letta", "maki": "Maki", "omp": "omp",
	"opencode": "opencode", "pi": "pi", "qoder": "Qoder", "qwen": "Qwen",
}

func (t Tuios) Prompt(ctx context.Context, text string) (string, error) {
	window, name, err := t.session(ctx)
	if err != nil {
		return "", err
	}
	if window == "" {
		_, err = tuiosRun(ctx, "start-agent", "claude", "--cwd", t.Dir, "--prompt", text)
		return "a new Claude Code pane", err
	}
	_, err = tuiosRun(ctx, "queue", "-w", window, text)
	return name, err
}

func (t Tuios) session(ctx context.Context) (string, string, error) {
	out, err := tuiosRun(ctx, "list-agents", "--json")
	if err != nil {
		return "", "", err
	}
	var list struct {
		Agents []struct {
			HarnessID string `json:"harness_id"`
			Cwd       string `json:"cwd"`
			WindowID  string `json:"window_id"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return "", "", fmt.Errorf("read tuios agents: %w", err)
	}
	for _, ag := range list.Agents {
		inRepo := ag.Cwd == t.Dir || strings.HasPrefix(ag.Cwd, t.Dir+"/")
		if name, ok := tuiosNames[ag.HarnessID]; ok && inRepo && ag.WindowID != os.Getenv("TUIOS_PANE_ID") {
			return ag.WindowID, name, nil
		}
	}
	return "", "", nil
}

func tuiosRun(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "tuios", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tuios %s: %w %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
