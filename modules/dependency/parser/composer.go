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
	return name == "composer.json"
}

type composerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

func (p *composerParser) Parse(reader io.Reader) ([]Dependency, error) {
	var pkg composerJSON
	if err := json.NewDecoder(reader).Decode(&pkg); err != nil {
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
