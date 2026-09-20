package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const includeLine = "Include ~/.ssh/repose/config"
const includeComment = "# added by repose"

// purgeCLIFiles removes everything repose owns on the laptop (07-cli.md
// §8 "Rollback": `~/.config/repose/`, `~/.ssh/repose/` and the one
// `Include` line).
func purgeCLIFiles(configDirPath, sshDirPath string) error {
	if err := os.RemoveAll(configDirPath); err != nil {
		return err
	}
	if err := os.RemoveAll(sshDirPath); err != nil {
		return err
	}
	sc, err := userSSHConfig()
	if err != nil {
		return err
	}
	return removeIncludeLine(sc)
}

// removeIncludeLine drops the "# added by repose" comment and the Include
// line that follows it, leaving every other line untouched.
func removeIncludeLine(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := splitLines(string(b))
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == includeComment && i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == includeLine {
			i++
			continue
		}
		out = append(out, lines[i])
	}
	perm := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}
	return writeFileAtomic(path, []byte(strings.Join(out, "\n")), perm)
}

func splitLines(s string) []string {
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// ensureIncludeLine inserts the comment and Include line as the first two
// lines of ~/.ssh/config, once, never touching any other line (07-cli.md
// §5.4 and its checklist "exactly one Include line, first line, and no
// other line changes on repeated runs").
func ensureIncludeLine(path string) error {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := string(b)
	if strings.Contains(existing, includeLine) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	newContent := includeComment + "\n" + includeLine + "\n"
	if existing != "" {
		newContent += existing
	}
	perm := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}
	return writeFileAtomic(path, []byte(newContent), perm)
}
