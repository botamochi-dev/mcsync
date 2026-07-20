// Package javautil provides a Java runtime for running the Forge server,
// without requiring the user to install Java themselves. It downloads the
// matching Eclipse Temurin (Adoptium) JRE on first use and caches it under
// the user's cache directory, shared across all mcsync projects on the PC.
package javautil

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RequiredMajor returns the Java major version Forge needs to run the given
// Minecraft version, based on Mojang/Forge's published requirements.
func RequiredMajor(mcVersion string) int {
	parts := strings.SplitN(mcVersion, ".", 3)
	minor, patch := 0, 0
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(strings.SplitN(parts[2], "-", 2)[0])
	}
	switch {
	case minor > 20 || (minor == 20 && patch >= 5):
		return 21
	case minor >= 18:
		return 17
	case minor == 17:
		return 16
	default:
		return 8
	}
}

// EnsureJRE makes sure a JRE for the given major version is downloaded and
// extracted, returning the directory containing the java executable.
// Downloads happen at most once per major version per PC.
func EnsureJRE(major int) (binDir string, err error) {
	root, err := cacheRoot()
	if err != nil {
		return "", fmt.Errorf("Javaキャッシュディレクトリの特定に失敗しました: %w", err)
	}
	dir := filepath.Join(root, fmt.Sprintf("%d-%s-%s", major, osName(), archName()))
	marker := filepath.Join(dir, ".complete")

	if _, err := os.Stat(marker); err == nil {
		if bin, err := findJavaBin(dir); err == nil {
			return bin, nil
		}
		// Marker present but the binary is missing somehow -- fall through
		// and re-download rather than failing outright.
	}

	fmt.Printf("Java %d ランタイムをダウンロードしています(初回のみ。以降のmcsyncプロジェクトでも再利用されます)...\n", major)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	archivePath, err := downloadJRE(major, dir)
	if err != nil {
		return "", err
	}
	defer os.Remove(archivePath)

	if err := extract(archivePath, dir); err != nil {
		return "", fmt.Errorf("Javaランタイムの展開に失敗しました: %w", err)
	}
	bin, err := findJavaBin(dir)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		return "", err
	}
	return bin, nil
}

func cacheRoot() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "mcsync", "jre"), nil
}

func osName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "mac"
	default:
		return "linux"
	}
}

func archName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}

func downloadJRE(major int, destDir string) (string, error) {
	url := fmt.Sprintf("https://api.adoptium.net/v3/binary/latest/%d/ga/%s/%s/jre/hotspot/normal/eclipse",
		major, osName(), archName())

	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("Java %d ランタイムのダウンロードに失敗しました: %w", major, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Java %d ランタイムのダウンロードに失敗しました: HTTP %s (%s/%s向けのビルドが無いようです)",
			major, resp.Status, osName(), archName())
	}

	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	path := filepath.Join(destDir, "download"+ext)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("Javaランタイムのダウンロード保存に失敗しました: %w", err)
	}
	return path, nil
}

func extract(archivePath, destDir string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir)
	}
	return extractTarGz(archivePath, destDir)
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target, err := safeJoin(destDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, target string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm()|0o644)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode().Perm()|0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(dst, tr); err != nil {
				dst.Close()
				return err
			}
			dst.Close()
		case tar.TypeSymlink:
			// Adoptium archives don't rely on symlinks for the java
			// binary itself; skip anything else to avoid link-target
			// escapes.
		}
	}
}

// safeJoin joins base and name, rejecting paths that would escape base
// (zip-slip protection for archive entries).
func safeJoin(base, name string) (string, error) {
	target := filepath.Join(base, name)
	if !strings.HasPrefix(target, filepath.Clean(base)+string(os.PathSeparator)) && target != filepath.Clean(base) {
		return "", fmt.Errorf("アーカイブ内のエントリ %q が展開先ディレクトリの外を指しています", name)
	}
	return target, nil
}

func findJavaBin(root string) (string, error) {
	target := "java"
	if runtime.GOOS == "windows" {
		target = "java.exe"
	}
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == target {
			found = filepath.Dir(path)
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%s 以下にjava実行ファイルが見つかりませんでした", root)
	}
	return found, nil
}
