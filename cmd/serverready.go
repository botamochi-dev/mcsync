package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"mcsync/internal/forgeinstall"
	"mcsync/internal/javautil"
	"mcsync/internal/manifest"
)

// prepareServer loads the project manifest and makes sure a matching Java
// runtime and Forge server install are present in dir, downloading either
// if needed (first run on this PC only -- see javautil/forgeinstall).
// Returns the Java bin directory to use when launching the server.
func prepareServer(dir string) (javaBinDir string, err error) {
	m, err := manifest.Load(dir)
	if err != nil {
		return "", err
	}
	// Resolve to absolute before deriving data/state paths: they get
	// passed as subprocess arguments (Forge installer) run with a
	// different working directory, and a relative path would then resolve
	// against *that* directory instead of the caller's.
	dir = mustAbs(dir)
	major := javautil.RequiredMajor(m.Minecraft.Version)
	javaBinDir, err = javautil.EnsureJRE(major)
	if err != nil {
		return "", fmt.Errorf("Java %d ランタイムの準備に失敗しました: %w", major, err)
	}
	javaExe := filepath.Join(javaBinDir, javaExeName())
	dataDir := filepath.Join(dir, "data")
	if err := ensureDefaultServerProperties(dataDir); err != nil {
		return "", fmt.Errorf("server.propertiesの初期化に失敗しました: %w", err)
	}
	stateDir := filepath.Join(dir, ".mcsync")
	if err := forgeinstall.Ensure(stateDir, dataDir, m.Minecraft.Version, m.Minecraft.Forge, javaExe); err != nil {
		return "", err
	}
	return javaBinDir, nil
}

// defaultServerProperties are mcsync's preferred overrides to Minecraft's
// own server.properties defaults. Minecraft's property loader fills in
// every other key with its usual defaults and rewrites the full file on
// first boot, so seeding just these two lines is enough.
const defaultServerProperties = "allow-flight=true\nenable-command-block=true\n"

// ensureDefaultServerProperties writes data/server.properties with
// mcsync's defaults before the very first boot, so Minecraft picks them up
// immediately instead of writing its own (allow-flight=false,
// enable-command-block=false) defaults. Never touches an existing file --
// once the server (or the user) has written one, that's authoritative and
// syncs via git like any other tracked state.
func ensureDefaultServerProperties(dataDir string) error {
	path := filepath.Join(dataDir, "server.properties")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultServerProperties), 0o644)
}

func javaExeName() string {
	if runtime.GOOS == "windows" {
		return "java.exe"
	}
	return "java"
}

// mustAbs resolves dir to an absolute path, falling back to dir itself if
// that somehow fails (better than panicking on a cosmetic path-display
// helper).
func mustAbs(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}
