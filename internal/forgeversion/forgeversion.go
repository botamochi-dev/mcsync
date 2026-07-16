// Package forgeversion looks up recommended Forge versions from the
// official promotions_slim.json feed, so mcsync never has to bundle or
// hardcode Forge version tables itself.
package forgeversion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const promotionsURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"

type promotions struct {
	Promos map[string]string `json:"promos"`
}

// Recommended returns the recommended (falling back to latest) Forge
// version for the given Minecraft version, e.g. "1.20.1" -> "47.3.0".
func Recommended(mcVersion string) (string, error) {
	proms, err := fetch()
	if err != nil {
		return "", err
	}
	if v, ok := proms.Promos[mcVersion+"-recommended"]; ok {
		return v, nil
	}
	if v, ok := proms.Promos[mcVersion+"-latest"]; ok {
		return v, nil
	}
	return "", fmt.Errorf("no Forge build found for Minecraft %s (checked -recommended and -latest)", mcVersion)
}

func fetch() (*promotions, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(promotionsURL)
	if err != nil {
		return nil, fmt.Errorf("fetching Forge promotions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching Forge promotions: HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Forge promotions: %w", err)
	}
	var p promotions
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("parsing Forge promotions: %w", err)
	}
	return &p, nil
}

// KnownMCVersions returns the set of Minecraft versions that have at least
// one Forge promotion, newest-looking first is not guaranteed (map order).
func KnownMCVersions() ([]string, error) {
	proms, err := fetch()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var versions []string
	for key := range proms.Promos {
		mc := strings.TrimSuffix(strings.TrimSuffix(key, "-recommended"), "-latest")
		if !seen[mc] {
			seen[mc] = true
			versions = append(versions, mc)
		}
	}
	return versions, nil
}
