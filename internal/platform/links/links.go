// Package links makes Bear Den the desktop's handler for http and https
// links (docs/operations.md "Sign-ins on the TV") and hands on the links it
// does not open itself. `bear-den-tv open-url`, the handler, asks the running
// coordinator first (IPC link.open: a link an app opens while Bear Den or one
// of its apps is in front shows on the TV); anything else goes to the
// browser that was the default before Bear Den took over, run from its own
// desktop entry (Forward), never through xdg-open, which would come back
// here.
//
// The handler's desktop entry ships in the .deb
// (/usr/share/applications/bear-den-tv-links.desktop); a checkout writes
// its own in the user's applications folder. Removing the package removes
// the entry, and the desktop falls back to its own browser by itself. Ensure
// (the coordinator at start, `bear-den-tv links enable`) makes it the
// default through xdg-mime and remembers the one before; Disable (`bear-den-tv
// links disable`) puts that one back and keeps Bear Den from taking over again.
package links

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// DesktopID is the handler's desktop entry.
const DesktopID = "bear-den-tv-links.desktop"

// SystemEntry is where the .deb installs it.
const SystemEntry = "/usr/share/applications/" + DesktopID

// Schemes are the link types Bear Den handles.
var Schemes = []string{"x-scheme-handler/http", "x-scheme-handler/https"}

// Runner runs a program and returns its standard output (exec in
// production; tests fake xdg-mime).
type Runner func(ctx context.Context, name string, args ...string) (string, error)

