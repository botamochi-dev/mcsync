// Package modurl extracts a project slug and platform from a Modrinth or
// CurseForge mod page URL (or accepts a bare slug directly).
package modurl

import (
	"fmt"
	"net/url"
	"strings"
)

// Platform identifies which mod host a slug came from.
type Platform string

const (
	Modrinth   Platform = "modrinth"
	CurseForge Platform = "curseforge"
)

var modrinthTypes = map[string]bool{
	"mod": true, "plugin": true, "datapack": true, "resourcepack": true, "shader": true,
}

// Parse extracts (platform, slug) from a Modrinth/CurseForge URL. If input
// isn't a URL, it's treated as a bare slug for the given fallback platform.
func Parse(input string, fallback Platform) (Platform, string, error) {
	input = strings.TrimSpace(input)
	if !strings.Contains(input, "://") {
		if input == "" {
			return "", "", fmt.Errorf("empty mod reference")
		}
		return fallback, input, nil
	}

	u, err := url.Parse(input)
	if err != nil {
		return "", "", fmt.Errorf("parsing URL %q: %w", input, err)
	}
	host := strings.ToLower(u.Host)
	var parts []string
	for _, p := range strings.Split(u.Path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}

	switch {
	case strings.Contains(host, "modrinth.com"):
		for i, p := range parts {
			if modrinthTypes[p] && i+1 < len(parts) {
				return Modrinth, parts[i+1], nil
			}
		}
		return "", "", fmt.Errorf("couldn't find a mod slug in Modrinth URL %q", input)

	case strings.Contains(host, "curseforge.com"):
		for i, p := range parts {
			if p == "minecraft" && i+2 < len(parts) {
				return CurseForge, parts[i+2], nil
			}
		}
		return "", "", fmt.Errorf("couldn't find a mod slug in CurseForge URL %q", input)

	default:
		return "", "", fmt.Errorf("unrecognized host %q (expected modrinth.com or curseforge.com)", host)
	}
}
