// Package proc answers two questions about a process of this user from
// /proc, for verified MPRIS player ownership (internal/platform/mpris,
// docs/security.md "Now playing"): which Flatpak app it runs in, and whether
// it descends from a given process (a web app's own browser, started by
// internal/applications/web).
//
// The Flatpak app id comes from the same place xdg-desktop-portal reads it:
// the process's own /.flatpak-info ([Application] name=), reached through
// /proc/<pid>/root. Flatpak also puts that file into the sandbox of the
// D-Bus proxy it runs for an app with a filtered session bus, which is the
// connection the bus daemon names for such an app's player. The systemd
// scope in /proc/<pid>/cgroup (app-flatpak-<id>-<n>.scope) is the second
// source; when both are present and disagree the answer is an error, never
// a guess. Only processes owned by this user are read: another user's
// process is an error (ErrOtherUser). A same-user process could fake either
// source; the threat model trusts the owner's own session (docs/security.md).
package proc

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ErrOtherUser: the process belongs to another user and is never read.
var ErrOtherUser = errors.New("proc: process belongs to another user")

// maxInfoBytes bounds the .flatpak-info and cgroup files read.
const maxInfoBytes = 64 << 10

// maxDepth bounds a parent walk (a loop or a very deep tree ends it).
const maxDepth = 64

// Table reads one /proc tree. Root is "/proc" on the TV; tests point it at a
// directory shaped like /proc. UID is the only owner whose processes are read.
type Table struct {
	Root string
	UID  int
}

// Host is this machine's /proc for the current user.
func Host() Table { return Table{Root: "/proc", UID: os.Getuid()} }

func (t Table) dir(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("proc: invalid pid %d", pid)
	}
	d := filepath.Join(t.Root, strconv.Itoa(pid))
	fi, err := os.Stat(d)
	if err != nil {
		return "", fmt.Errorf("proc: pid %d: %w", pid, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != t.UID {
		return "", fmt.Errorf("%w: pid %d", ErrOtherUser, pid)
	}
	return d, nil
}

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxInfoBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxInfoBytes {
		return nil, fmt.Errorf("proc: %s is too large", filepath.Base(path))
	}
	return data, nil
}

// FlatpakID returns the Flatpak app id pid runs in, or "" when it runs
// outside any Flatpak (neither a /.flatpak-info nor a Flatpak scope).
func (t Table) FlatpakID(pid int) (string, error) {
	d, err := t.dir(pid)
	if err != nil {
		return "", err
	}
	fromInfo := ""
	data, err := readBounded(filepath.Join(d, "root", ".flatpak-info"))
	switch {
	case err == nil:
		fromInfo, err = InfoAppID(data)
		if err != nil {
			return "", fmt.Errorf("proc: pid %d: %w", pid, err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return "", fmt.Errorf("proc: pid %d .flatpak-info: %w", pid, err)
	}
	fromScope := ""
	if cg, err := readBounded(filepath.Join(d, "cgroup")); err == nil {
		fromScope = ScopeAppID(cg)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("proc: pid %d cgroup: %w", pid, err)
	}
	switch {
	case fromInfo != "" && fromScope != "" && fromInfo != fromScope:
		return "", fmt.Errorf("proc: pid %d: .flatpak-info names %s but its scope names %s", pid, fromInfo, fromScope)
	case fromInfo != "":
		return fromInfo, nil
	}
	return fromScope, nil
}

// InfoAppID is `name` in the [Application] group of a .flatpak-info file
// (GKeyFile syntax). A file without it, or with a malformed id, is an error.
func InfoAppID(data []byte) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	group := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			group = line[1 : len(line)-1]
			continue
		}
		if group != "Application" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == "name" {
			id := strings.TrimSpace(v)
			if !ValidAppID(id) {
				return "", fmt.Errorf(".flatpak-info names an invalid app id")
			}
			return id, nil
		}
	}
	return "", errors.New(".flatpak-info has no [Application] name")
}

// ScopeAppID reads the Flatpak app id from a /proc/<pid>/cgroup file: the
// last path element app-flatpak-<id>-<n>.scope (systemd escapes '-' in the
// id as \x2d). "" when the process is in no such scope.
func ScopeAppID(cgroup []byte) string {
	for _, line := range strings.Split(string(cgroup), "\n") {
		// hierarchy-ID:controllers:path
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 {
			continue
		}
		for _, el := range strings.Split(parts[2], "/") {
			rest, ok := strings.CutPrefix(el, "app-flatpak-")
			if !ok {
				continue
			}
			rest, ok = strings.CutSuffix(rest, ".scope")
			if !ok {
				continue
			}
			i := strings.LastIndexByte(rest, '-')
			if i <= 0 {
				continue
			}
			if _, err := strconv.Atoi(rest[i+1:]); err != nil {
				continue
			}
			id := strings.ReplaceAll(rest[:i], `\x2d`, "-")
			if ValidAppID(id) {
				return id
			}
		}
	}
	return ""
}

// ValidAppID: a reverse-DNS Flatpak id (at least three dot-separated
// elements of letters, digits, '_' and '-', at most 255 bytes).
func ValidAppID(id string) bool {
	if len(id) == 0 || len(id) > 255 {
		return false
	}
	els := strings.Split(id, ".")
	if len(els) < 3 {
		return false
	}
	for _, e := range els {
		if e == "" {
			return false
		}
		for _, r := range e {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
				return false
			}
		}
	}
	return true
}

// Parent returns pid's parent process id from /proc/<pid>/stat.
func (t Table) Parent(pid int) (int, error) {
	d, err := t.dir(pid)
	if err != nil {
		return 0, err
	}
	data, err := readBounded(filepath.Join(d, "stat"))
	if err != nil {
		return 0, fmt.Errorf("proc: pid %d stat: %w", pid, err)
	}
	// "pid (comm) state ppid ...": comm may hold spaces and parentheses,
	// so the fields start after the last ')'.
	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return 0, fmt.Errorf("proc: pid %d stat is malformed", pid)
	}
	fields := strings.Fields(string(data[i+1:]))
	if len(fields) < 2 {
		return 0, fmt.Errorf("proc: pid %d stat is malformed", pid)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("proc: pid %d stat is malformed", pid)
	}
	return ppid, nil
}

// DescendsFrom reports whether pid is root or one of root's descendants,
// walking parents while they belong to this user. A parent of another user
// (or init) ends the walk with false.
func (t Table) DescendsFrom(pid, root int) (bool, error) {
	if root <= 1 {
		return false, fmt.Errorf("proc: invalid root pid %d", root)
	}
	cur := pid
	for range maxDepth {
		if cur == root {
			return true, nil
		}
		if cur <= 1 {
			return false, nil
		}
		next, err := t.Parent(cur)
		if errors.Is(err, ErrOtherUser) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		cur = next
	}
	return false, nil
}
