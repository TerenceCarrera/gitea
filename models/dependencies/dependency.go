// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package dependencies

import (
	"context"
	"fmt"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
)

// ErrDependencyNotExists represents a dependency not found error
type ErrDependencyNotExists struct {
	ID int64
}

func (err ErrDependencyNotExists) Error() string {
	return fmt.Sprintf("dependency does not exist [id: %d]", err.ID)
}

func (err ErrDependencyNotExists) Unwrap() error {
	return util.ErrNotExist
}

// Dependency represents a software dependency found in a repository
type Dependency struct {
	ID          int64              `xorm:"pk autoincr"`
	RepoID      int64              `xorm:"INDEX NOT NULL"`
	CommitSHA   string             `xorm:"VARCHAR(64) NOT NULL"`
	FilePath    string             `xorm:"NOT NULL"`
	Name        string             `xorm:"NOT NULL"`
	Version     string             `xorm:"NOT NULL"`
	Type        string             `xorm:"DEFAULT 'runtime'"`
	Ecosystem   string             `xorm:"VARCHAR(32) NOT NULL"`
	CreatedUnix timeutil.TimeStamp `xorm:"created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"updated"`
}

func init() {
	db.RegisterModel(new(Dependency))
}

// UpsertDependencies replaces all dependencies for a repo with the new set
func UpsertDependencies(ctx context.Context, repoID int64, commitSHA string, deps []Dependency) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		// Delete old dependencies for this repo
		if _, err := db.GetEngine(ctx).Where("repo_id = ?", repoID).Delete(&Dependency{}); err != nil {
			return err
		}

		// Insert new ones
		for i := range deps {
			deps[i].RepoID = repoID
			deps[i].CommitSHA = commitSHA
		}
		if len(deps) > 0 {
			if err := db.Insert(ctx, deps); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetDependenciesByRepo returns all dependencies for a repository
func GetDependenciesByRepo(ctx context.Context, repoID int64) ([]Dependency, error) {
	var deps []Dependency
	err := db.GetEngine(ctx).Where("repo_id = ?", repoID).Find(&deps)
	return deps, err
}

// GetDependenciesByRepoGrouped returns dependencies grouped by ecosystem
func GetDependenciesByRepoGrouped(ctx context.Context, repoID int64) (map[string][]Dependency, error) {
	deps, err := GetDependenciesByRepo(ctx, repoID)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]Dependency)
	for _, dep := range deps {
		result[dep.Ecosystem] = append(result[dep.Ecosystem], dep)
	}
	return result, nil
}

// DeleteDependenciesByRepo removes all dependencies for a repo
func DeleteDependenciesByRepo(ctx context.Context, repoID int64) error {
	_, err := db.GetEngine(ctx).Where("repo_id = ?", repoID).Delete(&Dependency{})
	return err
}
