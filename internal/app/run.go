package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"reviewstats/internal/auth"
	"reviewstats/internal/githubapi"
	"reviewstats/internal/period"
	"reviewstats/internal/report"
	"reviewstats/internal/repository"
	"reviewstats/internal/reviewstats"
)

type dependencies struct {
	now          func() time.Time
	currentRepo  func(context.Context) (repository.Repository, error)
	resolveToken func(context.Context) (string, error)
	newClient    func(string) githubClient
}

type githubClient interface {
	ListPullRequestsUpdatedSince(context.Context, repository.Repository, time.Time) ([]githubapi.PullRequest, error)
	ListReviews(context.Context, repository.Repository, int) ([]githubapi.Review, error)
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWithDependencies(ctx, args, stdout, stderr, productionDependencies())
}

func runWithDependencies(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	dependencies dependencies,
) int {
	flags := flag.NewFlagSet("reviewstats", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var repoValue string
	var sinceValue string

	flags.StringVar(&repoValue, "repo", "", "GitHub repository in owner/name form")
	flags.StringVar(&sinceValue, "since", "30d", "review period: Nd or YYYY-MM-DD")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}

		return 2
	}

	if flags.NArg() > 0 {
		reportError(stderr, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " ")))
		return 2
	}

	repo, exitCode := resolveRepository(ctx, repoValue, dependencies, stderr)
	if exitCode != 0 {
		return exitCode
	}

	since, err := period.ParseSince(sinceValue, dependencies.now())
	if err != nil {
		reportError(stderr, err)
		return 2
	}

	token, err := dependencies.resolveToken(ctx)
	if err != nil {
		reportError(stderr, err)
		return 2
	}

	client := dependencies.newClient(token)
	pulls, err := client.ListPullRequestsUpdatedSince(ctx, repo, since)
	if err != nil {
		reportError(stderr, err)
		return 1
	}

	reviewsByPull := make(map[int][]githubapi.Review, len(pulls))
	for _, pull := range pulls {
		reviews, err := client.ListReviews(ctx, repo, pull.Number)
		if err != nil {
			reportError(stderr, err)
			return 1
		}

		reviewsByPull[pull.Number] = reviews
	}

	stats := reviewstats.Aggregate(pulls, reviewsByPull, since)
	if err := report.WriteRetro(stdout, repo, since, stats); err != nil {
		reportError(stderr, err)
		return 1
	}

	return 0
}

func productionDependencies() dependencies {
	return dependencies{
		now:         time.Now,
		currentRepo: repository.Current,
		resolveToken: func(ctx context.Context) (string, error) {
			return auth.ResolveToken(ctx, os.Getenv, auth.ExecCommandRunner{})
		},
		newClient: func(token string) githubClient {
			return githubapi.NewClient(token, nil)
		},
	}
}

func resolveRepository(
	ctx context.Context,
	repoValue string,
	dependencies dependencies,
	stderr io.Writer,
) (repository.Repository, int) {
	if repoValue == "" {
		repo, err := dependencies.currentRepo(ctx)
		if err != nil {
			reportError(stderr, err)
			return repository.Repository{}, 2
		}

		return repo, 0
	}

	repo, err := repository.Parse(repoValue)
	if err != nil {
		reportError(stderr, err)
		return repository.Repository{}, 2
	}

	return repo, 0
}

func reportError(writer io.Writer, err error) {
	_, _ = fmt.Fprintf(writer, "error: %v\n", err)
}
