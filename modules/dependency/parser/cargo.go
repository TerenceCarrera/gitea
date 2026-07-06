// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"bufio"
	"io"
	"strings"
)

func init() {
	Register(&cargoParser{})
}

type cargoParser struct{}

func (p *cargoParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "cargo.toml" || name == "cargo.lock"
}

func (p *cargoParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if isCargoLock(content) {
		return parseCargoLock(content)
	}
	return parseCargoToml(content)
}

func isCargoLock(content []byte) bool {
	return strings.Contains(string(content), "[[package]]")
}

func parseCargoToml(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency
	var inSection bool
	currentSection := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimPrefix(line, "[")
			section = strings.TrimSuffix(section, "]")
			currentSection = section
			inSection = strings.Contains(section, "dependency")
			continue
		}

		if !inSection {
			continue
		}

		if idx := strings.Index(line, "="); idx > 0 {
			name := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			version := strings.Trim(value, "\"{} ")

			depType := "runtime"
			if strings.Contains(currentSection, "dev") {
				depType = "dev"
			} else if strings.Contains(currentSection, "build") {
				depType = "optional"
			}

			deps = append(deps, Dependency{Name: name, Version: version, Type: depType})
		}
	}

	return deps, scanner.Err()
}

func parseCargoLock(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency
	var inPackage bool
	var currentName, currentVersion string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "[[package]]" {
			if inPackage && currentName != "" && currentVersion != "" {
				deps = append(deps, Dependency{Name: currentName, Version: currentVersion, Type: "runtime"})
			}
			inPackage = true
			currentName = ""
			currentVersion = ""
			continue
		}

		if !inPackage {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if currentName != "" && currentVersion != "" {
				deps = append(deps, Dependency{Name: currentName, Version: currentVersion, Type: "runtime"})
			}
			inPackage = false
			continue
		}

		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, "\"")

			switch key {
			case "name":
				currentName = value
			case "version":
				currentVersion = value
			}
		}
	}

	if inPackage && currentName != "" && currentVersion != "" {
		deps = append(deps, Dependency{Name: currentName, Version: currentVersion, Type: "runtime"})
	}

	return deps, scanner.Err()
}
