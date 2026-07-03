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
	return name == "packages.config"
}

var nugetPackage = regexp.MustCompile(`<package\s+id="([^"]+)"\s+version="([^"]+)"`)

func (p *nugetParser) Parse(reader io.Reader) ([]Dependency, error) {
	scanner := bufio.NewScanner(reader)
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if m := nugetPackage.FindStringSubmatch(line); m != nil {
			deps = append(deps, Dependency{Name: m[1], Version: m[2], Type: "runtime"})
		}
	}

	return deps, scanner.Err()
}
