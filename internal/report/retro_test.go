package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"reviewstats/internal/repository"
	"reviewstats/internal/reviewstats"
)

func TestWriteRetro(t *testing.T) {
	t.Parallel()

	stats := reviewstats.RetroStats{
		Members: []reviewstats.MemberStat{
			{Login: "alice", PullRequests: 2},
			{Login: "bob", PullRequests: 1},
			{Login: "charlie", PullRequests: 0},
		},
		Matrix: reviewstats.ReviewMatrix{
			Authors:   []string{"alice", "bob", "charlie"},
			Reviewers: []string{"alice", "bob", "charlie"},
			Counts: map[string]map[string]int{
				"alice":   {"bob": 2},
				"bob":     {"alice": 1, "charlie": 1},
				"charlie": {"alice": 1},
			},
		},
	}

	var output bytes.Buffer
	err := WriteRetro(
		&output,
		repository.Repository{Owner: "acme", Name: "frontend"},
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		stats,
	)
	if err != nil {
		t.Fatalf("WriteRetro() error = %v", err)
	}

	want := `Repository: acme/frontend
Since:      2026-09-01

Review participation
Reviewer  PRs reviewed
alice     2
bob       1
charlie   0

Review matrix
Author/Reviewer  alice  bob  charlie
alice            -      2    0
bob              1      -    1
charlie          1      0    -

Retro prompt:
Use this report to discuss whether review work is concentrated on a few people
and whether recurring reviewer/author pairs could indicate knowledge silos.
Counts are discussion signals, not individual performance scores.
`
	if output.String() != want {
		t.Fatalf("WriteRetro() output =\n%s\nwant:\n%s", output.String(), want)
	}

	for _, line := range strings.Split(output.String(), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("output line has trailing whitespace: %q", line)
		}
	}
}

func TestWriteRetroReturnsWriterError(t *testing.T) {
	t.Parallel()

	err := WriteRetro(
		failingWriter{},
		repository.Repository{Owner: "acme", Name: "frontend"},
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		reviewstats.RetroStats{},
	)
	if err == nil {
		t.Fatal("WriteRetro() error = nil, want error")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
