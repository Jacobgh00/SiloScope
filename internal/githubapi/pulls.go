package githubapi

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"reviewstats/internal/repository"
)

const pullRequestsPerPage = 100

type PullRequest struct {
	Number    int
	Author    string
	UpdatedAt time.Time
}

type pullRequestResponse struct {
	Number int `json:"number"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *Client) ListPullRequestsUpdatedSince(
	ctx context.Context,
	repo repository.Repository,
	since time.Time,
) ([]PullRequest, error) {
	pullRequests := make([]PullRequest, 0)

	for page := 1; ; page++ {
		var currentPage []pullRequestResponse
		_, err := c.get(
			ctx,
			fmt.Sprintf("/repos/%s/%s/pulls", repo.Owner, repo.Name),
			url.Values{
				"direction": {"desc"},
				"page":      {strconv.Itoa(page)},
				"per_page":  {strconv.Itoa(pullRequestsPerPage)},
				"sort":      {"updated"},
				"state":     {"all"},
			},
			&currentPage,
		)
		if err != nil {
			return nil, fmt.Errorf("list pull requests updated since: %w", err)
		}

		for _, pullRequest := range currentPage {
			if pullRequest.UpdatedAt.Before(since) {
				return pullRequests, nil
			}

			pullRequests = append(pullRequests, PullRequest{
				Number:    pullRequest.Number,
				Author:    pullRequest.User.Login,
				UpdatedAt: pullRequest.UpdatedAt,
			})
		}

		if len(currentPage) < pullRequestsPerPage {
			return pullRequests, nil
		}
	}
}
