// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	deps_model "gitea.dev/models/dependencies"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/templates"
	"gitea.dev/services/context"
)

const (
	tplDependencies templates.TplName = "repo/dependencies/list"
)

// Dependencies renders the repository dependency list page
func Dependencies(ctx *context.Context) {
	ctx.Data["PageIsDependencies"] = true

	deps, err := deps_model.GetDependenciesByRepoGrouped(ctx, ctx.Repo.Repository.ID)
	if err != nil {
		ctx.ServerError("GetDependenciesByRepoGrouped", err)
		return
	}

	ctx.Data["DependenciesByEcosystem"] = deps

	if setting.DependencyChecker.VulnerabilityCheck {
		vulnsByDep, err := deps_model.GetVulnerabilitiesByRepoGrouped(ctx, ctx.Repo.Repository.ID)
		if err != nil {
			ctx.ServerError("GetVulnerabilitiesByRepoGrouped", err)
			return
		}
		ctx.Data["VulnerabilitiesByDep"] = vulnsByDep
	}

	ctx.Data["VulnerabilityCheckEnabled"] = setting.DependencyChecker.VulnerabilityCheck
	ctx.HTML(200, tplDependencies)
}
