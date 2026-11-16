package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/jfmainville/revunk/internal/provider/github"
	"github.com/jfmainville/revunk/internal/review"
	"github.com/jfmainville/revunk/internal/store"
	"github.com/jfmainville/revunk/internal/tui"
)

const usage = `Usage: revunk <command>

Commands:
  tui    Launch the terminal review interface (default)
  serve  Start the review API server (not implemented yet)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "revunk:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "tui"
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "tui":
		svc := review.NewService(github.SampleSource{}, store.NewMemory())
		pr := review.PullRequest{Owner: "jfmainville", Repo: "revunk", Number: 1}
		_, err := tea.NewProgram(tui.New(svc, pr)).Run()
		return err
	case "serve":
		return errors.New("serve is not implemented yet")
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", command)
	}
}
