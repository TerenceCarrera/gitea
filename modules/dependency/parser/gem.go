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
	return name == "gemfile" || name == "gemfile.lock" || name == "podfile.lock"
}

func (p *gemParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	text := string(content)
	if strings.Contains(text, "GEM") || strings.Contains(text, "RUBY VERSION") {
		return parseGemfileLock(text)
	}
	if strings.Contains(text, "PODS") {
		return parsePodfileLock(text)
	}
	return parseGemfile(text)
}

func parseGemfile(content string) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "gem ") {
			rest := strings.TrimPrefix(line, "gem ")
			rest = strings.Trim(rest, "\" ")
			parts := strings.SplitN(rest, "\"", 2)
			name := strings.Trim(parts[0], "\" ")
			version := ""
			if len(parts) > 1 {
				version = strings.Trim(parts[1], "\", ")
			}

			deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
		}
	}

	return deps, scanner.Err()
}

func parseGemfileLock(content string) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var deps []Dependency
	inSpecs := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "specs:") || strings.HasPrefix(trimmed, "PLATFORMS") {
			inSpecs = strings.HasPrefix(trimmed, "specs:")
			continue
		}

		// New section (not indented enough or new header)
		if !strings.HasPrefix(line, " ") && trimmed != "" {
			inSpecs = false
			continue
		}

		if !inSpecs {
			continue
		}

		// Spec lines look like: "    gemname (1.2.3)"
		// Or with platform: "    gemname (1.2.3-x86_64-linux)"
		trimmed = strings.TrimSpace(line)
		if idx := strings.LastIndex(trimmed, "("); idx > 0 {
			name := strings.TrimSpace(trimmed[:idx])
			version := strings.TrimPrefix(trimmed[idx:], "(")
			version = strings.TrimSuffix(version, ")")
			// Strip platform suffixes like "-x86_64-linux"
			if dashIdx := strings.Index(version, "-"); dashIdx > 0 {
				// Only strip if it looks like a platform suffix, not part of version
				candidate := version[:dashIdx]
				if strings.Count(candidate, ".") == 2 {
					version = candidate
				}
			}
			if name != "" && version != "" {
				deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
			}
		}
	}

	return deps, scanner.Err()
}

func parsePodfileLock(content string) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var deps []Dependency
	inPods := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "PODS:") {
			inPods = true
			continue
		}

		if !strings.HasPrefix(line, " ") && trimmed != "" {
			inPods = false
			continue
		}

		if !inPods {
			continue
		}

		// Pod lines look like: "    PodName (1.2.3)" or "    PodName/Core (1.2.3)"
		trimmed = strings.TrimSpace(line)
		if idx := strings.LastIndex(trimmed, "("); idx > 0 {
			name := strings.TrimSpace(trimmed[:idx])
			version := strings.TrimPrefix(trimmed[idx:], "(")
			version = strings.TrimSuffix(version, ")")
			if name != "" && version != "" {
				deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
			}
		}
	}

	return deps, scanner.Err()
}
