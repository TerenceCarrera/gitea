// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

func init() {
	Register(&pipParser{})
}

type pipParser struct{}

func (p *pipParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "requirements.txt" || name == "pipfile.lock"
}

func (p *pipParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if len(content) > 0 && content[0] == '{' {
		return parsePipfileLock(content)
	}
	return parseRequirementsTxt(content)
}

type pipfileLock struct {
	Default map[string]pipfileLockDep `json:"default"`
	Develop map[string]pipfileLockDep `json:"develop"`
}

type pipfileLockDep struct {
	Version string `json:"version"`
}

func parsePipfileLock(content []byte) ([]Dependency, error) {
	var lock pipfileLock
	if err := json.Unmarshal(content, &lock); err != nil {
		return nil, err
	}

	var deps []Dependency
	for name, dep := range lock.Default {
		version := strings.TrimPrefix(dep.Version, "==")
		deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
	}
	for name, dep := range lock.Develop {
		version := strings.TrimPrefix(dep.Version, "==")
		deps = append(deps, Dependency{Name: name, Version: version, Type: "dev"})
	}
	return deps, nil
}

func parseRequirementsTxt(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}

		parts := strings.SplitN(line, "==", 2)
		if len(parts) < 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		version := strings.TrimSpace(parts[1])

		deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
	}

	return deps, scanner.Err()
}
