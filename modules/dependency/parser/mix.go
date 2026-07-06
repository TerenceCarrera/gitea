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
	return name == "mix.exs" || name == "mix.lock"
}

func (p *mixParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if strings.Contains(string(content), "{\"erlang\"") || strings.HasPrefix(strings.TrimSpace(string(content)), "%{") {
		return parseMixLock(content)
	}
	return parseMixExs(content)
}

func parseMixExs(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if !strings.HasPrefix(line, "{:") {
			continue
		}

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

func parseMixLock(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "%" {
			continue
		}

		// Lines look like: "dep_name": {:hex, :dep_name, "hash", "1.2.3", ...}
		if !strings.HasPrefix(line, "\"") && !strings.HasPrefix(line, ":") {
			continue
		}

		// Extract name
		name := ""
		rest := line
		if strings.HasPrefix(line, "\"") {
			end := strings.Index(line[1:], "\"")
			if end >= 0 {
				name = line[1 : end+1]
				rest = strings.TrimSpace(line[end+2:])
			}
		} else if strings.HasPrefix(line, ":") {
			// :atom_name format
			spaceIdx := strings.Index(line, " ")
			if spaceIdx < 0 {
				spaceIdx = strings.Index(line, ":")
			}
			parts := strings.SplitN(line, ":", 3)
			if len(parts) >= 3 {
				name = parts[1]
				rest = strings.TrimSpace(parts[2])
			}
		}

		if name == "" {
			continue
		}

		// Extract version - it's the last quoted string before closing brace
		// Format: {:hex, :name, "hash", "1.2.3"}
		version := ""
		// Find all quoted strings
		quoted := extractQuotedStrings(rest)
		if len(quoted) >= 2 {
			// For hex packages: hash is quoted, version is quoted
			// For git packages: ref is quoted, no version
			for _, q := range quoted {
				if looksLikeVersion(q) {
					version = q
					break
				}
			}
		}

		if version != "" {
			deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
		}
	}

	return deps, scanner.Err()
}

func extractQuotedStrings(s string) []string {
	var result []string
	for {
		start := strings.Index(s, "\"")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+1:], "\"")
		if end < 0 {
			break
		}
		result = append(result, s[start+1:start+1+end])
		s = s[start+1+end+1:]
	}
	return result
}

func looksLikeVersion(s string) bool {
	if s == "" {
		return false
	}
	// Quick check: contains a digit and a dot
	hasDigit := false
	hasDot := false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
		if c == '.' {
			hasDot = true
		}
	}
	return hasDigit && hasDot
}
