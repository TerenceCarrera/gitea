// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

// DependencyChecker settings
var DependencyChecker = struct {
	Enabled            bool
	VulnerabilityCheck bool
}{
	Enabled:            false,
	VulnerabilityCheck: false,
}

func loadDependenciesFrom(rootCfg ConfigProvider) {
	DependencyChecker.Enabled = rootCfg.Section("dependency_checker").Key("ENABLED").MustBool(false)
	DependencyChecker.VulnerabilityCheck = rootCfg.Section("dependency_checker").Key("VULNERABILITY_CHECK").MustBool(false)
}
