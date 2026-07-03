// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"bufio"
	"io"
	"strings"
)

func init() {
	Register(&gemParser{})
}

type gemParser struct{}

func (p *gemParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "gemfile"
}

func (p *gemParser) Parse(reader io.Reader) ([]Dependency, error) {
	scanner := bufio.NewScanner(reader)
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// gem "name", "~> 1.2.3" or gem "name", ">= 1.0"
		if strings.HasPrefix(line, "gem ") {
			rest := strings.TrimPrefix(line, "gem ")
			rest = strings.Trim(rest, "\" ")
			parts := strings.SplitN(rest, "\"", 2)
			name := strings.Trim(parts[0], "\" ")
			version := ""
			if len(parts) > 1 {
				version = strings.Trim(parts[1], "\", ")
			}

			depType := "runtime"
			deps = append(deps, Dependency{Name: name, Version: version, Type: depType})
		}
	}

	return deps, scanner.Err()
}
