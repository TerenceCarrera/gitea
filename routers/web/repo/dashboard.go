// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"
	"strconv"
	"time"

	activities_model "code.gitea.io/gitea/models/activities"
	git_model "code.gitea.io/gitea/models/git"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/services/context"
)

const (
	tplDashboard             templates.TplName = "repo/dashboard"
	dashboardPageSize                          = 10
)

func pageParam(ctx *context.Context, name string) int {
	p := ctx.FormInt(name)
	if p < 1 {
		return 1
	}
	return p
}

// Dashboard renders the repository activity dashboard page
func Dashboard(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.dashboard")
	ctx.Data["PageIsDashboard"] = true

	// Redirect to code view if dashboard is disabled
	if !ctx.Repo.Repository.EnableDashboard {
		ctx.Redirect(ctx.Repo.RepoLink + "/code")
		return
	}

	// Allow configurable time period via query parameter (in hours).
	// Default: 168 (1 week). Set period=0 for all time.
	periodHours := 168
	if p := ctx.FormString("period"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v >= 0 {
			periodHours = v
		}
	}
	ctx.Data["DashboardPeriod"] = periodHours

	timeUntil := time.Now()
	var timeFrom time.Time
	if periodHours > 0 {
		timeFrom = timeUntil.Add(-time.Duration(periodHours) * time.Hour)
	} else {
		// All time: use epoch
		timeFrom = time.Unix(0, 0)
	}

	// Period notice for the template
	var periodNotice string
	if periodHours > 0 {
		days := periodHours / 24
		hours := periodHours % 24
		if days > 0 && hours > 0 {
			periodNotice = ctx.Locale.TrString("repo.dashboard.period_notice_days_hours", days, hours)
		} else if days > 0 {
			periodNotice = ctx.Locale.TrString("repo.dashboard.period_notice_days", days)
		} else {
			periodNotice = ctx.Locale.TrString("repo.dashboard.period_notice_hours", hours)
		}
	} else {
		periodNotice = ctx.Locale.TrString("repo.dashboard.period_notice_all")
	}
	ctx.Data["DashboardPeriodNotice"] = periodNotice

	canReadCode := ctx.Repo.CanRead(unit.TypeCode)
	if canReadCode {
		branchExist, _ := git_model.IsBranchExist(ctx, ctx.Repo.Repository.ID, ctx.Repo.Repository.DefaultBranch)
		if !branchExist {
			ctx.Data["NotFoundPrompt"] = ctx.Tr("repo.branch.default_branch_not_exist", ctx.Repo.Repository.DefaultBranch)
			ctx.NotFound(nil)
			return
		}
	}

	// Parse page params for each section
	mergedPRsPage := pageParam(ctx, "merged_prs_page")
	openedPRsPage := pageParam(ctx, "opened_prs_page")
	closedIssuesPage := pageParam(ctx, "closed_issues_page")
	openedIssuesPage := pageParam(ctx, "opened_issues_page")
	releasesPage := pageParam(ctx, "releases_page")

	var err error
	ctx.Data["Activity"], err = activities_model.GetActivityStats(ctx, ctx.Repo.Repository, timeFrom,
		ctx.Repo.CanRead(unit.TypeReleases),
		ctx.Repo.CanRead(unit.TypeIssues),
		ctx.Repo.CanRead(unit.TypePullRequests),
		canReadCode,
		mergedPRsPage, openedPRsPage, closedIssuesPage, openedIssuesPage, releasesPage, dashboardPageSize,
	)
	if err != nil {
		ctx.ServerError("GetActivityStats", err)
		return
	}

	if ctx.PageData["repoActivityTopAuthors"], err = activities_model.GetActivityStatsTopAuthors(ctx, ctx.Repo.Repository, timeFrom, 10); err != nil {
		ctx.ServerError("GetActivityStatsTopAuthors", err)
		return
	}

	activity := ctx.Data["Activity"].(*activities_model.ActivityStats)

	// Data is already SQL-paginated (only the current page was fetched).
	// Set paginated lists and pagination controls.
	makePager := func(total int64, pageParam string, page int) *context.Pagination {
		p := context.NewPagination(total, dashboardPageSize, page, 5)
		p.PageParamName = pageParam
		p.AddParamFromRequest(ctx.Req)
		return p
	}

	ctx.Data["ReleasesPageList"] = activity.PublishedReleases
	ctx.Data["ReleasesPaginator"] = makePager(int64(activity.PublishedReleaseCount()), "releases_page", releasesPage)

	ctx.Data["MergedPRsPageList"] = activity.MergedPRs
	ctx.Data["MergedPRsPaginator"] = makePager(int64(activity.MergedPRCount()), "merged_prs_page", mergedPRsPage)

	ctx.Data["OpenedPRsPageList"] = activity.OpenedPRs
	ctx.Data["OpenedPRsPaginator"] = makePager(int64(activity.OpenedPRCount()), "opened_prs_page", openedPRsPage)

	ctx.Data["ClosedIssuesPageList"] = activity.ClosedIssues
	ctx.Data["ClosedIssuesPaginator"] = makePager(int64(activity.ClosedIssueCount()), "closed_issues_page", closedIssuesPage)

	ctx.Data["OpenedIssuesPageList"] = activity.OpenedIssues
	ctx.Data["OpenedIssuesPaginator"] = makePager(int64(activity.OpenedIssueCount()), "opened_issues_page", openedIssuesPage)

	ctx.HTML(http.StatusOK, tplDashboard)
}
