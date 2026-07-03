// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"

	deps_model "gitea.dev/models/dependencies"
	unit_model "gitea.dev/models/unit"
	dep_service "gitea.dev/services/dependencies"
	"gitea.dev/modules/log"
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
	ctx.Data["CanWriteDependencies"] = ctx.Repo.Permission.CanWrite(unit_model.TypeDependencies)

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

// DependenciesScanVulnerabilities manually triggers a vulnerability scan
func DependenciesScanVulnerabilities(ctx *context.Context) {
	if !setting.DependencyChecker.VulnerabilityCheck {
		ctx.Status(http.StatusNotFound)
		return
	}

	if !ctx.Repo.Permission.CanWrite(unit_model.TypeDependencies) {
		ctx.Status(http.StatusForbidden)
		return
	}

	if err := dep_service.CheckVulnerabilities(ctx, ctx.Repo.Repository.ID); err != nil {
		log.Error("Manual vulnerability scan failed for repo %d: %v", ctx.Repo.Repository.ID, err)
		ctx.ServerError("CheckVulnerabilities", err)
		return
	}

	ctx.Redirect(ctx.Repo.Repository.Link() + "/dependencies")
}
