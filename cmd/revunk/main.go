package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/jfmainville/revunk/internal/agent/herdr"
	"github.com/jfmainville/revunk/internal/provider/git"
	"github.com/jfmainville/revunk/internal/review"
	"github.com/jfmainville/revunk/internal/store"
	"github.com/jfmainville/revunk/internal/tui"
)

const usage = `Usage: revunk [-base <branch>] [command]

Reviews the commits on the current git branch against its base branch.

Commands:
  tui    Launch the terminal review interface (default)

Flags:
  -base  Branch to compare against (default: origin's default branch, else main)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "revunk:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("revunk", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(flags.Output(), usage) }
	base := flags.String("base", "", "")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	command := "tui"
	if flags.NArg() > 0 {
		command = flags.Arg(0)
	}
	switch command {
	case "tui":
		model, err := setup(context.Background(), ".", *base)
		if err != nil {
			return err
		}
		_, err = tea.NewProgram(model).Run()
		return err
	case "help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", command)
	}
}

func setup(ctx context.Context, dir, base string) (tui.Model, error) {
	root, err := git.Root(ctx, dir)
	if err != nil {
		return tui.Model{}, fmt.Errorf("run revunk inside a git repository: %w", err)
	}
	branch, err := git.Branch(ctx, root)
	if err != nil {
		return tui.Model{}, err
	}
	if base == "" {
		base = git.DefaultBase(ctx, root)
	}
	statePath, err := git.StatePath(ctx, root)
	if err != nil {
		return tui.Model{}, err
	}
	st, err := store.Open(statePath)
	if err != nil {
		return tui.Model{}, err
	}
	svc := review.NewService(git.Source{Dir: root, Base: base}, st, herdr.Agent{Dir: root}, branch)
	return tui.New(svc, filepath.Base(root), base), nil
}
