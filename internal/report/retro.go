package report

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"reviewstats/internal/repository"
	"reviewstats/internal/reviewstats"
)

func WriteRetro(
	writer io.Writer,
	repo repository.Repository,
	since time.Time,
	stats reviewstats.RetroStats,
) error {
	if _, err := fmt.Fprintf(writer, "Repository: %s\nSince:      %s\n\n", repo, since.Format(time.DateOnly)); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(writer, "Review participation"); err != nil {
		return err
	}
	if err := writeParticipation(writer, stats.Members); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, "Review matrix"); err != nil {
		return err
	}
	if err := writeMatrix(writer, stats.Matrix); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}
	_, err := fmt.Fprint(writer, "Retro prompt:\n"+
		"Use this report to discuss whether review work is concentrated on a few people\n"+
		"and whether recurring reviewer/author pairs could indicate knowledge silos.\n"+
		"Counts are discussion signals, not individual performance scores.\n")

	return err
}

func writeParticipation(writer io.Writer, members []reviewstats.MemberStat) error {
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "Reviewer\tPRs reviewed"); err != nil {
		return err
	}

	for _, member := range members {
		if _, err := fmt.Fprintf(table, "%s\t%d\n", member.Login, member.PullRequests); err != nil {
			return err
		}
	}

	return table.Flush()
}

func writeMatrix(writer io.Writer, matrix reviewstats.ReviewMatrix) error {
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprint(table, "Author/Reviewer"); err != nil {
		return err
	}
	for _, reviewer := range matrix.Reviewers {
		if _, err := fmt.Fprintf(table, "\t%s", reviewer); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(table); err != nil {
		return err
	}

	for _, author := range matrix.Authors {
		if _, err := fmt.Fprint(table, author); err != nil {
			return err
		}

		for _, reviewer := range matrix.Reviewers {
			if author == reviewer {
				if _, err := fmt.Fprint(table, "\t-"); err != nil {
					return err
				}
				continue
			}

			if _, err := fmt.Fprintf(table, "\t%d", matrix.Counts[author][reviewer]); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprintln(table); err != nil {
			return err
		}
	}

	return table.Flush()
}
