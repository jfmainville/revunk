package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jfmainville/revunk/internal/review"
)

var multiplexers = []struct {
	env string
	new func(dir string) review.Agent
}{
	{"TUIOS_PANE_ID", func(dir string) review.Agent { return Tuios{Dir: dir} }},
	{"HERDR_PANE_ID", func(dir string) review.Agent { return Agent{Dir: dir} }},
}

func New(dir string) review.Agent {
	for _, m := range multiplexers {
		if os.Getenv(m.env) != "" {
			return m.new(dir)
		}
	}
	return noMultiplexer{}
}

type noMultiplexer struct{}

var errNoMultiplexer = errors.New("no multiplexer found, run Revunk inside herdr or tuios")

func (noMultiplexer) Panes(context.Context) ([]review.AgentPane, error) {
	return nil, errNoMultiplexer
}

func (noMultiplexer) Prompt(context.Context, review.AgentPane, string) (string, error) {
	return "", errNoMultiplexer
}

func inRepo(cwd, dir string) bool {
	return cwd == dir || strings.HasPrefix(cwd, dir+"/")
}

type Agent struct {
	Dir string
}

var names = map[string]string{
	"agy": "Antigravity", "amp": "Amp", "claude": "Claude Code", "cline": "Cline", "codex": "Codex",
	"copilot": "Copilot", "cursor": "Cursor", "devin": "Devin", "droid": "Droid", "gemini": "Gemini",
	"grok": "Grok", "hermes": "Hermes", "kilo": "Kilo", "kimi": "Kimi", "kiro": "Kiro", "letta": "Letta",
	"maki": "Maki", "mastracode": "Mastra Code", "muse": "Muse", "omp": "omp", "opencode": "opencode",
	"pi": "pi", "qodercli": "Qoder", "qwen": "Qwen",
}

func (a Agent) Prompt(ctx context.Context, pane review.AgentPane, text string) (string, error) {
	if pane.ID == "" {
		if os.Getenv("HERDR_PANE_ID") == "" {
			return "", fmt.Errorf("no agent session open in %s", a.Dir)
		}
		id, err := a.start(ctx)
		if err != nil {
			return "", err
		}
		pane = review.AgentPane{ID: id, Name: "a new Claude Code pane"}
	}
	_, err := run(ctx, "agent", "prompt", pane.ID, text)
	return pane.Name, err
}

func (a Agent) Panes(ctx context.Context) ([]review.AgentPane, error) {
	out, err := run(ctx, "agent", "list")
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("read herdr agents: %w", err)
	}
	var panes []review.AgentPane
	for _, ag := range list.Result.Agents {
		if name, ok := names[ag.Agent]; ok && inRepo(ag.Cwd, a.Dir) && ag.PaneID != os.Getenv("HERDR_PANE_ID") {
			panes = append(panes, review.AgentPane{ID: ag.PaneID, Name: name})
		}
	}
	return panes, nil
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

func run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "herdr", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("herdr %s %s: %w %s", args[0], args[1], err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
