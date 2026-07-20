// Package scaffold renders the initial mcsync.yml and .gitignore for a new
// mcsync project. mcsync.yml is the manifest: MC/Forge version and memory
// live in it, so mcsync itself never needs a separate config format.
package scaffold

import (
	"bytes"
	"regexp"
	"strings"
	"text/template"
)

// ManifestData holds the values needed to render a fresh mcsync.yml.
type ManifestData struct {
	ProjectName  string
	MCVersion    string
	ForgeVersion string
	Memory       string
}

var manifestTemplate = template.Must(template.New("manifest").Parse(
	`name: {{.ProjectName}}

minecraft:
  version: "{{.MCVersion}}"
  forge: "{{.ForgeVersion}}"

memory: "{{.Memory}}"
`))

// ModsLFSPattern is the git-lfs tracking pattern applied to a fresh
// project during `init`, so mod jars manually placed in data/mods don't
// bloat plain git history as they're added or updated.
const ModsLFSPattern = "data/mods/**/*.jar"

// RenderManifest renders the mcsync.yml content for a new project.
func RenderManifest(data ManifestData) (string, error) {
	var buf bytes.Buffer
	if err := manifestTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var invalidProjectNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// SanitizeProjectName converts an arbitrary human-entered name into a
// value that's safe to use as the manifest's `name:` key: lowercase,
// alphanumeric, '-' and '_' only.
func SanitizeProjectName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = invalidProjectNameChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")
	if s == "" {
		return "mcsync-server"
	}
	return s
}

// Gitignore is the .gitignore content for a fresh project. The Forge
// server runs directly with data/ as its working directory (no Docker
// volume mapping needed), so this still follows the same data/ layout as
// before: only durable player-facing state is tracked -- world, config,
// server.properties, ops/whitelist, and mods (jars placed directly in
// data/mods; tracked via Git LFS, see ModsLFSPattern). Everything else
// under data/ (Forge itself, libraries, run scripts, logs, eula.txt) is
// regenerated automatically by `mcsync start`/`setup` and ignored here.
// .mcsync/ holds purely local runtime state (PID/control files, download
// caches) and is never synced.
const Gitignore = `data/*
!data/world/
!data/world/**
!data/config/
!data/config/**
!data/server.properties
!data/ops.json
!data/whitelist.json
!data/mods/
!data/mods/**
data/libraries/
data/logs/
data/cache/
data/eula.txt
data/*.log
.mcsync/
`
