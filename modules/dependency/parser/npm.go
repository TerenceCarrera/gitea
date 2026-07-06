// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

func init() {
	Register(&npmParser{})
}

type npmParser struct{}

func (p *npmParser) Detect(path string) bool {
	name := strings.ToLower(path)
	return name == "package.json" || name == "package-lock.json" ||
		name == "pnpm-lock.yaml" || name == "yarn.lock"
}

type npmPackageJSON struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

type npmLockV3 struct {
	Packages map[string]npmLockPackage `json:"packages"`
}

type npmLockV2 struct {
	Dependencies map[string]npmLockDep `json:"dependencies"`
}

type npmLockPackage struct {
	Version string `json:"version"`
}

type npmLockDep struct {
	Version string `json:"version"`
}

func (p *npmParser) Parse(reader io.Reader) ([]Dependency, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// Try JSON first (package.json, package-lock.json)
	if len(content) > 0 && content[0] == '{' {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(content, &raw); err == nil {
			if _, ok := raw["packages"]; ok {
				return parsePackageLockV3(content)
			}
			if _, ok := raw["dependencies"]; ok {
				// Could be package-lock.json v2 or package.json
				// package.json has string values; package-lock.json v2 has object values
				var rawDeps map[string]json.RawMessage
				if err := json.Unmarshal(raw["dependencies"], &rawDeps); err == nil {
					for _, v := range rawDeps {
						if v[0] == '"' {
							// String value = package.json
							return parsePackageJSON(content)
						}
						break
					}
				}
				return parsePackageLockV2(content)
			}
			// package.json with no lock file markers
			return parsePackageJSON(content)
		}
	}

	// Try YAML (pnpm-lock.yaml)
	var yamlLock pnpmLockFile
	if err := yaml.Unmarshal(content, &yamlLock); err == nil && yamlLock.Packages != nil {
		return parsePnpmLockYAML(yamlLock)
	}

	// Try yarn.lock (custom text format)
	return parseYarnLock(content)
}

func parsePackageJSON(content []byte) ([]Dependency, error) {
	var pkg npmPackageJSON
	if err := json.Unmarshal(content, &pkg); err != nil {
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

func parsePackageLockV3(content []byte) ([]Dependency, error) {
	var lock npmLockV3
	if err := json.Unmarshal(content, &lock); err != nil {
		return nil, err
	}

	var deps []Dependency
	for path, pkg := range lock.Packages {
		if path == "" {
			continue
		}
		name := path
		if idx := strings.LastIndex(path, "node_modules/"); idx >= 0 {
			name = path[idx+len("node_modules/"):]
		}
		deps = append(deps, Dependency{Name: name, Version: pkg.Version, Type: "runtime"})
	}
	return deps, nil
}

func parsePackageLockV2(content []byte) ([]Dependency, error) {
	var lock npmLockV2
	if err := json.Unmarshal(content, &lock); err != nil {
		return nil, err
	}

	var deps []Dependency
	for name, dep := range lock.Dependencies {
		deps = append(deps, Dependency{Name: name, Version: dep.Version, Type: "runtime"})
	}
	return deps, nil
}

type pnpmLockFile struct {
	Packages map[string]any `yaml:"packages"`
}

func parsePnpmLockYAML(lock pnpmLockFile) ([]Dependency, error) {
	var deps []Dependency
	for key := range lock.Packages {
		name, version := parsePnpmKey(key)
		if name != "" && version != "" {
			deps = append(deps, Dependency{Name: name, Version: version, Type: "runtime"})
		}
	}
	return deps, nil
}

func parsePnpmKey(key string) (name, version string) {
	idx := strings.LastIndex(key, "@")
	if idx <= 0 {
		return "", ""
	}
	name = key[:idx]
	version = key[idx+1:]
	if i := strings.Index(version, ":"); i >= 0 {
		version = version[i+1:]
	}
	return name, version
}

func parseYarnLock(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var deps []Dependency
	var currentName string

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, " ") {
			currentName = ""
			entry := strings.TrimRight(line, ":")
			entry = strings.ReplaceAll(entry, "\"", "")
			if idx := strings.Index(entry, "@"); idx > 0 {
				currentName = entry[:idx]
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "version \"") && currentName != "" {
			version := strings.TrimPrefix(trimmed, "version \"")
			version = strings.TrimRight(version, "\"")
			deps = append(deps, Dependency{Name: currentName, Version: version, Type: "runtime"})
			currentName = ""
		}
	}

	return deps, scanner.Err()
}
