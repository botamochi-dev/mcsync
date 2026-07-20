// Package manifest reads mcsync.yml, the project manifest that replaced
// docker-compose.yml: it declares the Minecraft/Forge version and memory
// allocation, nothing else.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the manifest's filename, expected at a project's root.
const FileName = "mcsync.yml"

type Minecraft struct {
	Version string `yaml:"version"`
	Forge   string `yaml:"forge"`
}

type Manifest struct {
	Name      string    `yaml:"name"`
	Minecraft Minecraft `yaml:"minecraft"`
	Memory    string    `yaml:"memory"`
}

// Path returns the manifest path for project directory dir.
func Path(dir string) string {
	return filepath.Join(dir, FileName)
}

// Load reads and parses dir's mcsync.yml. If it's missing but a project
// created by the old Docker-based mcsync (docker-compose.yml) is found
// instead, it's transparently migrated: mcsync.yml is generated from it
// and .gitignore is patched to add the new .mcsync/ entry. The old
// docker-compose.yml itself is left in place (nothing is deleted) --
// see MigrationNote for what the caller should tell the user to do next.
func Load(dir string) (*Manifest, error) {
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		if os.IsNotExist(err) {
			if m, err := migrateLegacyCompose(dir); err == nil {
				return m, nil
			}
			return nil, fmt.Errorf("%s が見つかりません(プロジェクトのルートフォルダで実行しているか確認するか、先に `mcsync init` を実行してください)", FileName)
		}
		return nil, err
	}
	return parse(data)
}

func parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s の解析に失敗しました: %w", FileName, err)
	}
	if m.Minecraft.Version == "" {
		return nil, fmt.Errorf("%s: minecraft.version は必須です", FileName)
	}
	if m.Minecraft.Forge == "" {
		return nil, fmt.Errorf("%s: minecraft.forge は必須です", FileName)
	}
	return &m, nil
}

// legacyComposeFileName is the manifest filename used by mcsync before it
// dropped Docker (see the "docker-compose.yml is the manifest" era).
const legacyComposeFileName = "docker-compose.yml"

// legacyCompose captures just the fields the old scaffold ever wrote to
// docker-compose.yml -- enough to reconstruct an equivalent mcsync.yml.
type legacyCompose struct {
	Name     string `yaml:"name"`
	Services struct {
		MC struct {
			Environment struct {
				Version      string `yaml:"VERSION"`
				ForgeVersion string `yaml:"FORGE_VERSION"`
				Memory       string `yaml:"MEMORY"`
			} `yaml:"environment"`
		} `yaml:"mc"`
	} `yaml:"services"`
}

func migrateLegacyCompose(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, legacyComposeFileName))
	if err != nil {
		return nil, err
	}
	var legacy legacyCompose
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("旧形式の %s の解析に失敗しました: %w", legacyComposeFileName, err)
	}
	env := legacy.Services.MC.Environment
	if env.Version == "" || env.ForgeVersion == "" {
		return nil, fmt.Errorf("%s はmcsyncプロジェクトのものではないようです(services.mc.environment.VERSION/FORGE_VERSIONがありません)", legacyComposeFileName)
	}

	m := &Manifest{Name: legacy.Name, Memory: env.Memory}
	m.Minecraft.Version = env.Version
	m.Minecraft.Forge = env.ForgeVersion
	if m.Memory == "" {
		m.Memory = "4G"
	}

	out, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(Path(dir), out, 0o644); err != nil {
		return nil, err
	}
	addGitignoreEntry(dir, ".mcsync/")

	fmt.Printf("旧Docker版のプロジェクト構成から移行しました: %s から %s を生成しました。\n"+
		"内容を確認できたら、次回の保存時に `git add %s .gitignore` してください(不要になった %s の削除もご検討ください)。\n\n",
		legacyComposeFileName, FileName, FileName, legacyComposeFileName)
	return m, nil
}

// addGitignoreEntry appends line to dir's .gitignore if it exists and
// doesn't already contain it. Best-effort: failures are silently ignored,
// since this is a convenience patch-up, not something migration should
// fail over.
func addGitignoreEntry(dir, line string) {
	path := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), line) {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}
