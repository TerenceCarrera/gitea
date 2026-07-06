// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"bufio"
	"io"
	"strings"
)

func init() {
	Register(&goParser{})
}

type goParser struct{}

func (p *goParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "go.mod" || name == "go.sum"
}

func (p *goParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// go.sum lines look like "module version/go.mod hash algo hash"
	// go.mod lines look like "require module version" or "require ( module version ... )"
	if isGoSum(content) {
		return parseGoSum(content)
	}
	return parseGoMod(content)
}

func isGoSum(content []byte) bool {
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.Contains(parts[1], "/go.mod") {
			return true
		}
		return false
	}
	return false
}

func parseGoMod(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "require (") || strings.HasPrefix(line, ")") || line == "" {
			continue
		}

		line = strings.TrimPrefix(line, "require ")

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			name := parts[0]
			version := parts[1]
			depType := "runtime"
			if strings.Contains(line, "// indirect") {
				depType = "dev"
			}
			deps = append(deps, Dependency{Name: name, Version: version, Type: depType})
		}
	}

	return deps, scanner.Err()
}

func parseGoSum(content []byte) ([]Dependency, error) {
	lines := strings.Split(string(content), "\n")
	var deps []Dependency
	seen := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: module version/go.mod hash algo hash
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		version := parts[1]

		// Strip /go.mod suffix if present
		if idx := strings.Index(version, "/go.mod"); idx >= 0 {
			version = version[:idx]
		}

		// Skip pseudo-versions and invalid versions
		if version == "" || strings.HasPrefix(version, "v0.0.0-") {
			continue
		}

		// Deduplicate (each module appears twice: once for module, once for go.mod)
		if seen[name+"@"+version] {
			continue
		}
		seen[name+"@"+version] = true

		deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
	}

	return deps, nil
}
