// Package forgeinstall downloads the Forge installer and runs it to
// produce a runnable server in a project's data directory, replacing the
// job itzg/docker-minecraft-server used to do inside the container.
package forgeinstall

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type installedMarker struct {
	MCVersion    string `json:"mcVersion"`
	ForgeVersion string `json:"forgeVersion"`
}

func markerPath(stateDir string) string { return filepath.Join(stateDir, "installed.json") }

// Ensure makes sure Forge mcVersion/forgeVersion is installed in dataDir,
// downloading and running the installer only if it hasn't been done yet
// for that exact version on this PC (mirrors the "first start only"
// behavior the Docker image used to provide).
func Ensure(stateDir, dataDir, mcVersion, forgeVersion, javaExe string) error {
	if installedFor(stateDir, mcVersion, forgeVersion) {
		if _, err := FindLauncher(dataDir); err == nil {
			return nil
		}
		// Marker says installed but the launcher is missing (e.g. data/
		// was cleared) -- fall through and reinstall.
	} else if launcher, err := FindLauncher(dataDir); err == nil && launcher.matchesVersion(mcVersion, forgeVersion) {
		// No marker for this PC, but a matching install already exists on
		// disk -- e.g. a project migrated from the old Docker-based
		// mcsync, where Forge was already installed locally. Trust it
		// instead of reinstalling from scratch, and record the marker so
		// future starts take the fast path above.
		return writeMarker(stateDir, mcVersion, forgeVersion)
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}

	installerPath, err := downloadInstaller(stateDir, mcVersion, forgeVersion)
	if err != nil {
		return err
	}
	defer os.Remove(installerPath)

	fmt.Printf("Minecraft %s 用のForge %s をインストールしています(このPC・このバージョンでは初回のみ)...\n", mcVersion, forgeVersion)
	cmd := exec.Command(javaExe, "-jar", installerPath, "--installServer")
	cmd.Dir = dataDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Forgeインストーラーの実行に失敗しました: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dataDir, "eula.txt"), []byte("eula=true\n"), 0o644); err != nil {
		return fmt.Errorf("eula.txtの書き込みに失敗しました: %w", err)
	}

	if _, err := FindLauncher(dataDir); err != nil {
		return fmt.Errorf("Forgeインストーラーは完了しましたが、起動用のスクリプト/jarが見つかりませんでした: %w", err)
	}

	return writeMarker(stateDir, mcVersion, forgeVersion)
}

func writeMarker(stateDir, mcVersion, forgeVersion string) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(installedMarker{MCVersion: mcVersion, ForgeVersion: forgeVersion})
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath(stateDir), data, 0o644)
}

func installedFor(stateDir, mcVersion, forgeVersion string) bool {
	data, err := os.ReadFile(markerPath(stateDir))
	if err != nil {
		return false
	}
	var m installedMarker
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	return m.MCVersion == mcVersion && m.ForgeVersion == forgeVersion
}

func downloadInstaller(stateDir, mcVersion, forgeVersion string) (string, error) {
	url := fmt.Sprintf("https://maven.minecraftforge.net/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
		mcVersion, forgeVersion, mcVersion, forgeVersion)

	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("Forgeインストーラーのダウンロードに失敗しました: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Forgeインストーラーのダウンロードに失敗しました: HTTP %s (mcsync.ymlのminecraft.version/minecraft.forgeを確認してください)", resp.Status)
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(stateDir, "forge-installer.jar")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("Forgeインストーラーのダウンロード保存に失敗しました: %w", err)
	}
	return path, nil
}

// Launcher describes how to start the installed server directly with a
// java binary -- no cmd.exe/sh wrapper involved. Forge's own generated
// run.bat/run.sh scripts end with a "pause" (Windows) meant for someone
// double-clicking them from Explorer, which would otherwise block forever
// reading from the stdin pipe mcsync uses to send console commands (e.g.
// graceful "stop") once Minecraft itself has already exited. Running java
// directly, with the same arguments the script would have used, avoids
// that entirely.
type Launcher struct {
	// JarPath: for older Forge versions (no install-time run script), run
	// via `java -jar JarPath`.
	JarPath string
	// Args: for modern Forge (1.17+), the argfile-based arguments the
	// installer's run script would have passed to java (e.g.
	// "@user_jvm_args.txt @libraries/.../win_args.txt").
	Args []string
}

// matchesVersion reports whether this launcher looks like it was produced
// for mcVersion/forgeVersion: both the argfile paths a modern run script
// references (".../forge/<mc>-<forge>/...") and legacy standalone jar
// names (e.g. "forge-<mc>-<forge>-universal.jar") embed the version pair,
// so a substring check is enough -- used to trust an on-disk install found
// without a recorded marker (e.g. after migrating an old project) instead
// of blindly reusing it regardless of what mcsync.yml now says.
func (l *Launcher) matchesVersion(mcVersion, forgeVersion string) bool {
	needle := mcVersion + "-" + forgeVersion
	if strings.Contains(l.JarPath, needle) {
		return true
	}
	for _, a := range l.Args {
		if strings.Contains(a, needle) {
			return true
		}
	}
	return false
}

var javaInvocationRE = regexp.MustCompile(`(?im)^\s*java\s+(.+?)\s*$`)

// FindLauncher returns what to run to start the server: the java
// invocation extracted from the modern installer-generated run script
// (Forge 1.17+), or -- for older Forge versions, which don't generate one
// -- the standalone server jar the installer produced directly.
func FindLauncher(dataDir string) (*Launcher, error) {
	scriptName := "run.sh"
	if runtime.GOOS == "windows" {
		scriptName = "run.bat"
	}
	scriptPath := filepath.Join(dataDir, scriptName)
	if data, err := os.ReadFile(scriptPath); err == nil {
		m := javaInvocationRE.FindSubmatch(data)
		if m == nil {
			return nil, fmt.Errorf("%s の中にjava起動コマンドが見つかりませんでした", scriptPath)
		}
		args := strings.Fields(string(m[1]))
		var filtered []string
		for _, a := range args {
			// Drop the script's own passthrough-args placeholder ("%*" on
			// Windows, "$@"/"\"$@\"" on a POSIX shell) -- we append our
			// own args (nogui) instead.
			if a == "%*" || a == "$@" || a == `"$@"` {
				continue
			}
			filtered = append(filtered, a)
		}
		return &Launcher{Args: filtered}, nil
	}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("%s の読み取りに失敗しました: %w", dataDir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jar") {
			continue
		}
		if strings.HasPrefix(name, "forge-") && !strings.Contains(name, "installer") {
			return &Launcher{JarPath: filepath.Join(dataDir, name)}, nil
		}
	}
	return nil, fmt.Errorf("%s にrun.bat/run.shもforge-*.jarも見つかりませんでした(まだサーバーがインストールされていない可能性があります)", dataDir)
}
