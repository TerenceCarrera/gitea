// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	deps_model "gitea.dev/models/dependencies"
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
	ctx.HTML(200, tplDependencies)
}
