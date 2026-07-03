// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package dependencies

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	"gitea.dev/modules/timeutil"
)

// ScanStatus tracks the last scan state for a repository
type ScanStatus struct {
	ID            int64              `xorm:"pk autoincr"`
	RepoID        int64              `xorm:"UNIQUE NOT NULL"`
	LastCommitSHA string             `xorm:"VARCHAR(64) NOT NULL"`
	CreatedUnix   timeutil.TimeStamp `xorm:"created"`
	UpdatedUnix   timeutil.TimeStamp `xorm:"updated"`
}

func init() {
	db.RegisterModel(new(ScanStatus))
}

// GetScanStatus returns the scan status for a repo
func GetScanStatus(ctx context.Context, repoID int64) (*ScanStatus, error) {
	var status ScanStatus
	has, err := db.GetEngine(ctx).Where("repo_id = ?", repoID).Get(&status)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return &status, nil
}

// UpsertScanStatus creates or updates the scan status for a repo
func UpsertScanStatus(ctx context.Context, repoID int64, commitSHA string) error {
	existed, err := db.GetEngine(ctx).Where("repo_id = ?", repoID).Exist(&ScanStatus{})
	if err != nil {
		return err
	}

	if existed {
		_, err = db.GetEngine(ctx).Where("repo_id = ?", repoID).Update(&ScanStatus{
			LastCommitSHA: commitSHA,
		})
	} else {
		_, err = db.GetEngine(ctx).Insert(&ScanStatus{
			RepoID:        repoID,
			LastCommitSHA: commitSHA,
		})
	}
	return err
}

// DeleteScanStatus removes the scan status for a repo
func DeleteScanStatus(ctx context.Context, repoID int64) error {
	_, err := db.GetEngine(ctx).Where("repo_id = ?", repoID).Delete(&ScanStatus{})
	return err
}

// String returns a log-friendly representation
func (s *ScanStatus) String() string {
	return fmt.Sprintf("ScanStatus{RepoID: %d, LastCommitSHA: %s}", s.RepoID, s.LastCommitSHA)
}
