// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package activities

import (
	"context"
	"fmt"
	"sort"
	"time"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"

	"xorm.io/builder"
	"xorm.io/xorm"
)

// ActivityAuthorData represents statistical git commit count data
type ActivityAuthorData struct {
	Name       string `json:"name"`
	Login      string `json:"login"`
	AvatarLink string `json:"avatar_link"`
	HomeLink   string `json:"home_link"`
	Commits    int64  `json:"commits"`
}

// ActivityStats represents issue and pull request information.
type ActivityStats struct {
	OpenedPRs                   issues_model.PullRequestList
	OpenedPRAuthorCount         int64
	MergedPRs                   issues_model.PullRequestList
	MergedPRAuthorCount         int64
	ActiveIssues                issues_model.IssueList
	OpenedIssues                issues_model.IssueList
	OpenedIssueAuthorCount      int64
	ClosedIssues                issues_model.IssueList
	ClosedIssueAuthorCount      int64
	UnresolvedIssues            issues_model.IssueList
	PublishedReleases           []*repo_model.Release
	PublishedReleaseAuthorCount int64
	Code                        *git.CodeActivityStats

	// total counts for pagination (set by Fill methods)
	mergedPRsTotal    int64
	openedPRsTotal    int64
	closedIssuesTotal int64
	openedIssuesTotal int64
	publishedReleasesTotal int64
}

// GetActivityStats return stats for repository at given time range.
// Pass pageSize > 0 to enable per-section SQL-level pagination.
// When pageSize > 0, mergedPRsPage/openedPRsPage/closedIssuesPage/openedIssuesPage/releasesPage specify the page number (1-based).
func GetActivityStats(ctx context.Context, repo *repo_model.Repository, timeFrom time.Time, releases, issues, prs, code bool,
	mergedPRsPage, openedPRsPage, closedIssuesPage, openedIssuesPage, releasesPage, pageSize int) (*ActivityStats, error) {
	stats := &ActivityStats{Code: &git.CodeActivityStats{}}
	if releases {
		if err := stats.FillReleases(ctx, repo.ID, timeFrom, releasesPage, pageSize); err != nil {
			return nil, fmt.Errorf("FillReleases: %w", err)
		}
	}
	if prs {
		if err := stats.FillPullRequests(ctx, repo.ID, timeFrom, mergedPRsPage, openedPRsPage, pageSize); err != nil {
			return nil, fmt.Errorf("FillPullRequests: %w", err)
		}
	}
	if issues {
		if err := stats.FillIssues(ctx, repo.ID, timeFrom, closedIssuesPage, openedIssuesPage, pageSize); err != nil {
			return nil, fmt.Errorf("FillIssues: %w", err)
		}
	}
	if err := stats.FillUnresolvedIssues(ctx, repo.ID, timeFrom, issues, prs, 1, pageSize); err != nil {
		return nil, fmt.Errorf("FillUnresolvedIssues: %w", err)
	}
	if code {
		gitRepo, closer, err := gitrepo.RepositoryFromContextOrOpen(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("OpenRepository: %w", err)
		}
		defer closer.Close()

		code, err := gitRepo.GetCodeActivityStats(timeFrom, repo.DefaultBranch)
		if err != nil {
			return nil, fmt.Errorf("FillFromGit: %w", err)
		}
		stats.Code = code
	}
	return stats, nil
}

