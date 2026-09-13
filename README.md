# reviewstats

`reviewstats` is a Scrum retrospective aid for discussing code-review participation and knowledge distribution in one GitHub repository. It surfaces patterns; it is not an individual performance score.

## Install

Requires Go 1.27.

```sh
go install ./cmd/reviewstats
```

## Usage

Pass a GitHub repository explicitly:

```sh
reviewstats --repo acme/frontend
reviewstats --repo acme/frontend --since 7d
reviewstats --repo acme/frontend --since 2026-09-01
```

`--since` accepts a positive number of days such as `7d` or `30d`, or an ISO date in `YYYY-MM-DD` form. The default period is `30d`.

When `--repo` is omitted, `reviewstats` reads `remote.origin.url` from the current Git repository:

```sh
cd path/to/repository
reviewstats
```

V1 supports GitHub.com repositories only.

## Authentication

`reviewstats` resolves a GitHub token in this order:

1. `GH_TOKEN`
2. `GITHUB_TOKEN`
3. An authenticated `gh` CLI via `gh auth token`

The token is used only for the GitHub API request and is never printed.

## Reading the report

### Review participation

A pull request counts once for a reviewer when that person submitted at least one pull-request review during the selected period. Multiple reviews by the same reviewer on the same pull request still count as one. Self-reviews and pending reviews are excluded.

V1 uses GitHub repository collaborators as the available review population so people with zero reviews remain visible. Repository access is only a proxy for Scrum-team membership and should be interpreted in team context.

### Review matrix

Rows are pull-request authors and columns are reviewers. Each cell is the number of unique pull requests by that author reviewed by that reviewer. Repeatedly empty cross-review cells or tightly clustered review pairs can be useful prompts for discussing possible knowledge silos.

The report is descriptive: its counts are discussion signals, not individual performance scores.

## Exit codes

`reviewstats` exits with `0` on success, `2` for invalid input or configuration, and `1` for runtime or GitHub API failures.
