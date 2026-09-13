package app

import (
	"context"
	"flag"
	"io"
)

func Run(_ context.Context, args []string, _, stderr io.Writer) int {
	flags := flag.NewFlagSet("reviewstats", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var repo string
	var since string

	flags.StringVar(&repo, "repo", "", "GitHub repository in owner/name form")
	flags.StringVar(&since, "since", "30d", "review period: Nd or YYYY-MM-DD")

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}

		return 2
	}

	_ = repo
	_ = since

	return 0
}