// ExecRunner runs name with args.
func ExecRunner(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// Paths are the XDG folders the handler uses.
type Paths struct {
	DataHome  string   // $XDG_DATA_HOME: applications/ for a checkout's entry
	StateHome string   // $XDG_STATE_HOME: bear-den-tv/links.json
	DataDirs  []string // $XDG_DATA_DIRS: where desktop entries are found
	// System is the .deb's entry (SystemEntry; tests point it elsewhere).
	System string
}

// DefaultPaths reads the XDG variables, with the specification's defaults,
// and adds Flatpak's exported applications to the data folders.
func DefaultPaths() Paths {
	home, _ := os.UserHomeDir()
	p := Paths{
		DataHome:  envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share")),
		StateHome: envOr("XDG_STATE_HOME", filepath.Join(home, ".local", "state")),
		System:    SystemEntry,
	}
	dirs := strings.Split(envOr("XDG_DATA_DIRS", "/usr/local/share:/usr/share"), ":")
	dirs = append(dirs, filepath.Join(p.DataHome, "flatpak", "exports", "share"), "/var/lib/flatpak/exports/share")
	for _, d := range dirs {
		if d != "" && !contains(p.DataDirs, d) {
			p.DataDirs = append(p.DataDirs, d)
		}
	}
	return p
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// State is what Bear Den remembers ($XDG_STATE_HOME/bear-den-tv/links.json).
type State struct {
	// Previous is the desktop entry that handled https links before Bear
	// Den took over ("" when there was none).
	Previous string `json:"previous,omitempty"`
	// Off: the owner turned the handler off (`bear-den-tv links disable`);
	// Ensure does nothing until `links enable`.
	Off bool `json:"off,omitempty"`
}

func (p Paths) stateFile() string { return filepath.Join(p.StateHome, "bear-den-tv", "links.json") }
func (p Paths) userEntry() string { return filepath.Join(p.DataHome, "applications", DesktopID) }

// Load reads the state; a missing file is the zero State.
func Load(p Paths) (State, error) {
	var st State
	b, err := os.ReadFile(p.stateFile())
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(b, &st)
}

func save(p Paths, st State) error {
	if err := os.MkdirAll(filepath.Dir(p.stateFile()), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(st)
	return os.WriteFile(p.stateFile(), append(b, '\n'), 0o600)
}

// Default asks the desktop which entry handles https links ("" for none).
func Default(ctx context.Context, run Runner) (string, error) {
	out, err := run(ctx, "xdg-mime", "query", "default", "x-scheme-handler/https")
	if err != nil {
		return "", fmt.Errorf("links: xdg-mime: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// UserEntry is the desktop entry a checkout writes for its binary exe.
func UserEntry(exe string) string {
	return "[Desktop Entry]\nType=Application\nName=Bear Den TV links\n" +
		"Comment=Opens web links on the TV while Bear Den is in front, else in your usual browser\n" +
		"Exec=" + quoteExec(exe) + " open-url %u\nNoDisplay=true\nTerminal=false\nStartupNotify=false\n" +
		"MimeType=x-scheme-handler/http;x-scheme-handler/https;\n"
}

// quoteExec quotes a path for an Exec line when it needs it.
func quoteExec(path string) string {
	if !strings.ContainsAny(path, " \t\"'\\`$") {
		return path
	}
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`)
	return `"` + r.Replace(path) + `"`
}

// ensureEntry makes the handler's entry exist: the .deb's, else one in the
// user's applications folder for exe. With the .deb's present, a leftover
// user entry (from a checkout) is removed so it cannot shadow it.
func ensureEntry(p Paths, exe string) error {
	if _, err := os.Stat(p.System); err == nil {
		if err := os.Remove(p.userEntry()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if exe == "" {
		return errors.New("links: no handler entry and no binary to write one for")
	}
	if err := os.MkdirAll(filepath.Dir(p.userEntry()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p.userEntry(), []byte(UserEntry(exe)), 0o644)
}

// Ensure makes Bear Den the desktop's handler for http and https links,
// remembering the one before, unless the owner turned it off. It reports
// whether it changed the default.
func Ensure(ctx context.Context, p Paths, exe string, run Runner) (bool, error) {
	st, err := Load(p)
	if err != nil {
		return false, err
	}
	if st.Off {
		return false, nil
	}
	if err := ensureEntry(p, exe); err != nil {
		return false, err
	}
	cur, err := Default(ctx, run)
	if err != nil {
		return false, err
	}
	if cur == DesktopID {
		return false, nil
	}
	st.Previous = cur
	if err := save(p, st); err != nil {
		return false, err
	}
	if _, err := run(ctx, "xdg-mime", append([]string{"default", DesktopID}, Schemes...)...); err != nil {
		return false, fmt.Errorf("links: xdg-mime: %w", err)
	}
	return true, nil
}

// Enable is Ensure after the owner asked: it clears Off first.
func Enable(ctx context.Context, p Paths, exe string, run Runner) (bool, error) {
	st, err := Load(p)
	if err != nil {
		return false, err
	}
	if st.Off {
		st.Off = false
		if err := save(p, st); err != nil {
			return false, err
		}
	}
	return Ensure(ctx, p, exe, run)
}

// Disable gives http and https links back to the browser that had them
// before and keeps Bear Den from taking them again (Off). With no browser
// remembered, the default stays as it is and the error says to choose one in
// the desktop's settings.
func Disable(ctx context.Context, p Paths, run Runner) error {
	st, err := Load(p)
	if err != nil {
		return err
	}
	st.Off = true
	if err := save(p, st); err != nil {
		return err
	}
	if err := os.Remove(p.userEntry()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cur, err := Default(ctx, run)
	if err != nil || cur != DesktopID {
		return err
	}
	if st.Previous == "" || st.Previous == DesktopID {
		return errors.New("links: no browser was the default before Bear Den; choose one in the desktop's settings")
	}
	if _, err := run(ctx, "xdg-mime", append([]string{"default", st.Previous}, Schemes...)...); err != nil {
		return fmt.Errorf("links: xdg-mime: %w", err)
	}
	return nil
}

// Status reports whether Bear Den handles links now, and its state.
func Status(ctx context.Context, p Paths, run Runner) (bool, State, error) {
	st, err := Load(p)
	if err != nil {
		return false, st, err
	}
	cur, err := Default(ctx, run)
	return cur == DesktopID, st, err
}

// Forward opens link in the browser that handled links before Bear Den,
// from its own desktop entry, else in the system's x-www-browser,
// sensible-browser or firefox. It never goes through xdg-open (that would
// come back here) and never to Bear Den's own entry.
func Forward(p Paths, link string, start func(argv []string) error) error {
	st, _ := Load(p)
	if st.Previous != "" && st.Previous != DesktopID {
		if argv, err := ExecFor(p, st.Previous, link); err == nil {
			return start(argv)
		}
	}
	for _, b := range []string{"x-www-browser", "sensible-browser", "firefox"} {
		if path, err := exec.LookPath(b); err == nil {
			return start([]string{path, link})
		}
	}
	return errors.New("links: no browser to open the link in")
}

// Start runs argv in its own session, detached, without waiting.
func Start(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// ExecFor finds desktop entry id in the data folders (the user's first) and
// returns its Exec command line for opening link.
func ExecFor(p Paths, id, link string) ([]string, error) {
	if strings.ContainsAny(id, "/\\") || !strings.HasSuffix(id, ".desktop") {
		return nil, fmt.Errorf("links: %q is not a desktop entry id", id)
	}
	dirs := append([]string{p.DataHome}, p.DataDirs...)
	for _, d := range dirs {
		b, err := os.ReadFile(filepath.Join(d, "applications", id))
		if err != nil {
			continue
		}
		line, ok := execLine(string(b))
		if !ok {
			return nil, fmt.Errorf("links: %s has no Exec line", id)
		}
		return ExecArgv(line, link)
	}
	return nil, fmt.Errorf("links: %s not found", id)
}

// execLine is the Exec key of the [Desktop Entry] group.
func execLine(entry string) (string, bool) {
	group := ""
	for _, raw := range strings.Split(entry, "\n") {
		l := strings.TrimSpace(raw)
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			group = l
			continue
		}
		if group != "[Desktop Entry]" {
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok && strings.TrimSpace(k) == "Exec" {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// ExecArgv turns a desktop entry's Exec value into an argument list opening
// link (Desktop Entry Specification, "The Exec key"): the string escapes are
// undone, the line is split on spaces with double-quoted arguments, %u %U
// %f %F become link, %% a percent sign, and the other field codes are
// dropped. With no file or URL code, link is added at the end.
func ExecArgv(value, link string) ([]string, error) {
	value = unescapeString(value)
	var args []string
	var cur strings.Builder
	inArg, quoted, used := false, false, false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch {
		case quoted && ch == '\\' && i+1 < len(value) && strings.IndexByte("\"`$\\", value[i+1]) >= 0:
			cur.WriteByte(value[i+1])
			i++
		case ch == '"':
			quoted = !quoted
			inArg = true
		case !quoted && (ch == ' ' || ch == '\t'):
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		case ch == '%' && i+1 < len(value):
			code := value[i+1]
			i++
			inArg = true
			switch code {
			case 'u', 'U', 'f', 'F':
				cur.WriteString(link)
				used = true
			case '%':
				cur.WriteByte('%')
			}
		default:
			cur.WriteByte(ch)
			inArg = true
		}
	}
	if quoted {
		return nil, errors.New("links: unterminated quote in Exec")
	}
	if inArg {
		args = append(args, cur.String())
	}
	// An argument that was only a dropped field code (%i, %c, %k) leaves
	// an empty string behind: drop those.
	out := args[:0]
	for _, a := range args {
		if a != "" {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("links: empty Exec")
	}
	if !used {
		out = append(out, link)
	}
	return out, nil
}

// unescapeString undoes the desktop entry string escapes (\s \n \t \r \\);
// any other backslash stays for the quoting rules.
func unescapeString(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i+1 >= len(v) {
			b.WriteByte(v[i])
			continue
		}
		switch v[i+1] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(v[i+1])
		}
		i++
	}
	return b.String()
}
