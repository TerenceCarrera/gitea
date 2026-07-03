// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"bufio"
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
	scanner := bufio.NewScanner(reader)
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

		dep := Dependency{Name: name, Version: version, Type: "runtime"}
		deps = append(deps, dep)
	}

	return deps, scanner.Err()
}
