package githubapi

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	"reviewstats/internal/repository"
)

const collaboratorsPerPage = 100

type Collaborator struct {
	Login string `json:"login"`
}

func (c *Client) ListCollaborators(ctx context.Context, repo repository.Repository) ([]Collaborator, error) {
	logins := make(map[string]struct{})

	for page := 1; ; page++ {
		var currentPage []Collaborator
		_, err := c.get(
			ctx,
			fmt.Sprintf("/repos/%s/%s/collaborators", repo.Owner, repo.Name),
			url.Values{
				"page":     {strconv.Itoa(page)},
				"per_page": {strconv.Itoa(collaboratorsPerPage)},
			},
			&currentPage,
		)

		if err != nil {
			return nil, fmt.Errorf("list repository collaborators: %w", err)
		}

		for _, collaborator := range currentPage {
			if collaborator.Login != "" {
				logins[collaborator.Login] = struct{}{}
			}
		}

		if len(currentPage) < collaboratorsPerPage {
			break
		}
	}

	sortedLogins := make([]string, 0, len(logins))
	for login := range logins {
		sortedLogins = append(sortedLogins, login)
	}

	sort.Strings(sortedLogins)

	collaborators := make([]Collaborator, 0, len(sortedLogins))
	for _, login := range sortedLogins {
		collaborators = append(collaborators, Collaborator{Login: login})
	}

	return collaborators, nil
}
