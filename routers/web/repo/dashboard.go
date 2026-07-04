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
	tplDashboard templates.TplName = "repo/dashboard"
)

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
	// Default: 0 = all time. Set period=168 for legacy 1-week behavior.
	periodHours := 0
	if p := ctx.FormString("period"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
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

	canReadCode := ctx.Repo.CanRead(unit.TypeCode)
	if canReadCode {
		branchExist, _ := git_model.IsBranchExist(ctx, ctx.Repo.Repository.ID, ctx.Repo.Repository.DefaultBranch)
		if !branchExist {
			ctx.Data["NotFoundPrompt"] = ctx.Tr("repo.branch.default_branch_not_exist", ctx.Repo.Repository.DefaultBranch)
			ctx.NotFound(nil)
			return
		}
	}

	var err error
	ctx.Data["Activity"], err = activities_model.GetActivityStats(ctx, ctx.Repo.Repository, timeFrom,
		ctx.Repo.CanRead(unit.TypeReleases),
		ctx.Repo.CanRead(unit.TypeIssues),
		ctx.Repo.CanRead(unit.TypePullRequests),
		canReadCode,
	)
	if err != nil {
		ctx.ServerError("GetActivityStats", err)
		return
	}

	if ctx.PageData["repoActivityTopAuthors"], err = activities_model.GetActivityStatsTopAuthors(ctx, ctx.Repo.Repository, timeFrom, 10); err != nil {
		ctx.ServerError("GetActivityStatsTopAuthors", err)
		return
	}

	ctx.HTML(http.StatusOK, tplDashboard)
}
