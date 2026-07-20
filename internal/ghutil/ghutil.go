// Package ghutil wraps the GitHub CLI (`gh`) as subprocess calls, used
// only by `mcsync init` to optionally create a new GitHub repository.
// mcsync never talks to the GitHub API directly -- gh already handles
// authentication (including multiple accounts via `gh auth switch`), so
// there's no token to manage here.
package ghutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// IsInstalled reports whether the gh executable is reachable.
func IsInstalled() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// CurrentAccount returns the GitHub login gh is currently authenticated
// as, or ok=false if gh isn't authenticated at all.
func CurrentAccount() (login string, ok bool) {
	out, err := output("api", "user", "--jq", ".login")
	if err != nil || out == "" {
		return "", false
	}
	return out, true
}

var repoURLRE = regexp.MustCompile(`https://github\.com/([^/\s]+)/([^/\s]+)`)

// CreateRepo creates a new repository named name on GitHub (under the
// currently active gh account, or an org if name is "org/repo") with the
// given visibility. It never touches the local git repository -- the
// caller is expected to add the remote itself -- so it works regardless
// of whether a local repo has been initialized yet.
func CreateRepo(name string, private bool) (owner, repo string, err error) {
	args := []string{"repo", "create", name, "--description", ""}
	if private {
		args = append(args, "--private")
	} else {
		args = append(args, "--public")
	}
	out, err := output(args...)
	if err != nil {
		return "", "", fmt.Errorf("gh repo createに失敗しました: %w: %s", err, out)
	}
	m := repoURLRE.FindStringSubmatch(out)
	if m == nil {
		return "", "", fmt.Errorf("gh repo createの出力からリポジトリ情報を読み取れませんでした: %s", out)
	}
	return m[1], strings.TrimSuffix(m[2], ".git"), nil
}

func output(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}
