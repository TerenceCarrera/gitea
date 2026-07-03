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
	return strings.ToLower(path) == "go.mod"
}

func (p *goParser) Parse(reader io.Reader) ([]Dependency, error) {
	scanner := bufio.NewScanner(reader)
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Lines look like: "require github.com/foo/bar v1.2.3" or indirect: "github.com/foo/bar v1.2.3 // indirect"
		if strings.HasPrefix(line, "require (") || strings.HasPrefix(line, ")") || line == "" {
			continue
		}

		// Strip "require " prefix if present
		line = strings.TrimPrefix(line, "require ")

		// Check if it's a require line with a version
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
