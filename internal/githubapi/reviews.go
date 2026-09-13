package githubapi

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"reviewstats/internal/repository"
)

const reviewsPerPage = 100

type Review struct {
	PullRequestNumber int
	Reviewer          string
	State             string
	SubmittedAt       time.Time
}

type reviewResponse struct {
	State string `json:"state"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
	SubmittedAt time.Time `json:"submitted_at"`
}

func (c *Client) ListReviews(
	ctx context.Context,
	repo repository.Repository,
	pullRequestNumber int,
) ([]Review, error) {
	reviews := make([]Review, 0)

	for page := 1; ; page++ {
		var currentPage []reviewResponse
		_, err := c.get(
			ctx,
			fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", repo.Owner, repo.Name, pullRequestNumber),
			url.Values{
				"page":     {strconv.Itoa(page)},
				"per_page": {strconv.Itoa(reviewsPerPage)},
			},
			&currentPage,
		)
		if err != nil {
			return nil, fmt.Errorf("list reviews for pull request #%d: %w", pullRequestNumber, err)
		}

		for _, review := range currentPage {
			if review.User.Login == "" || review.SubmittedAt.IsZero() {
				continue
			}

			reviews = append(reviews, Review{
				PullRequestNumber: pullRequestNumber,
				Reviewer:          review.User.Login,
				State:             strings.ToUpper(review.State),
				SubmittedAt:       review.SubmittedAt,
			})
		}

		if len(currentPage) < reviewsPerPage {
			return reviews, nil
		}
	}
}
