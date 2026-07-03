// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package parser

import "io"

type Dependency struct {
	Name    string
	Version string
	Type    string // "runtime", "dev", "optional"
}

type Parser interface {
	// Detect checks if the given filename is a lock/manifest file this parser handles
	Detect(path string) bool
	// Parse reads the content of a lock/manifest file and extracts dependencies
	Parse(reader io.Reader) ([]Dependency, error)
}

// Registry of all available parsers
var parsers []Parser

func Register(p Parser) {
	parsers = append(parsers, p)
}

func Parsers() []Parser {
	return parsers
}

// DetectableFiles returns all filenames that any parser can handle
func DetectableFiles() []string {
	return []string{
		"package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"go.mod", "go.sum",
		"Cargo.toml", "Cargo.lock",
		"requirements.txt", "Pipfile.lock",
		"composer.json", "composer.lock",
		"Gemfile", "Gemfile.lock",
		"pom.xml", "gradle.lockfile", "build.gradle", "build.gradle.kts",
		"packages.config", "paket.lock",
		"pubspec.yaml", "pubspec.lock",
		"mix.exs", "mix.lock",
		"Podfile", "Podfile.lock",
		"environment.yml", "environment.yaml",
	}
}
