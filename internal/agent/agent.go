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
	if pane, name := herdrSession(ctx, a.Dir); pane != "" {
		_, err := run(exec.CommandContext(ctx, "herdr", "agent", "prompt", pane, text))
		return name, err
	}
	if pane, name := tmuxSession(ctx, a.Dir); pane != "" {
		return name, tmuxSend(ctx, pane, text)
	}
	switch {
	case os.Getenv("HERDR_PANE_ID") != "":
		return "a new Claude Code pane", herdrStart(ctx, a.Dir, text)
	case os.Getenv("TMUX") != "":
		_, err := run(exec.CommandContext(ctx, "tmux", "new-window", "-d", "-c", a.Dir, "claude", text))
		return "a new Claude Code tmux window", err
	}
	cmd := exec.CommandContext(ctx, "claude", "--bg", text)
	cmd.Dir = a.Dir
	_, err := run(cmd)
	return "a new background Claude Code session, see claude agents", err
}

func inside(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+"/")
}

func herdrSession(ctx context.Context, dir string) (string, string) {
	out, err := run(exec.CommandContext(ctx, "herdr", "agent", "list"))
	if err != nil {
		return "", ""
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
	if json.Unmarshal(out, &list) != nil {
		return "", ""
	}
	for _, ag := range list.Result.Agents {
		if name, ok := names[ag.Agent]; ok && inside(dir, ag.Cwd) && ag.PaneID != os.Getenv("HERDR_PANE_ID") {
			return ag.PaneID, name
		}
	}
	return "", ""
}

func herdrStart(ctx context.Context, dir, text string) error {
	out, err := run(exec.CommandContext(ctx, "herdr", "pane", "split", "--current", "--direction", "right", "--cwd", dir, "--no-focus"))
	if err != nil {
		return err
	}
	var split struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &split) != nil || split.Result.Pane.PaneID == "" {
		return errors.New("herdr pane split returned no pane")
	}
	pane := split.Result.Pane.PaneID
	if _, err := run(exec.CommandContext(ctx, "herdr", "agent", "start", "revunk-"+strings.ReplaceAll(pane, ":", "-"), "--kind", "claude", "--pane", pane)); err != nil {
		return err
	}
	_, err = run(exec.CommandContext(ctx, "herdr", "agent", "prompt", pane, text))
	return err
}

func tmuxSession(ctx context.Context, dir string) (string, string) {
	out, err := run(exec.CommandContext(ctx, "tmux", "list-panes", "-a", "-F", "#{pane_id}\t#{pane_current_command}\t#{pane_current_path}"))
	if err != nil {
		return "", ""
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		if name, ok := names[fields[1]]; ok && inside(dir, fields[2]) && fields[0] != os.Getenv("TMUX_PANE") {
			return fields[0], name
		}
	}
	return "", ""
}

func tmuxSend(ctx context.Context, pane, text string) error {
	load := exec.CommandContext(ctx, "tmux", "load-buffer", "-b", "revunk", "-")
	load.Stdin = strings.NewReader(text)
	if _, err := run(load); err != nil {
		return err
	}
	if _, err := run(exec.CommandContext(ctx, "tmux", "paste-buffer", "-p", "-d", "-b", "revunk", "-t", pane)); err != nil {
		return err
	}
	_, err := run(exec.CommandContext(ctx, "tmux", "send-keys", "-t", pane, "Enter"))
	return err
}

func run(cmd *exec.Cmd) ([]byte, error) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w %s", strings.Join(cmd.Args[:min(3, len(cmd.Args))], " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
