// Package scaffold renders the initial docker-compose.yml and .gitignore
// for a new mcsync project. docker-compose.yml is the manifest: MC/Forge
// version and (later) the mod list all live in it, so mcsync itself never
// needs a separate config format.
package scaffold

import (
	"bytes"
	"regexp"
	"strings"
	"text/template"
)

// ComposeData holds the values needed to render a fresh docker-compose.yml.
type ComposeData struct {
	ProjectName  string
	MCVersion    string
	ForgeVersion string
	Memory       string
}

var composeTemplate = template.Must(template.New("compose").Parse(
	`name: {{.ProjectName}}

services:
  mc:
    image: itzg/minecraft-server:latest
    environment:
      EULA: "true"
      TYPE: "FORGE"
      VERSION: "{{.MCVersion}}"
      FORGE_VERSION: "{{.ForgeVersion}}"
      MEMORY: "{{.Memory}}"
    ports:
      - "25565:25565"
    volumes:
      - ./data:/data
    stdin_open: true
    tty: true
`))

// RenderCompose renders the docker-compose.yml content for a new project.
func RenderCompose(data ComposeData) (string, error) {
	var buf bytes.Buffer
	if err := composeTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var invalidProjectNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// SanitizeProjectName converts an arbitrary human-entered name into a
// value that's safe to use as a Docker Compose project name (the
// top-level `name:` key): lowercase, alphanumeric, '-' and '_' only.
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

// Gitignore is the .gitignore content for a fresh project. It follows the
// itzg/docker-minecraft-server data/ layout: only durable player-facing
// state (world, config, server.properties, ops/whitelist) is tracked.
// Everything itzg regenerates on its own (mods, libraries, logs, eula.txt)
// is ignored.
const Gitignore = `data/*
!data/world/
!data/world/**
!data/config/
!data/config/**
!data/server.properties
!data/ops.json
!data/whitelist.json
data/mods/
data/libraries/
data/logs/
data/cache/
data/eula.txt
data/*.log
`