// GetActivityStatsTopAuthors returns top author stats for git commits for all branches
func GetActivityStatsTopAuthors(ctx context.Context, repo *repo_model.Repository, timeFrom time.Time, count int) ([]*ActivityAuthorData, error) {
	gitRepo, closer, err := gitrepo.RepositoryFromContextOrOpen(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("OpenRepository: %w", err)
	}
	defer closer.Close()

	code, err := gitRepo.GetCodeActivityStats(timeFrom, "")
	if err != nil {
		return nil, fmt.Errorf("FillFromGit: %w", err)
	}
	if code.Authors == nil {
		return nil, nil
	}
	users := make(map[int64]*ActivityAuthorData)
	var unknownUserID int64
	unknownUserAvatarLink := user_model.NewGhostUser().AvatarLink(ctx)
	for _, v := range code.Authors {
		if len(v.Email) == 0 {
			continue
		}
		u, err := user_model.GetUserByEmail(ctx, v.Email)
		if u == nil || user_model.IsErrUserNotExist(err) {
			unknownUserID--
			users[unknownUserID] = &ActivityAuthorData{
				Name:       v.Name,
				AvatarLink: unknownUserAvatarLink,
				Commits:    v.Commits,
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if user, ok := users[u.ID]; !ok {
			users[u.ID] = &ActivityAuthorData{
				Name:       u.DisplayName(),
				Login:      u.LowerName,
				AvatarLink: u.AvatarLink(ctx),
				HomeLink:   u.HomeLink(),
				Commits:    v.Commits,
			}
		} else {
			user.Commits += v.Commits
		}
	}
	v := make([]*ActivityAuthorData, 0, len(users))
	for _, u := range users {
		v = append(v, u)
	}

	sort.Slice(v, func(i, j int) bool {
		return v[i].Commits > v[j].Commits
	})

	cnt := min(count, len(v))

	return v[:cnt], nil
}

// ActivePRCount returns total active pull request count
func (stats *ActivityStats) ActivePRCount() int {
	return stats.OpenedPRCount() + stats.MergedPRCount()
}

// OpenedPRCount returns opened pull request count
func (stats *ActivityStats) OpenedPRCount() int {
	return int(stats.openedPRsTotal)
}


// OpenedPRPerc returns opened pull request percents from total active
func (stats *ActivityStats) OpenedPRPerc() int {
	return int(float32(stats.OpenedPRCount()) / float32(stats.ActivePRCount()) * 100.0)
}

// MergedPRCount returns merged pull request count
func (stats *ActivityStats) MergedPRCount() int {
	return int(stats.mergedPRsTotal)
}

// MergedPRPerc returns merged pull request percent from total active
func (stats *ActivityStats) MergedPRPerc() int {
	return int(float32(stats.MergedPRCount()) / float32(stats.ActivePRCount()) * 100.0)
}

// ActiveIssueCount returns total active issue count
func (stats *ActivityStats) ActiveIssueCount() int {
	return len(stats.ActiveIssues)
}

// OpenedIssueCount returns open issue count
func (stats *ActivityStats) OpenedIssueCount() int {
	return int(stats.openedIssuesTotal)
}

// OpenedIssuePerc returns open issue count percent from total active
func (stats *ActivityStats) OpenedIssuePerc() int {
	return int(float32(stats.OpenedIssueCount()) / float32(stats.ActiveIssueCount()) * 100.0)
}

// ClosedIssueCount returns closed issue count
func (stats *ActivityStats) ClosedIssueCount() int {
	return int(stats.closedIssuesTotal)
}

// ClosedIssuePerc returns closed issue count percent from total active
func (stats *ActivityStats) ClosedIssuePerc() int {
	return int(float32(stats.ClosedIssueCount()) / float32(stats.ActiveIssueCount()) * 100.0)
}

// UnresolvedIssueCount returns unresolved issue and pull request count
func (stats *ActivityStats) UnresolvedIssueCount() int {
	return len(stats.UnresolvedIssues)
}

// PublishedReleaseCount returns published release count
func (stats *ActivityStats) PublishedReleaseCount() int {
	return int(stats.publishedReleasesTotal)
}

// FillPullRequests returns pull request information for activity page
// If pageSize > 0, SQL-level pagination is applied per section.
func (stats *ActivityStats) FillPullRequests(ctx context.Context, repoID int64, fromTime time.Time, mergedPRsPage, openedPRsPage, pageSize int) error {
	var err error
	var count int64

	// Merged pull requests
	mergedSess := pullRequestsForActivityStatement(ctx, repoID, fromTime, true)
	mergedSess.OrderBy("pull_request.merged_unix DESC")
	if pageSize > 0 {
		totalSess := pullRequestsForActivityStatement(ctx, repoID, fromTime, true)
		stats.mergedPRsTotal, err = totalSess.Count(&issues_model.PullRequest{})
		if err != nil {
			return err
		}
		mergedSess.Limit(pageSize, (mergedPRsPage-1)*pageSize)
	}
	stats.MergedPRs = make(issues_model.PullRequestList, 0)
	if err = mergedSess.Find(&stats.MergedPRs); err != nil {
		return err
	}
	if err = stats.MergedPRs.LoadAttributes(ctx); err != nil {
		return err
	}

	// Merged pull request authors
	mergedAuthSess := pullRequestsForActivityStatement(ctx, repoID, fromTime, true)
	if _, err = mergedAuthSess.Select("count(distinct issue.poster_id) as `count`").Table("pull_request").Get(&count); err != nil {
		return err
	}
	stats.MergedPRAuthorCount = count

	if pageSize == 0 {
		stats.mergedPRsTotal = int64(len(stats.MergedPRs))
	}

	// Opened pull requests
	openedSess := pullRequestsForActivityStatement(ctx, repoID, fromTime, false)
	openedSess.OrderBy("issue.created_unix ASC")
	if pageSize > 0 {
		totalSess := pullRequestsForActivityStatement(ctx, repoID, fromTime, false)
		stats.openedPRsTotal, err = totalSess.Count(&issues_model.PullRequest{})
		if err != nil {
			return err
		}
		openedSess.Limit(pageSize, (openedPRsPage-1)*pageSize)
	}
	stats.OpenedPRs = make(issues_model.PullRequestList, 0)
	if err = openedSess.Find(&stats.OpenedPRs); err != nil {
		return err
	}
	if err = stats.OpenedPRs.LoadAttributes(ctx); err != nil {
		return err
	}

	// Opened pull request authors
	openedAuthSess := pullRequestsForActivityStatement(ctx, repoID, fromTime, false)
	if _, err = openedAuthSess.Select("count(distinct issue.poster_id) as `count`").Table("pull_request").Get(&count); err != nil {
		return err
	}
	stats.OpenedPRAuthorCount = count

	if pageSize == 0 {
		stats.openedPRsTotal = int64(len(stats.OpenedPRs))
	}

	return nil
}

func pullRequestsForActivityStatement(ctx context.Context, repoID int64, fromTime time.Time, merged bool) *xorm.Session {
	sess := db.GetEngine(ctx).Where("pull_request.base_repo_id=?", repoID).
		Join("INNER", "issue", "pull_request.issue_id = issue.id")

	if merged {
		sess.And("pull_request.has_merged = ?", true)
		sess.And("pull_request.merged_unix >= ?", fromTime.Unix())
	} else {
		sess.And("issue.is_closed = ?", false)
		sess.And("issue.created_unix >= ?", fromTime.Unix())
	}

	return sess
}

// FillIssues returns issue information for activity page
// If pageSize > 0, SQL-level pagination is applied per section.
func (stats *ActivityStats) FillIssues(ctx context.Context, repoID int64, fromTime time.Time, closedIssuesPage, openedIssuesPage, pageSize int) error {
	var err error
	var count int64

	// Closed issues
	closedSess := issuesForActivityStatement(ctx, repoID, fromTime, true, false)
	closedSess.OrderBy("issue.closed_unix DESC")
	if pageSize > 0 {
		totalSess := issuesForActivityStatement(ctx, repoID, fromTime, true, false)
		stats.closedIssuesTotal, err = totalSess.Count(&issues_model.Issue{})
		if err != nil {
			return err
		}
		closedSess.Limit(pageSize, (closedIssuesPage-1)*pageSize)
	}
	stats.ClosedIssues = make(issues_model.IssueList, 0)
	if err = closedSess.Find(&stats.ClosedIssues); err != nil {
		return err
	}

	// Closed issue authors
	closedAuthSess := issuesForActivityStatement(ctx, repoID, fromTime, true, false)
	if _, err = closedAuthSess.Select("count(distinct issue.poster_id) as `count`").Table("issue").Get(&count); err != nil {
		return err
	}
	stats.ClosedIssueAuthorCount = count

	if pageSize == 0 {
		stats.closedIssuesTotal = int64(len(stats.ClosedIssues))
	}

	// New issues
	openedSess := newlyCreatedIssues(ctx, repoID, fromTime)
	openedSess.OrderBy("issue.created_unix ASC")
	if pageSize > 0 {
		totalSess := newlyCreatedIssues(ctx, repoID, fromTime)
		stats.openedIssuesTotal, err = totalSess.Count(&issues_model.Issue{})
		if err != nil {
			return err
		}
		openedSess.Limit(pageSize, (openedIssuesPage-1)*pageSize)
	}
	stats.OpenedIssues = make(issues_model.IssueList, 0)
	if err = openedSess.Find(&stats.OpenedIssues); err != nil {
		return err
	}

	// Active issues (always all, for chart/overview data)
	sess := activeIssues(ctx, repoID, fromTime)
	sess.OrderBy("issue.created_unix ASC")
	stats.ActiveIssues = make(issues_model.IssueList, 0)
	if err = sess.Find(&stats.ActiveIssues); err != nil {
		return err
	}

	// Opened issue authors
	openedAuthSess := issuesForActivityStatement(ctx, repoID, fromTime, false, false)
	if _, err = openedAuthSess.Select("count(distinct issue.poster_id) as `count`").Table("issue").Get(&count); err != nil {
		return err
	}
	stats.OpenedIssueAuthorCount = count

	if pageSize == 0 {
		stats.openedIssuesTotal = int64(len(stats.OpenedIssues))
	}

	return nil
}

// FillUnresolvedIssues returns unresolved issue and pull request information for activity page
func (stats *ActivityStats) FillUnresolvedIssues(ctx context.Context, repoID int64, fromTime time.Time, issues, prs bool, page, pageSize int) error {
	// Check if we need to select anything
	if !issues && !prs {
		return nil
	}
	sess := issuesForActivityStatement(ctx, repoID, fromTime, false, true)
	if !issues || !prs {
		sess.And("issue.is_pull = ?", prs)
	}
	sess.OrderBy("issue.updated_unix DESC")
	if pageSize > 0 {
		sess.Limit(pageSize, (page-1)*pageSize)
	}
	stats.UnresolvedIssues = make(issues_model.IssueList, 0)
	return sess.Find(&stats.UnresolvedIssues)
}

func newlyCreatedIssues(ctx context.Context, repoID int64, fromTime time.Time) *xorm.Session {
	sess := db.GetEngine(ctx).Where("issue.repo_id = ?", repoID).
		And("issue.is_pull = ?", false).                // Retain the is_pull check to exclude pull requests
		And("issue.created_unix >= ?", fromTime.Unix()) // Include all issues created after fromTime

	return sess
}

func activeIssues(ctx context.Context, repoID int64, fromTime time.Time) *xorm.Session {
	sess := db.GetEngine(ctx).Where("issue.repo_id = ?", repoID).
		And("issue.is_pull = ?", false).
		And(builder.Or(
			builder.Gte{"issue.created_unix": fromTime.Unix()},
			builder.Gte{"issue.closed_unix": fromTime.Unix()},
		))

	return sess
}

func issuesForActivityStatement(ctx context.Context, repoID int64, fromTime time.Time, closed, unresolved bool) *xorm.Session {
	sess := db.GetEngine(ctx).Where("issue.repo_id = ?", repoID).
		And("issue.is_closed = ?", closed)

	if !unresolved {
		sess.And("issue.is_pull = ?", false)
		if closed {
			sess.And("issue.closed_unix >= ?", fromTime.Unix())
		} else {
			sess.And("issue.created_unix >= ?", fromTime.Unix())
		}
	} else {
		sess.And("issue.created_unix < ?", fromTime.Unix())
		sess.And("issue.updated_unix >= ?", fromTime.Unix())
	}

	return sess
}

// FillReleases returns release information for activity page
// If pageSize > 0, SQL-level pagination is applied.
func (stats *ActivityStats) FillReleases(ctx context.Context, repoID int64, fromTime time.Time, page, pageSize int) error {
	var err error
	var count int64

	// Published releases list
	dataSess := releasesForActivityStatement(ctx, repoID, fromTime)
	dataSess.OrderBy("`release`.created_unix DESC")
	if pageSize > 0 {
		totalSess := releasesForActivityStatement(ctx, repoID, fromTime)
		stats.publishedReleasesTotal, err = totalSess.Count(&repo_model.Release{})
		if err != nil {
			return err
		}
		dataSess.Limit(pageSize, (page-1)*pageSize)
	}
	stats.PublishedReleases = make([]*repo_model.Release, 0)
	if err = dataSess.Find(&stats.PublishedReleases); err != nil {
		return err
	}

	// Published releases authors
	authSess := releasesForActivityStatement(ctx, repoID, fromTime)
	if _, err = authSess.Select("count(distinct `release`.publisher_id) as `count`").Table("release").Get(&count); err != nil {
		return err
	}
	stats.PublishedReleaseAuthorCount = count

	if pageSize == 0 {
		stats.publishedReleasesTotal = int64(len(stats.PublishedReleases))
	}

	return nil
}

func releasesForActivityStatement(ctx context.Context, repoID int64, fromTime time.Time) *xorm.Session {
	return db.GetEngine(ctx).Where("`release`.repo_id = ?", repoID).
		And("`release`.is_draft = ?", false).
		And("`release`.created_unix >= ?", fromTime.Unix())
}
