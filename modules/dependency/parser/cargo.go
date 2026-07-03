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
	return name == "cargo.toml"
}

func (p *cargoParser) Parse(reader io.Reader) ([]Dependency, error) {
	scanner := bufio.NewScanner(reader)
	var deps []Dependency
	var inSection bool
	currentSection := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Detect sections like [dependencies], [dev-dependencies]
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

		// Parse "name = version" or "name = { version = "x.y.z" }"
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
