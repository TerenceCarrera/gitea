// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package dependencies

import (
	"context"
	"runtime/pprof"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/process"
	"code.gitea.io/gitea/modules/queue"
	"code.gitea.io/gitea/modules/setting"
	notify_service "code.gitea.io/gitea/services/notify"
)

var scanQueue *queue.WorkerPoolQueue[*ScanTask]

type ScanTask struct {
	RepoID int64
}

func Init(ctx context.Context) error {
	if !setting.DependencyChecker.Enabled {
		return nil
	}

	ctx, cancel, finished := process.GetManager().AddTypedContext(context.Background(), "Service: DependencyChecker", process.SystemProcessType, false)

	graceful.GetManager().RunAtTerminate(func() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		cancel()
		finished()
	})

	handler := func(items ...*ScanTask) (unhandled []*ScanTask) {
		for _, task := range items {
			if err := scan(ctx, task.RepoID); err != nil {
				log.Error("Dependency scanner: error scanning repo %d: %v", task.RepoID, err)
			}
		}
		return nil
	}

	scanQueue = queue.CreateUniqueQueue(ctx, "dependency_scanner", handler)
	if scanQueue == nil {
		log.Fatal("Unable to create dependency scanner queue")
	}

	notify_service.RegisterNotifier(NewNotifier())

	go func() {
		pprof.SetGoroutineLabels(ctx)
		graceful.GetManager().RunWithCancel(scanQueue)
	}()

	// Populate existing repos on first run
	go graceful.GetManager().RunWithShutdownContext(populateRepos)
	return nil
}

func ScheduleScan(repo *repo_model.Repository) {
	if !setting.DependencyChecker.Enabled {
		return
	}
	if err := scanQueue.Push(&ScanTask{RepoID: repo.ID}); err != nil {
		log.Error("Dependency scan push failed for repo %d: %v", repo.ID, err)
	}
}

func populateRepos(ctx context.Context) {
	log.Info("Populating dependency scanner with existing repositories")

	exist, err := db.IsTableNotEmpty("repository")
	if err != nil {
		log.Error("System error: %v", err)
		return
	} else if !exist {
		return
	}

	var maxRepoID int64
	if maxRepoID, err = db.GetMaxID("repository"); err != nil {
		log.Error("System error: %v", err)
		return
	}

	for maxRepoID > 0 {
		select {
		case <-ctx.Done():
			log.Info("Dependency scanner population shutdown before completion")
			return
		default:
		}

		ids, err := repo_model.GetUnindexedRepos(ctx, repo_model.RepoIndexerTypeCode, maxRepoID, 0, 50)
		if err != nil {
			log.Error("populateRepos: %v", err)
			return
		}
		if len(ids) == 0 {
			break
		}

		for _, id := range ids {
			select {
			case <-ctx.Done():
				log.Info("Dependency scanner population shutdown before completion")
				return
			default:
			}
			if err := scanQueue.Push(&ScanTask{RepoID: id}); err != nil {
				log.Error("scanQueue.Push: %v", err)
				return
			}
			maxRepoID = id - 1
		}
	}
	log.Info("Done populating dependency scanner with existing repositories")
}
