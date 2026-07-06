// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

func init() {
	Register(&nugetParser{})
}

type nugetParser struct{}

func (p *nugetParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "packages.config" || name == "paket.lock"
}

var nugetPackage = regexp.MustCompile(`<package\s+id="([^"]+)"\s+version="([^"]+)"`)

func (p *nugetParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if strings.Contains(string(content), "GROUP") && strings.Contains(string(content), "NUGET") {
		return parsePaketLock(content)
	}
	return parsePackagesConfig(content)
}

func parsePackagesConfig(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if m := nugetPackage.FindStringSubmatch(line); m != nil {
			deps = append(deps, Dependency{Name: m[1], Version: m[2], Type: "runtime"})
		}
	}

	return deps, scanner.Err()
}

func parsePaketLock(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency
	inNugroup := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Section headers like "GROUP: NUGET" or "GROUP: SOURCE"
		if strings.HasPrefix(trimmed, "GROUP:") {
			inNugroup = strings.Contains(strings.ToUpper(trimmed), "NUGET")
			continue
		}

		if strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "  ") {
			// Sub-section like "  remote: ..."
			continue
		}

		if !inNugroup {
			continue
		}

		// Package lines look like: "    PackageName (1.2.3)"
		trimmed = strings.TrimSpace(line)
		if idx := strings.LastIndex(trimmed, "("); idx > 0 {
			name := strings.TrimSpace(trimmed[:idx])
			version := strings.TrimPrefix(trimmed[idx:], "(")
			version = strings.TrimSuffix(version, ")")
			if name != "" && version != "" && !strings.Contains(name, " ") {
				deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
			}
		}
	}

	return deps, scanner.Err()
}
