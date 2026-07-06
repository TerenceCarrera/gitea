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
	return name == "pom.xml" || name == "gradle.lockfile"
}

var (
	mavenGroup    = regexp.MustCompile(`<groupId>([^<]+)</groupId>`)
	mavenArtifact = regexp.MustCompile(`<artifactId>([^<]+)</artifactId>`)
	mavenVersion  = regexp.MustCompile(`<version>([^<]+)</version>`)
	mavenScope    = regexp.MustCompile(`<scope>([^<]+)</scope>`)
)

func (p *mavenParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if strings.Contains(string(content), "=") && strings.Contains(string(content), ":") &&
		!strings.Contains(string(content), "<dependency>") {
		return parseGradleLockfile(content)
	}
	return parsePomXML(content)
}

func parsePomXML(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
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

func parseGradleLockfile(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "empty") {
			continue
		}

		// Format: depName=group:artifact:version
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}

		rightSide := strings.TrimSpace(line[idx+1:])

		// Parse group:artifact:version
		parts := strings.Split(rightSide, ":")
		if len(parts) < 3 {
			continue
		}

		group := parts[0]
		artifact := parts[1]
		version := parts[2]

		name := group + ":" + artifact
		deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
	}

	return deps, scanner.Err()
}
