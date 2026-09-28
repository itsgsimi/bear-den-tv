// Package lock observes desktop-session lock state from two independent
// sources: the session-bus screensaver (org.freedesktop.ScreenSaver, served by
// light-locker or xfce4-screensaver) and the logind session's LockedHint and
// Active properties on the system bus. The session counts as locked when any
// source says so, or when the logind session is not the active seat session
// (the TV is then showing the greeter or another VT, so remote input must not
// be delivered). When no source is reachable the observer fails closed:
// Locked reports an ErrUnsupported-wrapped error and the capability says why.
package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/dbusx"
)

const (
	screensaverDest  = "org.freedesktop.ScreenSaver"
	screensaverPath  = "/org/freedesktop/ScreenSaver"
	screensaverIface = "org.freedesktop.ScreenSaver"

	logindDest         = "org.freedesktop.login1"
	logindManagerPath  = "/org/freedesktop/login1"
	logindManagerIface = "org.freedesktop.login1.Manager"
	logindSessionIface = "org.freedesktop.login1.Session"
	propertiesIface    = "org.freedesktop.DBus.Properties"

	// Backend is the capability backend name reported by Observer.
	Backend = "dbus-screensaver+logind"
)

// Options configures New. Zero values connect to the real buses and resolve
// the logind session from XDG_SESSION_ID, then from the process pid.
type Options struct {
	// ConnectSession dials the session bus; nil uses dbusx.ConnectSession.
	ConnectSession func(ctx context.Context) (dbusx.Bus, error)
	// ConnectSystem dials the system bus; nil uses dbusx.ConnectSystem.
	ConnectSystem func(ctx context.Context) (dbusx.Bus, error)
	// SessionID pins the logind session; empty consults Getenv("XDG_SESSION_ID").
	SessionID string
	// PID is used for GetSessionByPID when no session id is known; 0 = os.Getpid().
	PID int
	// Display, when set (":0"), overrides the env/pid session with the logind
	// session whose Display property matches — the env session is an SSH
	// session when the probe runs remotely.
	Display string
	// Getenv reads environment variables; nil uses os.Getenv.
	Getenv func(string) string
}

