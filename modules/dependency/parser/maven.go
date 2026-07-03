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
	Register(&mavenParser{})
}

type mavenParser struct{}

func (p *mavenParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "pom.xml"
}

var (
	mavenGroup   = regexp.MustCompile(`<groupId>([^<]+)</groupId>`)
	mavenArtifact = regexp.MustCompile(`<artifactId>([^<]+)</artifactId>`)
	mavenVersion  = regexp.MustCompile(`<version>([^<]+)</version>`)
	mavenScope    = regexp.MustCompile(`<scope>([^<]+)</scope>`)
)

func (p *mavenParser) Parse(reader io.Reader) ([]Dependency, error) {
	scanner := bufio.NewScanner(reader)
	var deps []Dependency
	var inDependency bool
	var currentGroup, currentArtifact, currentVersion, currentScope string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.Contains(line, "<dependency>") {
			inDependency = true
			currentGroup = ""
			currentArtifact = ""
			currentVersion = ""
			currentScope = ""
			continue
		}

		if strings.Contains(line, "</dependency>") && inDependency {
			inDependency = false
			if currentGroup != "" && currentArtifact != "" {
				name := currentGroup + ":" + currentArtifact
				depType := "runtime"
				if currentScope == "test" {
					depType = "dev"
				} else if currentScope == "provided" {
					depType = "optional"
				}
				deps = append(deps, Dependency{Name: name, Version: currentVersion, Type: depType})
			}
			continue
		}

		if inDependency {
			if m := mavenGroup.FindStringSubmatch(line); m != nil {
				currentGroup = m[1]
			}
			if m := mavenArtifact.FindStringSubmatch(line); m != nil {
				currentArtifact = m[1]
			}
			if m := mavenVersion.FindStringSubmatch(line); m != nil {
				currentVersion = m[1]
			}
			if m := mavenScope.FindStringSubmatch(line); m != nil {
				currentScope = m[1]
			}
		}
	}

	return deps, scanner.Err()
}
