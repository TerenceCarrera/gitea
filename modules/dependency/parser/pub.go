// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"gopkg.in/yaml.v3"
	"io"
	"strings"
)

func init() {
	Register(&pubParser{})
}

type pubParser struct{}

func (p *pubParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "pubspec.yaml"
}

type pubspecYAML struct {
	Dependencies    map[string]any `yaml:"dependencies"`
	DevDependencies map[string]any `yaml:"dev_dependencies"`
}

func (p *pubParser) Parse(reader io.Reader) ([]Dependency, error) {
	var spec pubspecYAML
	if err := yaml.NewDecoder(reader).Decode(&spec); err != nil {
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
