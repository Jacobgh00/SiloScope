package reviewstats

import (
	"reflect"
	"testing"
	"time"

	"reviewstats/internal/githubapi"
)

func TestAggregateCountsOneApprovedReview(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(
		[]githubapi.PullRequest{pullRequest(10, "bob")},
		map[int][]githubapi.Review{
			10: {reviewEvent(10, "alice", "APPROVED", cutoff)},
		},
		cutoff,
	)

	assertParticipants(t, stats, []ParticipantStat{
		{Login: "alice", ReviewedPullRequests: 1},
		{Login: "bob", ReviewedPullRequests: 0},
	})
}

func TestAggregateDeduplicatesRepeatedReviewsOnOnePullRequest(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(
		[]githubapi.PullRequest{pullRequest(10, "bob")},
		map[int][]githubapi.Review{
			10: {
				reviewEvent(10, "alice", "COMMENTED", cutoff),
				reviewEvent(10, "alice", "APPROVED", cutoff.Add(time.Minute)),
			},
		},
		cutoff,
	)

	assertParticipants(t, stats, []ParticipantStat{
		{Login: "alice", ReviewedPullRequests: 1},
		{Login: "bob", ReviewedPullRequests: 0},
	})

	if got := stats.Matrix.Counts["bob"]["alice"]; got != 1 {
		t.Fatalf("matrix bob to alice = %d, want %d", got, 1)
	}
}

func TestAggregateCountsDistinctPullRequests(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(
		[]githubapi.PullRequest{
			pullRequest(10, "bob"),
			pullRequest(11, "bob"),
		},
		map[int][]githubapi.Review{
			10: {reviewEvent(10, "alice", "APPROVED", cutoff)},
			11: {reviewEvent(11, "alice", "APPROVED", cutoff)},
		},
		cutoff,
	)

	assertParticipants(t, stats, []ParticipantStat{
		{Login: "alice", ReviewedPullRequests: 2},
		{Login: "bob", ReviewedPullRequests: 0},
	})
}

func TestAggregateKeepsPullRequestAuthorsWithNoQualifyingReviews(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(
		[]githubapi.PullRequest{pullRequest(10, "alice")},
		map[int][]githubapi.Review{
			10: {reviewEvent(10, "bob", "APPROVED", cutoff.Add(-time.Second))},
		},
		cutoff,
	)

	assertParticipants(t, stats, []ParticipantStat{
		{Login: "alice", ReviewedPullRequests: 0},
	})
}

func TestAggregateExcludesSelfReviewsAndPendingReviews(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(
		[]githubapi.PullRequest{pullRequest(10, "alice")},
		map[int][]githubapi.Review{
			10: {
				reviewEvent(10, "alice", "APPROVED", cutoff),
				reviewEvent(10, "bob", "PENDING", cutoff),
			},
		},
		cutoff,
	)

	assertParticipants(t, stats, []ParticipantStat{
		{Login: "alice", ReviewedPullRequests: 0},
	})
}

func TestAggregateCountsDismissedReviews(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(
		[]githubapi.PullRequest{pullRequest(10, "bob")},
		map[int][]githubapi.Review{
			10: {reviewEvent(10, "alice", "DISMISSED", cutoff)},
		},
		cutoff,
	)

	assertParticipants(t, stats, []ParticipantStat{
		{Login: "alice", ReviewedPullRequests: 1},
		{Login: "bob", ReviewedPullRequests: 0},
	})
}

func TestAggregateSortsParticipantsByReviewCountThenLogin(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(
		[]githubapi.PullRequest{
			pullRequest(10, "author"),
			pullRequest(11, "author"),
		},
		map[int][]githubapi.Review{
			10: {
				reviewEvent(10, "alpha", "APPROVED", cutoff),
				reviewEvent(10, "bob", "APPROVED", cutoff),
				reviewEvent(10, "zed", "APPROVED", cutoff),
			},
			11: {reviewEvent(11, "bob", "APPROVED", cutoff)},
		},
		cutoff,
	)

	assertParticipants(t, stats, []ParticipantStat{
		{Login: "bob", ReviewedPullRequests: 2},
		{Login: "alpha", ReviewedPullRequests: 1},
		{Login: "zed", ReviewedPullRequests: 1},
		{Login: "author", ReviewedPullRequests: 0},
	})
}

func TestAggregateBuildsDistinctMatrixLinksAndIncludesExternalReviewers(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(
		[]githubapi.PullRequest{
			pullRequest(10, "bob"),
			pullRequest(11, "charlie"),
			pullRequest(12, "alice"),
		},
		map[int][]githubapi.Review{
			10: {
				reviewEvent(10, "alice", "COMMENTED", cutoff),
				reviewEvent(10, "alice", "APPROVED", cutoff.Add(time.Minute)),
				reviewEvent(10, "eve", "APPROVED", cutoff),
			},
			11: {reviewEvent(11, "alice", "CHANGES_REQUESTED", cutoff)},
			12: {reviewEvent(12, "bob", "DISMISSED", cutoff)},
		},
		cutoff,
	)

	assertParticipants(t, stats, []ParticipantStat{
		{Login: "alice", ReviewedPullRequests: 2},
		{Login: "bob", ReviewedPullRequests: 1},
		{Login: "eve", ReviewedPullRequests: 1},
		{Login: "charlie", ReviewedPullRequests: 0},
	})

	wantMatrix := ReviewMatrix{
		Authors:   []string{"alice", "bob", "charlie"},
		Reviewers: []string{"alice", "bob", "eve"},
		Counts: map[string]map[string]int{
			"alice":   {"bob": 1},
			"bob":     {"alice": 1, "eve": 1},
			"charlie": {"alice": 1},
		},
	}
	if !reflect.DeepEqual(stats.Matrix, wantMatrix) {
		t.Fatalf("matrix = %#v, want %#v", stats.Matrix, wantMatrix)
	}
}

func pullRequest(number int, author string) githubapi.PullRequest {
	return githubapi.PullRequest{Number: number, Author: author}
}

func reviewEvent(number int, reviewer, state string, submittedAt time.Time) githubapi.Review {
	return githubapi.Review{
		PullRequestNumber: number,
		Reviewer:          reviewer,
		State:             state,
		SubmittedAt:       submittedAt,
	}
}

func assertParticipants(t *testing.T, stats RetroStats, want []ParticipantStat) {
	t.Helper()

	if !reflect.DeepEqual(stats.Participants, want) {
		t.Fatalf("participants = %#v, want %#v", stats.Participants, want)
	}
}
