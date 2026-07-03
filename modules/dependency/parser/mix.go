// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"bufio"
	"io"
	"strings"
)

func init() {
	Register(&mixParser{})
}

type mixParser struct{}

func (p *mixParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "mix.exs"
}

func (p *mixParser) Parse(reader io.Reader) ([]Dependency, error) {
	scanner := bufio.NewScanner(reader)
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if !strings.HasPrefix(line, "{:") {
			continue
		}

		// {:dep_name, "~> 1.0"} or {:dep_name, ">= 1.0", only: :dev}
		line = strings.TrimPrefix(line, "{:")
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		rest := strings.TrimSpace(parts[1])
		rest = strings.TrimRight(rest, "} ")

		version := ""
		depType := "runtime"

		// Extract version from quoted string
		if strings.HasPrefix(rest, "\"") {
			end := strings.Index(rest[1:], "\"")
			if end >= 0 {
				version = rest[1 : end+1]
			}
		}

		if strings.Contains(rest, ":dev") || strings.Contains(rest, ":test") {
			depType = "dev"
		}

		deps = append(deps, Dependency{Name: name, Version: version, Type: depType})
	}

	return deps, scanner.Err()
}