// State is one observation across both sources. Pointer fields are nil when
// the source did not answer.
type State struct {
	ScreensaverActive *bool  `json:"screensaver_active"`
	ScreensaverError  string `json:"screensaver_error,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	SessionPath       string `json:"session_path,omitempty"`
	LockedHint        *bool  `json:"locked_hint"`
	SessionActive     *bool  `json:"session_active"`
	LogindError       string `json:"logind_error,omitempty"`
	// Known is true when at least one source answered.
	Known bool `json:"known"`
	// Locked is the fail-closed verdict: any source locked, or session inactive.
	Locked bool `json:"locked"`
}

// Observer implements platform.LockObserver over the two bus sources.
type Observer struct {
	session dbusx.Bus
	system  dbusx.Bus

	sessionErr error
	systemErr  error

	sessionID   string
	sessionPath string
	resolveErr  error

	mu   sync.Mutex
	last *bool
}

// New connects both buses and resolves the logind session. It never fails:
// unreachable sources are recorded and reported through Capability, Locked,
// and State.
func New(ctx context.Context, opts Options) *Observer {
	o := &Observer{}
	connectSession := opts.ConnectSession
	if connectSession == nil {
		connectSession = func(ctx context.Context) (dbusx.Bus, error) { return dbusx.ConnectSession(ctx) }
	}
	connectSystem := opts.ConnectSystem
	if connectSystem == nil {
		connectSystem = func(ctx context.Context) (dbusx.Bus, error) { return dbusx.ConnectSystem(ctx) }
	}
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	o.session, o.sessionErr = connectSession(ctx)
	o.system, o.systemErr = connectSystem(ctx)
	if o.systemErr == nil {
		pid := opts.PID
		if pid == 0 {
			pid = os.Getpid()
		}
		id := opts.SessionID
		if id == "" {
			id = getenv("XDG_SESSION_ID")
		}
		o.sessionID, o.sessionPath, o.resolveErr = resolveSession(ctx, o.system, id, pid, opts.Display)
	}
	return o
}

// resolveSession finds the logind session object path: by id when known,
// otherwise by pid; when display is set and the found session's Display
// differs, the session owning that display wins.
func resolveSession(ctx context.Context, bus dbusx.Bus, id string, pid int, display string) (string, string, error) {
	var path string
	var err error
	if id != "" {
		path, err = callPath(ctx, bus, logindManagerIface+".GetSession", id)
	} else {
		path, err = callPath(ctx, bus, logindManagerIface+".GetSessionByPID", uint32(pid))
		if err == nil {
			if v, perr := bus.Property(ctx, logindDest, path, logindSessionIface, "Id"); perr == nil {
				id, _ = v.(string)
			}
		}
	}
	if display != "" {
		if path != "" {
			if v, perr := bus.Property(ctx, logindDest, path, logindSessionIface, "Display"); perr == nil {
				if d, _ := v.(string); d == display {
					return id, path, nil
				}
			}
		}
		if did, dpath, derr := sessionByDisplay(ctx, bus, display); derr == nil {
			return did, dpath, nil
		} else if err == nil {
			err = derr
		}
	}
	if err != nil {
		return id, "", err
	}
	return id, path, nil
}

func callPath(ctx context.Context, bus dbusx.Bus, method string, arg any) (string, error) {
	body, err := bus.Call(ctx, logindDest, logindManagerPath, method, arg)
	if err != nil {
		return "", err
	}
	if len(body) != 1 {
		return "", fmt.Errorf("lock: %s returned %d values", method, len(body))
	}
	return fmt.Sprint(body[0]), nil
}

// sessionByDisplay scans Manager.ListSessions for the session whose Display
// property equals display.
func sessionByDisplay(ctx context.Context, bus dbusx.Bus, display string) (string, string, error) {
	body, err := bus.Call(ctx, logindDest, logindManagerPath, logindManagerIface+".ListSessions")
	if err != nil {
		return "", "", err
	}
	if len(body) != 1 {
		return "", "", errors.New("lock: ListSessions returned no list")
	}
	rows, ok := body[0].([][]any)
	if !ok {
		return "", "", fmt.Errorf("lock: ListSessions returned %T", body[0])
	}
	for _, row := range rows {
		if len(row) < 5 {
			continue
		}
		id := fmt.Sprint(row[0])
		path := fmt.Sprint(row[4])
		v, perr := bus.Property(ctx, logindDest, path, logindSessionIface, "Display")
		if perr != nil {
			continue
		}
		if d, _ := v.(string); d == display {
			return id, path, nil
		}
	}
	return "", "", fmt.Errorf("lock: no logind session owns display %q", display)
}

// Capability reports whether lock observation has at least one source.
func (o *Observer) Capability() platform.Capability {
	var sources, problems []string
	if o.sessionErr == nil {
		sources = append(sources, "screensaver")
	} else {
		problems = append(problems, "session bus: "+o.sessionErr.Error())
	}
	switch {
	case o.systemErr != nil:
		problems = append(problems, "system bus: "+o.systemErr.Error())
	case o.resolveErr != nil:
		problems = append(problems, "logind session: "+o.resolveErr.Error())
	default:
		sources = append(sources, "logind")
	}
	if len(sources) == 0 {
		return platform.Capability{Available: false, Backend: Backend, Reason: "no lock source reachable (" + strings.Join(problems, "; ") + ")"}
	}
	c := platform.Capability{Available: true, Backend: Backend}
	if len(problems) > 0 {
		c.Reason = "sources: " + strings.Join(sources, ",") + "; degraded: " + strings.Join(problems, "; ")
	}
	return c
}

// State queries both sources once.
func (o *Observer) State(ctx context.Context) State {
	st := State{SessionID: o.sessionID, SessionPath: o.sessionPath}
	if o.sessionErr != nil {
		st.ScreensaverError = o.sessionErr.Error()
	} else {
		body, err := o.session.Call(ctx, screensaverDest, screensaverPath, screensaverIface+".GetActive")
		switch {
		case err != nil && dbusx.IsNoOwner(err):
			st.ScreensaverError = "no screensaver on the session bus"
		case err != nil:
			st.ScreensaverError = err.Error()
		case len(body) != 1:
			st.ScreensaverError = fmt.Sprintf("GetActive returned %d values", len(body))
		default:
			if b, ok := body[0].(bool); ok {
				st.ScreensaverActive = &b
				st.Known = true
			} else {
				st.ScreensaverError = fmt.Sprintf("GetActive returned %T", body[0])
			}
		}
	}
	switch {
	case o.systemErr != nil:
		st.LogindError = o.systemErr.Error()
	case o.resolveErr != nil:
		st.LogindError = o.resolveErr.Error()
	default:
		if b, err := o.boolProp(ctx, "LockedHint"); err != nil {
			st.LogindError = err.Error()
		} else {
			st.LockedHint = &b
			st.Known = true
		}
		if b, err := o.boolProp(ctx, "Active"); err != nil {
			if st.LogindError == "" {
				st.LogindError = err.Error()
			}
		} else {
			st.SessionActive = &b
			st.Known = true
		}
	}
	st.Locked = (st.ScreensaverActive != nil && *st.ScreensaverActive) ||
		(st.LockedHint != nil && *st.LockedHint) ||
		(st.SessionActive != nil && !*st.SessionActive)
	return st
}

func (o *Observer) boolProp(ctx context.Context, name string) (bool, error) {
	v, err := o.system.Property(ctx, logindDest, o.sessionPath, logindSessionIface, name)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("lock: %s is %T, not bool", name, v)
	}
	return b, nil
}

// Locked implements platform.LockObserver. It returns an error wrapping
// platform.ErrUnsupported when no source answered, so callers fail closed.
func (o *Observer) Locked(ctx context.Context) (bool, error) {
	st := o.State(ctx)
	if !st.Known {
		return false, fmt.Errorf("%w: lock state unknown (screensaver: %s; logind: %s)", platform.ErrUnsupported, orNone(st.ScreensaverError), orNone(st.LogindError))
	}
	return st.Locked, nil
}

func orNone(s string) string {
	if s == "" {
		return "ok"
	}
	return s
}

// Watch implements platform.LockObserver. It emits the current verdict first,
// then a new verdict whenever a screensaver ActiveChanged signal or a logind
// PropertiesChanged signal on the session changes the answer.
func (o *Observer) Watch(ctx context.Context) (<-chan bool, error) {
	if o.sessionErr != nil && (o.systemErr != nil || o.resolveErr != nil) {
		return nil, fmt.Errorf("%w: %s", platform.ErrUnsupported, o.Capability().Reason)
	}
	var wake []<-chan dbusx.Signal
	if o.sessionErr == nil {
		ch, err := o.session.Subscribe(ctx, screensaverIface, "ActiveChanged")
		if err != nil {
			return nil, err
		}
		wake = append(wake, ch)
	}
	if o.systemErr == nil && o.resolveErr == nil {
		ch, err := o.system.Subscribe(ctx, propertiesIface, "PropertiesChanged")
		if err != nil {
			return nil, err
		}
		wake = append(wake, ch)
	}
	merged := make(chan dbusx.Signal, 16)
	var wg sync.WaitGroup
	for _, ch := range wake {
		wg.Add(1)
		go func(ch <-chan dbusx.Signal) {
			defer wg.Done()
			for s := range ch {
				if s.Name == propertiesIface+".PropertiesChanged" && s.Path != o.sessionPath {
					continue
				}
				select {
				case merged <- s:
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}
	go func() {
		wg.Wait()
		close(merged)
	}()
	out := make(chan bool, 1)
	go func() {
		defer close(out)
		emit := func() bool {
			locked, err := o.Locked(ctx)
			if err != nil {
				return true
			}
			o.mu.Lock()
			changed := o.last == nil || *o.last != locked
			o.last = &locked
			o.mu.Unlock()
			if !changed {
				return true
			}
			select {
			case out <- locked:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !emit() {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-merged:
				if !ok {
					return
				}
				if !emit() {
					return
				}
			}
		}
	}()
	return out, nil
}

// Close releases both bus connections.
func (o *Observer) Close() error {
	var errs []error
	if o.session != nil {
		errs = append(errs, o.session.Close())
	}
	if o.system != nil {
		errs = append(errs, o.system.Close())
	}
	return errors.Join(errs...)
}
