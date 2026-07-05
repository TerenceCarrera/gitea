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

const (
	osvQueryBatchEndpoint = "https://api.osv.dev/v1/querybatch"
	osvVulnEndpoint       = "https://api.osv.dev/v1/vulns/"
)

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

// osvQueryResult is the result for a single query (batch only returns id+modified)
type osvQueryResult struct {
	Vulns []osvBatchVuln `json:"vulns"`
}

// osvBatchVuln is a minimal vuln entry from the batch query
type osvBatchVuln struct {
	ID       string `json:"id"`
	Modified string `json:"modified"`
}

// osvVuln is a vulnerability from OSV
type osvVuln struct {
	ID               string            `json:"id"`
	Summary          string            `json:"summary"`
	Details          string            `json:"details"`
	Aliases          []string          `json:"aliases"`
	Database         *osvDB            `json:"database"`
	Severity         []osvSeverity     `json:"severity"`
	Affected         []osvAffected     `json:"affected"`
	DatabaseSpecific map[string]any    `json:"database_specific"`
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
	Ranges            []osvRange           `json:"ranges"`
	EcosystemSpecific map[string]any       `json:"ecosystem_specific"`
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

	osvCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: proxy.Proxy(),
		},
		Timeout: 30 * time.Second,
	}

	// Step 1: Batch query to get vulnerability IDs per dependency
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

	req, err := http.NewRequestWithContext(osvCtx, http.MethodPost, osvQueryBatchEndpoint, bytes.NewReader(bodyBytes))
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

	// Collect all unique vuln IDs and map them to dependency indices
	vulnIDToDepIndices := make(map[string][]int) // vuln ID -> which deps reference it
	depVulnIDs := make([][]string, len(deps))    // vuln IDs per dep index
	for i, result := range osvResp.Results {
		if i >= len(deps) {
			break
		}
		for _, v := range result.Vulns {
			depVulnIDs[i] = append(depVulnIDs[i], v.ID)
			vulnIDToDepIndices[v.ID] = append(vulnIDToDepIndices[v.ID], i)
		}
	}

	// Step 2: Fetch full details for each unique vuln ID
	uniqueVulnIDs := make(map[string]struct{})
	for _, ids := range depVulnIDs {
		for _, id := range ids {
			uniqueVulnIDs[id] = struct{}{}
		}
	}

	fullVulns := make(map[string]osvVuln, len(uniqueVulnIDs))
	for vulnID := range uniqueVulnIDs {
		fullVuln, err := fetchOSVVuln(osvCtx, httpClient, vulnID)
		if err != nil {
			log.Warn("Failed to fetch OSV vuln %s: %v", vulnID, err)
			continue
		}
		fullVulns[vulnID] = fullVuln
	}

	// Step 3: Build results using full vuln data
	results := make([]CheckResult, 0, len(deps))
	for i, dep := range deps {
		vulnIDs := depVulnIDs[i]
		if len(vulnIDs) == 0 {
			continue
		}
		var vulns []VulnerabilityInfo
		for _, id := range vulnIDs {
			if v, ok := fullVulns[id]; ok {
				vulns = append(vulns, osvVulnToInfo(v))
			} else {
				// Fallback: use minimal info from batch query
				vulns = append(vulns, VulnerabilityInfo{
					SourceID: id,
					SourceURL: fmt.Sprintf("https://github.com/advisories/%s", id),
					Severity: "UNKNOWN",
					Title:    id,
				})
			}
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

// fetchOSVVuln fetches full vulnerability details from GET /v1/vulns/{id}
func fetchOSVVuln(ctx context.Context, httpClient *http.Client, vulnID string) (osvVuln, error) {
	var v osvVuln

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, osvVulnEndpoint+vulnID, nil)
	if err != nil {
		return v, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return v, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return v, fmt.Errorf("OSV API returned status %d for vuln %s", resp.StatusCode, vulnID)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return v, err
	}

	err = json.Unmarshal(respBytes, &v)
	return v, err
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

	severity := parseOSVSeverity(v)

	fixedVersion := extractFixedVersion(v.Affected)

	return VulnerabilityInfo{
		SourceID:     v.ID,
		SourceURL:    sourceURL,
		Severity:     severity,
		Title:        title,
		FixedVersion: fixedVersion,
	}
}

func parseOSVSeverity(v osvVuln) string {
	// 1. Check ecosystem_specific.severity from affected entries (simple string like "HIGH", "MEDIUM")
	for _, a := range v.Affected {
		if a.EcosystemSpecific != nil {
			if sev, ok := a.EcosystemSpecific["severity"]; ok {
				if s, ok := sev.(string); ok {
					if mapped := mapSeverityString(s); mapped != "" {
						return mapped
					}
				}
			}
		}
	}

	// 2. Check database_specific.severity (GitHub uses "MODERATE" for "MEDIUM")
	if v.DatabaseSpecific != nil {
		if sev, ok := v.DatabaseSpecific["severity"]; ok {
			if s, ok := sev.(string); ok {
				if mapped := mapSeverityString(s); mapped != "" {
					return mapped
				}
			}
		}
	}

	// 3. Parse CVSS vector strings from top-level severity array
	for _, s := range v.Severity {
		if s.Type == "CVSS_V3" || s.Type == "CVSS_V4" {
			if sev := parseCVSSVector(s.Score); sev != "UNKNOWN" {
				return sev
			}
		}
	}

	// 4. Parse CVSS v2 vectors
	for _, s := range v.Severity {
		if s.Type == "CVSS_V2" {
			if sev := parseCVSSVector(s.Score); sev != "UNKNOWN" {
				return sev
			}
		}
	}

	return "UNKNOWN"
}

// mapSeverityString maps various severity strings to standard CRITICAL/HIGH/MEDIUM/LOW
func mapSeverityString(s string) string {
	switch strings.ToUpper(s) {
	case "CRITICAL", "HIGH":
		return strings.ToUpper(s)
	case "MODERATE", "MEDIUM":
		return "MEDIUM"
	case "LOW":
		return "LOW"
	case "NONE":
		return "LOW"
	}
	return ""
}

// parseCVSSVector extracts severity from a CVSS vector string like "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:N/A:N"
func parseCVSSVector(vector string) string {
	vector = strings.TrimSpace(vector)
	if vector == "" {
		return "UNKNOWN"
	}

	// Extract CIA impact values from the vector (last 3 components)
	// Format: .../C:{N,L,H}/I:{N,L,H}/A:{N,L,H}
	parts := strings.Split(vector, "/")
	ciaValues := make(map[string]string)
	hasScope := false
	scopeChanged := false

	for _, part := range parts {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := kv[0], kv[1]
		switch key {
		case "S":
			hasScope = true
			scopeChanged = strings.EqualFold(val, "C")
		case "C":
			ciaValues["C"] = strings.ToUpper(val)
		case "I":
			ciaValues["I"] = strings.ToUpper(val)
		case "A":
			ciaValues["A"] = strings.ToUpper(val)
		}
	}

	c := ciaValues["C"]
	i := ciaValues["I"]
	a := ciaValues["A"]

	if c == "" && i == "" && a == "" {
		return "UNKNOWN"
	}

	// Heuristic based on CVSS v3 specification:
	// Scope Changed + any High impact → CRITICAL
	// Scope Changed + any Low impact → HIGH
	// Scope Unchanged + any High impact → HIGH
	// Scope Unchanged + any Low impact → MEDIUM
	// All None → LOW
	hasHigh := c == "H" || i == "H" || a == "H"
	hasLow := c == "L" || i == "L" || a == "L"

	if hasScope {
		if scopeChanged {
			if hasHigh {
				return "CRITICAL"
			}
			if hasLow {
				return "HIGH"
			}
			return "MEDIUM"
		}
		// Scope Unchanged
		if hasHigh {
			return "HIGH"
		}
		if hasLow {
			return "MEDIUM"
		}
		return "LOW"
	}

	// No scope info (CVSS v2 or unknown)
	if hasHigh {
		return "HIGH"
	}
	if hasLow {
		return "MEDIUM"
	}
	return "LOW"
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
