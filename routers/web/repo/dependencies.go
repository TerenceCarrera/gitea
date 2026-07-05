// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"fmt"
	"net/http"
	"strings"
	"text/template"

	deps_model "code.gitea.io/gitea/models/dependencies"
	issues_model "code.gitea.io/gitea/models/issues"
	unit_model "code.gitea.io/gitea/models/unit"
	dep_service "code.gitea.io/gitea/services/dependencies"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/services/context"
)

const (
	tplDependencies templates.TplName = "repo/dependencies/list"
)

// Dependencies renders the repository dependency list page
func Dependencies(ctx *context.Context) {
	ctx.Data["PageIsDependencies"] = true
	ctx.Data["CanWriteDependencies"] = ctx.Repo.Permission.CanWrite(unit_model.TypeDependencies)

	// Auto-scan if the default branch HEAD has changed since last scan
	repo := ctx.Repo.Repository
	if !repo.IsEmpty {
		status, err := deps_model.GetScanStatus(ctx, repo.ID)
		if err != nil {
			log.Error("GetScanStatus: %v", err)
		} else {
			gitRepo, err := gitrepo.OpenRepository(ctx, repo)
			if err == nil {
				commit, err := gitRepo.GetBranchCommit(repo.DefaultBranch)
				if err == nil {
					headSHA := commit.ID.String()
					if status == nil || status.LastCommitSHA != headSHA {
						if err := dep_service.ScanRepository(ctx, repo.ID); err != nil {
							log.Error("Dependency scanner auto-scan failed for repo %d: %v", repo.ID, err)
						}
					}
				}
				gitRepo.Close()
			} else {
				log.Error("Failed to open git repo %d: %v", repo.ID, err)
			}
		}
	}

	deps, err := deps_model.GetDependenciesByRepoGrouped(ctx, repo.ID)
	if err != nil {
		ctx.ServerError("GetDependenciesByRepoGrouped", err)
		return
	}

	ctx.Data["DependenciesByEcosystem"] = deps

	// Build a map of dep name → version for issue body rendering
	depVersionByName := make(map[string]string)
	for _, depList := range deps {
		for _, d := range depList {
			depVersionByName[d.Name] = d.Version
		}
	}

	if setting.DependencyChecker.VulnerabilityCheck {
		vulnsByDep, err := deps_model.GetVulnerabilitiesByRepoGrouped(ctx, repo.ID)
		if err != nil {
			ctx.ServerError("GetVulnerabilitiesByRepoGrouped", err)
			return
		}
		ctx.Data["VulnerabilitiesByDep"] = vulnsByDep

		// Render issue body templates for each vulnerability
		issueBodyTmpl, err := parseIssueBodyTemplate()
		if err != nil {
			log.Error("Failed to load issue body template: %v", err)
		} else {
			issueBodies := make(map[string]string)
			for depName, vulns := range vulnsByDep {
				for _, v := range vulns {
					key := depName + "|" + v.SourceID
					body, err := renderIssueBody(issueBodyTmpl, issueBodyData{
						AppName:      setting.AppName,
						SourceID:     v.SourceID,
						DepName:      depName,
						DepVersion:   depVersionByName[depName],
						Severity:     v.Severity,
						Title:        v.Title,
						FixedVersion: v.FixedVersion,
						SourceURL:    v.SourceURL,
					})
					if err != nil {
						log.Error("Failed to render issue body for %s: %v", v.SourceID, err)
						continue
					}
					issueBodies[key] = body
				}
			}
			ctx.Data["IssueBodies"] = issueBodies
		}
	}

	if _, ok := ctx.Data["IssueBodies"]; !ok {
		ctx.Data["IssueBodies"] = make(map[string]string)
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

// DependenciesCreateIssue creates an issue for a vulnerability and records the link
func DependenciesCreateIssue(ctx *context.Context) {
	if !setting.DependencyChecker.VulnerabilityCheck {
		ctx.Status(http.StatusNotFound)
		return
	}

	if !ctx.Repo.Permission.CanWrite(unit_model.TypeIssues) {
		ctx.Status(http.StatusForbidden)
		return
	}

	vulnID := ctx.FormInt64("vuln_id")
	if vulnID == 0 {
		ctx.Status(http.StatusBadRequest)
		return
	}

	vuln, err := deps_model.GetVulnerabilityByID(ctx, vulnID)
	if err != nil {
		ctx.ServerError("GetVulnerabilityByID", err)
		return
	}

	if vuln.RepoID != ctx.Repo.Repository.ID {
		ctx.Status(http.StatusNotFound)
		return
	}

	if vuln.IssueID > 0 {
		ctx.Redirect(fmt.Sprintf("%s/issues/%d", ctx.Repo.Repository.Link(), vuln.IssueID))
		return
	}

	dep, err := deps_model.GetDependencyByID(ctx, vuln.DependencyID)
	if err != nil {
		ctx.ServerError("GetDependencyByID", err)
		return
	}

	issueBodyTmpl, err := parseIssueBodyTemplate()
	if err != nil {
		ctx.ServerError("parseIssueBodyTemplate", err)
		return
	}

	body, err := renderIssueBody(issueBodyTmpl, issueBodyData{
		AppName:      setting.AppName,
		SourceID:     vuln.SourceID,
		DepName:      dep.Name,
		DepVersion:   dep.Version,
		Severity:     vuln.Severity,
		Title:        vuln.Title,
		FixedVersion: vuln.FixedVersion,
		SourceURL:    vuln.SourceURL,
	})
	if err != nil {
		ctx.ServerError("renderIssueBody", err)
		return
	}

	title := fmt.Sprintf("[Vuln] %s in %s", vuln.SourceID, dep.Name)

	issue := &issues_model.Issue{
		RepoID:   ctx.Repo.Repository.ID,
		Title:    title,
		Content:  body,
		PosterID: ctx.Doer.ID,
		Poster:   ctx.Doer,
	}

	if err := issues_model.NewIssue(ctx, ctx.Repo.Repository, issue, nil, nil); err != nil {
		ctx.ServerError("NewIssue", err)
		return
	}

	if err := deps_model.UpdateVulnerabilityIssueID(ctx, vuln.ID, issue.Index); err != nil {
		log.Error("UpdateVulnerabilityIssueID: %v", err)
	}

	ctx.Redirect(fmt.Sprintf("%s/issues/%d", ctx.Repo.Repository.Link(), issue.Index))
}

type issueBodyData struct {
	AppName      string
	SourceID     string
	DepName      string
	DepVersion   string
	Severity     string
	Title        string
	FixedVersion string
	SourceURL    string
}

const issueBodyMD = `## Vulnerability Report

| Field | Details |
|-------|---------|
| **Advisory** | {{.SourceID}} |
| **Package** | {{.DepName}} |
| **Installed Version** | {{.DepVersion}} |
| **Severity** | {{.Severity}} |
| **Fixed Version** | {{.FixedVersion}} |

### Description

{{.Title}}

### References

{{.SourceURL}}

---

_Automated dependency vulnerability scan from {{.AppName}}_`

func parseIssueBodyTemplate() (*template.Template, error) {
	return template.New("issue_body").Parse(strings.TrimSpace(issueBodyMD))
}

func renderIssueBody(tmpl *template.Template, data issueBodyData) (string, error) {
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
