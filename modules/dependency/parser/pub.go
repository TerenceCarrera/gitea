// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"encoding/json"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

func init() {
	Register(&pubParser{})
}

type pubParser struct{}

func (p *pubParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "pubspec.yaml" || name == "pubspec.lock"
}

type pubspecYAML struct {
	Dependencies    map[string]any `yaml:"dependencies"`
	DevDependencies map[string]any `yaml:"dev_dependencies"`
}

func (p *pubParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if isPubspecLock(content) {
		return parsePubspecLock(content)
	}
	return parsePubspecYAML(content)
}

func isPubspecLock(content []byte) bool {
	// pubspec.lock has a "packages:" key with a map of package entries
	var raw map[string]any
	if err := yaml.Unmarshal(content, &raw); err == nil {
		_, hasPackages := raw["packages"]
		_, hasSDks := raw["sdks"]
		return hasPackages && hasSDks
	}
	return false
}

func parsePubspecYAML(content []byte) ([]Dependency, error) {
	var spec pubspecYAML
	if err := yaml.Unmarshal(content, &spec); err != nil {
		return nil, err
	}

	var deps []Dependency
	for name, val := range spec.Dependencies {
		deps = append(deps, pubDependency(name, val, "runtime"))
	}
	for name, val := range spec.DevDependencies {
		deps = append(deps, pubDependency(name, val, "dev"))
	}
	return deps, nil
}

func pubDependency(name string, val any, depType string) Dependency {
	version := ""
	switch v := val.(type) {
	case string:
		version = v
	case map[string]any:
		if ver, ok := v["version"].(string); ok {
			version = ver
		}
	}
	return Dependency{Name: name, Version: version, Type: depType}
}

type pubspecLock struct {
	Packages map[string]pubspecLockEntry `yaml:"packages"`
}

type pubspecLockEntry struct {
	Version string `yaml:"version"`
}

func parsePubspecLock(content []byte) ([]Dependency, error) {
	var lock pubspecLock
	if err := yaml.Unmarshal(content, &lock); err != nil {
		// Try JSON fallback
		var jsonLock pubspecLock
		if err2 := json.Unmarshal(content, &jsonLock); err2 != nil {
			return nil, err
		}
		lock = jsonLock
	}

	var deps []Dependency
	for name, entry := range lock.Packages {
		deps = append(deps, Dependency{Name: name, Version: entry.Version, Type: "runtime"})
	}
	return deps, nil
}
