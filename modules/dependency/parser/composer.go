// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"encoding/json"
	"io"
	"strings"
)

func init() {
	Register(&composerParser{})
}

type composerParser struct{}

func (p *composerParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "composer.json" || name == "composer.lock"
}

type composerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

type composerLockPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type composerLock struct {
	Packages    []composerLockPackage `json:"packages"`
	PackagesDev []composerLockPackage `json:"packages-dev"`
}

func (p *composerParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, err
	}

	if _, ok := raw["packages"]; ok {
		return parseComposerLock(content)
	}
	return parseComposerJSON(content)
}

func parseComposerJSON(content []byte) ([]Dependency, error) {
	var pkg composerJSON
	if err := json.Unmarshal(content, &pkg); err != nil {
		return nil, err
	}

	var deps []Dependency
	for name, version := range pkg.Require {
		deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
	}
	for name, version := range pkg.RequireDev {
		deps = append(deps, Dependency{Name: name, Version: version, Type: "dev"})
	}
	return deps, nil
}

func parseComposerLock(content []byte) ([]Dependency, error) {
	var lock composerLock
	if err := json.Unmarshal(content, &lock); err != nil {
		return nil, err
	}

	var deps []Dependency
	for _, pkg := range lock.Packages {
		deps = append(deps, Dependency{Name: pkg.Name, Version: pkg.Version, Type: "runtime"})
	}
	for _, pkg := range lock.PackagesDev {
		deps = append(deps, Dependency{Name: pkg.Name, Version: pkg.Version, Type: "dev"})
	}
	return deps, nil
}
