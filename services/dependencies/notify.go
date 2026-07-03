// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package dependencies

import (
	"context"

	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/repository"
	notify_service "gitea.dev/services/notify"
)

type depNotifier struct {
	notify_service.NullNotifier
}

var _ notify_service.Notifier = &depNotifier{}

func NewNotifier() notify_service.Notifier {
	return &depNotifier{}
}

func (r *depNotifier) PushCommits(ctx context.Context, pusher *user_model.User, repo *repo_model.Repository, opts *repository.PushUpdateOptions, commits *repository.PushCommits) {
	if !opts.RefFullName.IsBranch() {
		return
	}
	if opts.RefFullName.BranchName() == repo.DefaultBranch {
		ScheduleScan(repo)
	}
}

func (r *depNotifier) MigrateRepository(ctx context.Context, doer, u *user_model.User, repo *repo_model.Repository) {
	if !repo.IsEmpty {
		ScheduleScan(repo)
	}
}

func (r *depNotifier) ChangeDefaultBranch(ctx context.Context, repo *repo_model.Repository) {
	if !repo.IsEmpty {
		ScheduleScan(repo)
	}
}

func (r *depNotifier) DeleteRepository(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) {
	// cleanup will happen via the scanner when it detects the repo no longer exists
}
