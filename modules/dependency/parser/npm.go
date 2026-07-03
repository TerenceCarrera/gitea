// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"encoding/json"
	"io"
	"strings"
)

func init() {
	Register(&npmParser{})
}

type npmParser struct{}

func (p *npmParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "package.json"
}

type npmPackageJSON struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

func (p *npmParser) Parse(reader io.Reader) ([]Dependency, error) {
	var pkg npmPackageJSON
	if err := json.NewDecoder(reader).Decode(&pkg); err != nil {
		return nil, err
	}

	var deps []Dependency
	for name, version := range pkg.Dependencies {
		deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
	}
	for name, version := range pkg.DevDependencies {
		deps = append(deps, Dependency{Name: name, Version: version, Type: "dev"})
	}
	for name, version := range pkg.PeerDependencies {
		deps = append(deps, Dependency{Name: name, Version: version, Type: "dev"})
	}
	for name, version := range pkg.OptionalDependencies {
		deps = append(deps, Dependency{Name: name, Version: version, Type: "optional"})
	}
	return deps, nil
}
