// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package checker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/proxy"
	"code.gitea.io/gitea/modules/setting"
)

const osvQueryEndpoint = "https://api.osv.dev/v1/querybatch"

var ecosystemMapping = map[string]string{
	"npm":      "npm",
	"go":       "Go",
	"cargo":    "crates.io",
	"pip":      "PyPI",
	"composer": "Packagist",
	"rubygems": "RubyGems",
	"maven":    "Maven",
	"nuget":    "NuGet",
	"pub":      "Pub",
	"mix":      "Hex",
	"cocoapods": "CocoaPods",
	"conda":    "PyPI",
}

// CheckResult holds vulnerability info for a single dependency
type CheckResult struct {
	DependencyName    string
	DependencyVersion string
	DependencyID      int64
	Ecosystem         string
	Vulnerabilities   []VulnerabilityInfo
}

// VulnerabilityInfo holds details about a specific vulnerability
type VulnerabilityInfo struct {
	SourceID     string // e.g., CVE-2024-1234
	SourceURL    string
	Severity     string // CRITICAL, HIGH, MEDIUM, LOW, UNKNOWN
	Title        string
	FixedVersion string
}

// osvQueryBatchRequest is the request body for OSV batch query
type osvQueryBatchRequest struct {
	Queries []osvQuery `json:"queries"`
}

// osvQuery is a single query in the batch
type osvQuery struct {
	Package osvPackage `json:"package"`
	Version string     `json:"version"`
}

// osvPackage identifies a package in the OSV database
type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// osvQueryBatchResponse is the response from OSV batch query
type osvQueryBatchResponse struct {
	Results []osvQueryResult `json:"results"`
}

// osvQueryResult is the result for a single query
type osvQueryResult struct {
	Vulns []osvVuln `json:"vulns"`
}

// osvVuln is a vulnerability from OSV
type osvVuln struct {
	ID       string        `json:"id"`
	Summary  string        `json:"summary"`
	Details  string        `json:"details"`
	Aliases  []string      `json:"aliases"`
	Database *osvDB        `json:"database"`
	Severity []osvSeverity `json:"severity"`
	Fixed    string        `json:"fixed"`
	Affected []osvAffected `json:"affected"`
}

// osvDB identifies the database source
type osvDB struct {
	URL string `json:"url"`
}

// osvSeverity represents a severity entry
type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

// osvAffected represents affected versions info
type osvAffected struct {
	Ranges []osvRange `json:"ranges"`
}

// osvRange represents version ranges
type osvRange struct {
	Type   string     `json:"type"`
	Events []osvEvent `json:"events"`
}

// osvEvent represents an event in a version range
type osvEvent struct {
	Introduced string `json:"introduced"`
	Fixed      string `json:"fixed"`
}

// CheckVulnerabilities queries the OSV database for known vulnerabilities
func CheckVulnerabilities(ctx context.Context, deps []CheckInput) []CheckResult {
	if !setting.DependencyChecker.VulnerabilityCheck {
		return nil
	}

	osvCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var queries []osvQuery
	for _, dep := range deps {
		osvEcosystem, ok := ecosystemMapping[dep.Ecosystem]
		if !ok {
			continue
		}
		queries = append(queries, osvQuery{
			Package: osvPackage{
				Name:      dep.Name,
				Ecosystem: osvEcosystem,
			},
			Version: dep.Version,
		})
	}

	if len(queries) == 0 {
		return nil
	}

	reqBody := osvQueryBatchRequest{Queries: queries}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		log.Error("OSV batch query marshal error: %v", err)
		return nil
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: proxy.Proxy(),
		},
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(osvCtx, http.MethodPost, osvQueryEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Error("OSV request creation error: %v", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Error("OSV API request error: %v", err)
		return nil
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("OSV response read error: %v", err)
		return nil
	}

	var osvResp osvQueryBatchResponse
	if err := json.Unmarshal(respBytes, &osvResp); err != nil {
		log.Error("OSV response unmarshal error: %v", err)
		return nil
	}

	results := make([]CheckResult, 0, len(osvResp.Results))
	for i, result := range osvResp.Results {
		if len(result.Vulns) == 0 {
			continue
		}
		if i >= len(deps) {
			break
		}
		dep := deps[i]
		var vulns []VulnerabilityInfo
		for _, v := range result.Vulns {
			vulns = append(vulns, osvVulnToInfo(v))
		}
		results = append(results, CheckResult{
			DependencyName:    dep.Name,
			DependencyVersion: dep.Version,
			DependencyID:      dep.DependencyID,
			Ecosystem:         dep.Ecosystem,
			Vulnerabilities:   vulns,
		})
	}

	return results
}

func osvVulnToInfo(v osvVuln) VulnerabilityInfo {
	title := v.Summary
	if title == "" {
		title = truncateText(v.Details, 200)
	}
	if title == "" {
		title = v.ID
	}

	sourceURL := ""
	if v.Database != nil {
		sourceURL = v.Database.URL
	}
	if sourceURL == "" && strings.HasPrefix(v.ID, "GHSA-") {
		sourceURL = fmt.Sprintf("https://github.com/advisories/%s", v.ID)
	}

	severity := parseOSVSeverity(v.Severity)

	fixedVersion := extractFixedVersion(v.Affected)

	return VulnerabilityInfo{
		SourceID:     v.ID,
		SourceURL:    sourceURL,
		Severity:     severity,
		Title:        title,
		FixedVersion: fixedVersion,
	}
}

func parseOSVSeverity(severities []osvSeverity) string {
	for _, s := range severities {
		if s.Type == "CVSS_V3" {
			return cvssToSeverity(s.Score)
		}
	}
	// Also check aliases for CVE-based severity lookup
	return "UNKNOWN"
}

func cvssToSeverity(score string) string {
	var f float64
	if _, err := fmt.Sscanf(score, "%f", &f); err != nil {
		return "UNKNOWN"
	}
	switch {
	case f >= 9.0:
		return "CRITICAL"
	case f >= 7.0:
		return "HIGH"
	case f >= 4.0:
		return "MEDIUM"
	case f > 0:
		return "LOW"
	default:
		return "UNKNOWN"
	}
}

func extractFixedVersion(affected []osvAffected) string {
	for _, a := range affected {
		for _, r := range a.Ranges {
			if r.Type == "SEMVER" || r.Type == "ECOSYSTEM" {
				for _, e := range r.Events {
					if e.Fixed != "" {
						return e.Fixed
					}
				}
			}
		}
	}
	return ""
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// CheckInput describes a dependency to check for vulnerabilities
type CheckInput struct {
	Name         string
	Version      string
	Ecosystem    string
	DependencyID int64
}
