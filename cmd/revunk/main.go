package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jfmainville/revunk/internal/agent"
	"github.com/jfmainville/revunk/internal/config"
	"github.com/jfmainville/revunk/internal/provider/git"
	"github.com/jfmainville/revunk/internal/provider/github"
	"github.com/jfmainville/revunk/internal/review"
	"github.com/jfmainville/revunk/internal/store"
	"github.com/jfmainville/revunk/internal/tui"
	"github.com/jfmainville/revunk/internal/upgrade"
)

var version = "dev"

const usage = `Usage: revunk [-base <branch>] [-cleanup] [-version] [command]

Reviews the commits on the current git branch against its base branch.

Commands:
  tui                  Launch the terminal review interface (default)
  upgrade              Replace this binary with the latest GitHub release

Flags:
  -base     Branch to compare against (default: config base, else origin's default branch, else main)
  -cleanup  Delete .git/revunk.json, every saved comment and viewed file, then exit
  -leader   Key that starts leader sequences, such as <leader> e for the explorer (default: config leader, else space)
  -version  Print the version, then exit
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
	cleanup := flags.Bool("cleanup", false, "")
	leader := flags.String("leader", "", "")
	showVersion := flags.Bool("version", false, "")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Println("revunk", version)
		return nil
	}
	if *cleanup {
		ctx := context.Background()
		root, err := git.Root(ctx, ".")
		if err != nil {
			return fmt.Errorf("run revunk inside a git repository: %w", err)
		}
		path, err := git.StatePath(ctx, root)
		if err != nil {
			return err
		}
		if err := os.Remove(path); errors.Is(err, fs.ErrNotExist) {
			fmt.Println("nothing to clean up, no", path)
			return nil
		} else if err != nil {
			return err
		}
		fmt.Println("removed", path)
		return nil
	}
	command := "tui"
	if flags.NArg() > 0 {
		command = flags.Arg(0)
	}
	switch command {
	case "tui":
		cfg, path, err := loadConfig()
		if err != nil {
			return err
		}
		model, err := setup(context.Background(), ".", pick(*base, cfg.Base))
		if err != nil {
			return err
		}
		model, err = model.WithLeader(pick(*leader, cfg.Leader)).WithKeys(cfg.Keys)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		model, err = model.WithTheme(cfg.Theme)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		_, err = tea.NewProgram(model).Run()
		return err
	case "reply":
		if flags.NArg() < 3 {
			return errors.New("usage: revunk reply <id> <text>")
		}
		cfg, _, err := loadConfig()
		if err != nil {
			return err
		}
		ctx := context.Background()
		svc, _, _, _, err := service(ctx, ".", pick(*base, cfg.Base))
		if err != nil {
			return err
		}
		if err := svc.Reply(ctx, flags.Arg(1), strings.Join(flags.Args()[2:], " ")); err != nil {
			return err
		}
		fmt.Println("replied to", flags.Arg(1))
		return nil
	case "upgrade":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return err
		}
		up := upgrade.Updater{
			HTTP:        &http.Client{Timeout: 5 * time.Minute},
			APIURL:      "https://api.github.com/repos/jfmainville/revunk",
			DownloadURL: "https://github.com/jfmainville/revunk",
		}
		return up.Run(context.Background(), exe, version, os.Stdout)
	case "help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", command)
	}
}

func loadConfig() (config.Config, string, error) {
	path, err := config.Path()
	if err != nil {
		return config.Config{}, "", err
	}
	cfg, err := config.Load(path)
	return cfg, path, err
}

func pick(flag, fromConfig string) string {
	if flag != "" {
		return flag
	}
	return fromConfig
}

func setup(ctx context.Context, dir, base string) (tui.Model, error) {
	svc, st, root, base, err := service(ctx, dir, base)
	if err != nil {
		return tui.Model{}, err
	}
	checkout := func(ctx context.Context, pr review.PullRequest) error { return git.CheckoutPullRequest(ctx, root, pr) }
	return tui.New(svc, root, base).WithHost(github.Client{}, pullRequestReview(github.Client{}, st)).WithCheckout(checkout).WithRepo(git.Repo(ctx, root)), nil
}

func pullRequestReview(client github.Client, st review.Store) func(review.PullRequest) *review.Service {
	return func(pr review.PullRequest) *review.Service {
		source := github.Source{Client: client, Repo: pr.Repo, Number: pr.Number}
		comments := github.Review{Store: st, Client: client, Repo: pr.Repo, Number: pr.Number}
		return review.NewService(source, comments, nil, fmt.Sprintf("%s#%d", pr.Repo, pr.Number))
	}
}

func service(ctx context.Context, dir, base string) (*review.Service, *store.JSON, string, string, error) {
	root, err := git.Root(ctx, dir)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("run revunk inside a git repository: %w", err)
	}
	branch, err := git.Branch(ctx, root)
	if err != nil {
		return nil, nil, "", "", err
	}
	if base == "" {
		base = git.DefaultBase(ctx, root)
	}
	statePath, err := git.StatePath(ctx, root)
	if err != nil {
		return nil, nil, "", "", err
	}
	st, err := store.Open(statePath)
	if err != nil {
		return nil, nil, "", "", err
	}
	branches, err := git.Branches(ctx, root)
	if err != nil {
		return nil, nil, "", "", err
	}
	err = st.KeepBranches(func(b string) bool {
		return b == branch || slices.Contains(branches, b) || strings.Contains(b, "#")
	})
	if err != nil {
		return nil, nil, "", "", err
	}
	svc := review.NewService(git.Source{Dir: root, Base: base}, st, agent.New(root), branch)
	svc.ReplyCommand, _ = os.Executable()
	svc.CurrentBranch = func(ctx context.Context) (string, error) { return git.Branch(ctx, root) }
	svc.Exists = func(path string) bool {
		_, err := os.Lstat(filepath.Join(root, path))
		return err == nil
	}
	return svc, st, root, base, nil
}
