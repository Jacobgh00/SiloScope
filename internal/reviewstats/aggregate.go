package reviewstats

import (
	"sort"
	"time"

	"reviewstats/internal/githubapi"
)

type MemberStat struct {
	Login        string
	PullRequests int
}

type ReviewMatrix struct {
	Authors   []string
	Reviewers []string
	Counts    map[string]map[string]int
}

type RetroStats struct {
	Members []MemberStat
	Matrix  ReviewMatrix
}

func Aggregate(
	collaborators []githubapi.Collaborator,
	pulls []githubapi.PullRequest,
	reviewsByPull map[int][]githubapi.Review,
	since time.Time,
) RetroStats {
	members := make(map[string]struct{})
	authors := make(map[string]struct{})
	reviewedPullsByReviewer := make(map[string]map[int]struct{})
	matrixPulls := make(map[string]map[string]map[int]struct{})

	for _, collaborator := range collaborators {
		if collaborator.Login != "" {
			members[collaborator.Login] = struct{}{}
		}
	}

	for _, pull := range pulls {
		if pull.Author != "" {
			authors[pull.Author] = struct{}{}
		}

		for _, review := range reviewsByPull[pull.Number] {
			if !qualifies(review, pull.Author, since) {
				continue
			}

			members[review.Reviewer] = struct{}{}
			addPullForReviewer(reviewedPullsByReviewer, review.Reviewer, pull.Number)

			if pull.Author != "" {
				addPullForMatrix(matrixPulls, pull.Author, review.Reviewer, pull.Number)
			}
		}
	}

	memberStats := memberStatsFrom(members, reviewedPullsByReviewer)
	authorLogins := sortedLogins(authors)
	reviewerLogins := sortedLogins(members)

	return RetroStats{
		Members: memberStats,
		Matrix: ReviewMatrix{
			Authors:   authorLogins,
			Reviewers: reviewerLogins,
			Counts:    matrixCounts(authorLogins, matrixPulls),
		},
	}
}

func qualifies(review githubapi.Review, author string, since time.Time) bool {
	return review.Reviewer != "" &&
		review.Reviewer != author &&
		!review.SubmittedAt.Before(since) &&
		review.State != "PENDING"
}

func addPullForReviewer(reviewedPullsByReviewer map[string]map[int]struct{}, reviewer string, pullNumber int) {
	if reviewedPullsByReviewer[reviewer] == nil {
		reviewedPullsByReviewer[reviewer] = make(map[int]struct{})
	}

	reviewedPullsByReviewer[reviewer][pullNumber] = struct{}{}
}

func addPullForMatrix(
	matrixPulls map[string]map[string]map[int]struct{},
	author, reviewer string,
	pullNumber int,
) {
	if matrixPulls[author] == nil {
		matrixPulls[author] = make(map[string]map[int]struct{})
	}

	if matrixPulls[author][reviewer] == nil {
		matrixPulls[author][reviewer] = make(map[int]struct{})
	}

	matrixPulls[author][reviewer][pullNumber] = struct{}{}
}

func memberStatsFrom(
	members map[string]struct{},
	reviewedPullsByReviewer map[string]map[int]struct{},
) []MemberStat {
	stats := make([]MemberStat, 0, len(members))
	for login := range members {
		stats = append(stats, MemberStat{
			Login:        login,
			PullRequests: len(reviewedPullsByReviewer[login]),
		})
	}

	sort.Slice(stats, func(left, right int) bool {
		if stats[left].PullRequests != stats[right].PullRequests {
			return stats[left].PullRequests > stats[right].PullRequests
		}

		return stats[left].Login < stats[right].Login
	})

	return stats
}

func sortedLogins(logins map[string]struct{}) []string {
	sorted := make([]string, 0, len(logins))
	for login := range logins {
		sorted = append(sorted, login)
	}
	sort.Strings(sorted)

	return sorted
}

func matrixCounts(
	authorLogins []string,
	matrixPulls map[string]map[string]map[int]struct{},
) map[string]map[string]int {
	counts := make(map[string]map[string]int, len(authorLogins))
	for _, author := range authorLogins {
		counts[author] = make(map[string]int)
		for reviewer, pullNumbers := range matrixPulls[author] {
			counts[author][reviewer] = len(pullNumbers)
		}
	}

	return counts
}
