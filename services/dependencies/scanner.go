// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package dependencies

import (
	"context"
	"path/filepath"
	"strings"

	deps_model "gitea.dev/models/dependencies"
	repo_model "gitea.dev/models/repo"
	unit_model "gitea.dev/models/unit"
	"gitea.dev/modules/dependency/checker"
	"gitea.dev/modules/dependency/parser"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
)

func scan(ctx context.Context, repoID int64) error {
	repo, err := repo_model.GetRepositoryByID(ctx, repoID)
	if repo_model.IsErrRepoNotExist(err) {
		return deps_model.DeleteDependenciesByRepo(ctx, repoID)
	}
	if err != nil {
		return err
	}

	// Skip if dependencies unit is not enabled for this repo
	if !repo.UnitEnabled(ctx, unit_model.TypeDependencies) {
		return deps_model.DeleteDependenciesByRepo(ctx, repoID)
	}

	if repo.IsEmpty {
		return nil
	}

	// Check scan status to avoid unnecessary work
	status, err := deps_model.GetScanStatus(ctx, repoID)
	if err != nil {
		return err
	}

	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return err
	}
	defer gitRepo.Close()

	commit, err := gitRepo.GetBranchCommit(repo.DefaultBranch)
	if err != nil {
		return err
	}

	headSHA := commit.ID.String()

	// If we already scanned this SHA, skip
	if status != nil && status.LastCommitSHA == headSHA {
		return nil
	}

	// Walk the tree recursively looking for dependency files
	entries, err := commit.Tree.ListEntriesRecursiveFast()
	if err != nil {
		return err
	}

	detectableFiles := parser.DetectableFiles()
	detectableSet := make(map[string]bool, len(detectableFiles))
	for _, f := range detectableFiles {
		detectableSet[strings.ToLower(f)] = true
	}

	var allDeps []deps_model.Dependency

	for _, entry := range entries {
		if entry.IsDir() || entry.IsSubModule() {
			continue
		}

		path := entry.Name() // full path for recursive entries
		base := strings.ToLower(filepath.Base(path))

		if !detectableSet[base] {
			continue
		}

		for _, p := range parser.Parsers() {
			if p.Detect(path) {
				blob := entry.Blob()
				content, err := blob.GetBlobContent(setting.Indexer.MaxIndexerFileSize)
				if err != nil {
					log.Error("Failed to read blob %s in repo %d: %v", path, repoID, err)
					continue
				}

				parsedDeps, err := p.Parse(strings.NewReader(content))
				if err != nil {
					log.Debug("Failed to parse %s in repo %d: %v", path, repoID, err)
					continue
				}

				ecosystem := detectEcosystem(path)

				for _, dep := range parsedDeps {
					allDeps = append(allDeps, deps_model.Dependency{
						RepoID:    repoID,
						CommitSHA: headSHA,
						FilePath:  path,
						Name:      dep.Name,
						Version:   dep.Version,
						Type:      dep.Type,
						Ecosystem: ecosystem,
					})
				}
				break
			}
		}
	}

	if len(allDeps) > 0 {
		if err := deps_model.UpsertDependencies(ctx, repoID, headSHA, allDeps); err != nil {
			return err
		}
	}

	// Check for known vulnerabilities if enabled
	if setting.DependencyChecker.VulnerabilityCheck && len(allDeps) > 0 {
		if err := checkVulnerabilities(ctx, repoID); err != nil {
			log.Error("Dependency vulnerability check failed for repo %d: %v", repoID, err)
		}
	}

	return deps_model.UpsertScanStatus(ctx, repoID, headSHA)
}

func checkVulnerabilities(ctx context.Context, repoID int64) error {
	deps, err := deps_model.GetDependenciesByRepo(ctx, repoID)
	if err != nil {
		return err
	}

	var inputs []checker.CheckInput
	for _, dep := range deps {
		inputs = append(inputs, checker.CheckInput{
			Name:         dep.Name,
			Version:      dep.Version,
			Ecosystem:    dep.Ecosystem,
			DependencyID: dep.ID,
		})
	}

	results := checker.CheckVulnerabilities(ctx, inputs)
	if len(results) == 0 {
		return nil
	}

	var vulns []deps_model.Vulnerability
	for _, result := range results {
		for _, v := range result.Vulnerabilities {
			vulns = append(vulns, deps_model.Vulnerability{
				RepoID:       repoID,
				DependencyID: result.DependencyID,
				SourceID:     v.SourceID,
				SourceURL:    v.SourceURL,
				Severity:     v.Severity,
				Title:        v.Title,
				FixedVersion: v.FixedVersion,
			})
		}
	}

	return deps_model.UpsertVulnerabilities(ctx, repoID, vulns)
}

func detectEcosystem(path string) string {
	name := strings.ToLower(filepath.Base(path))
	switch name {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock":
		return "npm"
	case "go.mod", "go.sum":
		return "go"
	case "cargo.toml", "cargo.lock":
		return "cargo"
	case "requirements.txt", "pipfile.lock":
		return "pip"
	default:
		return "other"
	}
}
