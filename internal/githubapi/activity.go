package githubapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"reviewstats/internal/repository"
)

const (
	pullRequestsPerGraphQLPage = 20
	reviewsPerGraphQLPage      = 100
)

var reviewActivityQuery = fmt.Sprintf(`
query ReviewActivity($owner: String!, $name: String!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(
      first: %d
      after: $after
      states: [OPEN, CLOSED, MERGED]
      orderBy: {field: UPDATED_AT, direction: DESC}
    ) {
      nodes {
        number
        updatedAt
        author { login }
        reviews(first: %d) {
          nodes {
            author { login }
            state
            submittedAt
          }
          pageInfo { hasNextPage endCursor }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`, pullRequestsPerGraphQLPage, reviewsPerGraphQLPage)

var reviewContinuationQuery = fmt.Sprintf(`
query PullRequestReviewPage($owner: String!, $name: String!, $number: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviews(first: %d, after: $after) {
        nodes {
          author { login }
          state
          submittedAt
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}
`, reviewsPerGraphQLPage)

type PullRequest struct {
	Number    int
	Author    string
	UpdatedAt time.Time
}

type Review struct {
	PullRequestNumber int
	Reviewer          string
	State             string
	SubmittedAt       time.Time
}

type ReviewActivity struct {
	PullRequests  []PullRequest
	ReviewsByPull map[int][]Review
}

type reviewActivityVariables struct {
	Owner string  `json:"owner"`
	Name  string  `json:"name"`
	After *string `json:"after"`
}

type reviewContinuationVariables struct {
	Owner  string  `json:"owner"`
	Name   string  `json:"name"`
	Number int     `json:"number"`
	After  *string `json:"after"`
}

type graphQLReviewActivityResponse struct {
	Repository *graphQLRepository `json:"repository"`
}

type graphQLRepository struct {
	PullRequests graphQLPullRequestConnection `json:"pullRequests"`
	PullRequest  *graphQLPullRequest          `json:"pullRequest"`
}

type graphQLPullRequestConnection struct {
	Nodes    []graphQLPullRequest `json:"nodes"`
	PageInfo graphQLPageInfo      `json:"pageInfo"`
}

type graphQLPullRequest struct {
	Number    int                     `json:"number"`
	Author    *graphQLActor           `json:"author"`
	UpdatedAt time.Time               `json:"updatedAt"`
	Reviews   graphQLReviewConnection `json:"reviews"`
}

type graphQLReviewConnection struct {
	Nodes    []graphQLReviewNode `json:"nodes"`
	PageInfo graphQLPageInfo     `json:"pageInfo"`
}

type graphQLReviewNode struct {
	Author      *graphQLActor `json:"author"`
	State       string        `json:"state"`
	SubmittedAt *time.Time    `json:"submittedAt"`
}

type graphQLActor struct {
	Login string `json:"login"`
}

type graphQLPageInfo struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

func (c *Client) LoadReviewActivity(
	ctx context.Context,
	repo repository.Repository,
	since time.Time,
) (ReviewActivity, error) {
	activity := ReviewActivity{
		PullRequests:  make([]PullRequest, 0),
		ReviewsByPull: make(map[int][]Review),
	}

	var after *string
	for {
		var response graphQLReviewActivityResponse
		err := c.graphql(
			ctx,
			reviewActivityQuery,
			reviewActivityVariables{
				Owner: repo.Owner,
				Name:  repo.Name,
				After: after,
			},
			&response,
		)
		if err != nil {
			return ReviewActivity{}, fmt.Errorf("load review activity: %w", err)
		}

		if response.Repository == nil {
			return ReviewActivity{}, fmt.Errorf("load review activity: repository %s not found", repo)
		}

		for _, pullRequest := range response.Repository.PullRequests.Nodes {
			if pullRequest.UpdatedAt.Before(since) {
				return activity, nil
			}

			activity.PullRequests = append(activity.PullRequests, PullRequest{
				Number:    pullRequest.Number,
				Author:    actorLogin(pullRequest.Author),
				UpdatedAt: pullRequest.UpdatedAt,
			})

			reviews := normalizeReviews(pullRequest.Number, pullRequest.Reviews.Nodes)
			reviewCursor, err := nextCursor(pullRequest.Reviews.PageInfo)
			if err != nil {
				return ReviewActivity{}, fmt.Errorf("load reviews for pull request #%d: %w", pullRequest.Number, err)
			}

			if reviewCursor != nil {
				continuedReviews, err := c.loadReviewContinuation(ctx, repo, pullRequest.Number, reviewCursor)
				if err != nil {
					return ReviewActivity{}, fmt.Errorf("load reviews for pull request #%d: %w", pullRequest.Number, err)
				}

				reviews = append(reviews, continuedReviews...)
			}

			activity.ReviewsByPull[pullRequest.Number] = reviews
		}

		nextPage, err := nextCursor(response.Repository.PullRequests.PageInfo)
		if err != nil {
			return ReviewActivity{}, fmt.Errorf("load review activity: %w", err)
		}

		if nextPage == nil {
			return activity, nil
		}

		after = nextPage
	}
}

func (c *Client) loadReviewContinuation(
	ctx context.Context,
	repo repository.Repository,
	pullRequestNumber int,
	after *string,
) ([]Review, error) {
	reviews := make([]Review, 0)

	for after != nil {
		var response graphQLReviewActivityResponse
		err := c.graphql(
			ctx,
			reviewContinuationQuery,
			reviewContinuationVariables{
				Owner:  repo.Owner,
				Name:   repo.Name,
				Number: pullRequestNumber,
				After:  after,
			},
			&response,
		)
		if err != nil {
			return nil, err
		}

		if response.Repository == nil {
			return nil, fmt.Errorf("repository %s not found", repo)
		}

		if response.Repository.PullRequest == nil {
			return nil, fmt.Errorf("pull request #%d not found", pullRequestNumber)
		}

		reviewPage := response.Repository.PullRequest.Reviews
		reviews = append(reviews, normalizeReviews(pullRequestNumber, reviewPage.Nodes)...)

		nextPage, err := nextCursor(reviewPage.PageInfo)
		if err != nil {
			return nil, err
		}

		after = nextPage
	}

	return reviews, nil
}

func normalizeReviews(pullRequestNumber int, nodes []graphQLReviewNode) []Review {
	reviews := make([]Review, 0, len(nodes))

	for _, node := range nodes {
		if node.Author == nil || node.Author.Login == "" || node.SubmittedAt == nil {
			continue
		}

		reviews = append(reviews, Review{
			PullRequestNumber: pullRequestNumber,
			Reviewer:          node.Author.Login,
			State:             strings.ToUpper(node.State),
			SubmittedAt:       *node.SubmittedAt,
		})
	}

	return reviews
}

func actorLogin(actor *graphQLActor) string {
	if actor == nil {
		return ""
	}

	return actor.Login
}

func nextCursor(pageInfo graphQLPageInfo) (*string, error) {
	if !pageInfo.HasNextPage {
		return nil, nil
	}

	if pageInfo.EndCursor == nil || *pageInfo.EndCursor == "" {
		return nil, fmt.Errorf("GitHub GraphQL response has next page without a cursor")
	}

	return pageInfo.EndCursor, nil
}
