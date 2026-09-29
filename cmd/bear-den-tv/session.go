// `bear-den-tv session` and `dev`: wire and run the coordinator, shell
// supervisor and LAN remote (docs/HOW_IT_WORKS.md).

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/flatpak"
	"bear-den-tv/internal/applications/tuning"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/doctor"
	"bear-den-tv/internal/pairing"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/audio"
	"bear-den-tv/internal/platform/dbusx"
	"bear-den-tv/internal/platform/detect"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/platform/lock"
	"bear-den-tv/internal/platform/mpris"
	"bear-den-tv/internal/platform/suspend"
	"bear-den-tv/internal/platform/x11"
	"bear-den-tv/internal/providers"
	"bear-den-tv/internal/providers/fixtures"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/session"
	"bear-den-tv/internal/shellipc"
	"bear-den-tv/internal/storage"
	"bear-den-tv/internal/themes"
	"bear-den-tv/internal/weather"
)

type sessionFlags struct {
	dev         bool
	devFixtures bool
	devListen   string
	dataDir     string
	noShell     bool
	shellBinary string
	verbose     bool
}

func cmdSession(args []string, dev bool) error {
	f := sessionFlags{dev: dev}
	fs := flag.NewFlagSet("session", flag.ExitOnError)
	fs.BoolVar(&f.noShell, "no-shell", false, "do not start or supervise the TV shell")
	fs.StringVar(&f.shellBinary, "shell-binary", "", "path to bear-den-tv-shell (default: next to this binary, then PATH)")
	fs.BoolVar(&f.verbose, "verbose", false, "debug logging")
	if dev {
		fs.BoolVar(&f.devFixtures, "dev-fixtures", false, "label the session DEMO (fixture content only)")
		fs.StringVar(&f.devListen, "dev-listen", "127.0.0.1:8787", "loopback address for the phone remote (no LAN exposure)")
		fs.StringVar(&f.dataDir, "data-dir", "", "isolated config/data root (default: a per-user temp directory)")
	} else {
		// Testing affordance: while set, the phone remote listens only on this
		// loopback address (reached over an SSH tunnel) and the consent-gated
		// LAN listener stays off.
		fs.StringVar(&f.devListen, "dev-listen", "", "serve the phone remote only on this loopback address (testing; no LAN exposure)")
	}
	_ = fs.Parse(args)
	return runSession(f)
}

func devPaths(root string) doctor.Paths {
	if root == "" {
		root = filepath.Join(os.TempDir(), fmt.Sprintf("bear-den-tv-dev-%d", os.Getuid()))
	}
	return doctor.Paths{
		ConfigDir: filepath.Join(root, "config"),
		DataDir:   filepath.Join(root, "data"),
		CacheDir:  filepath.Join(root, "cache"),
		Socket:    filepath.Join(root, "run", "shell.sock"),
	}
}

func runSession(f sessionFlags) error {
	level := slog.LevelInfo
	if f.verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	paths := doctor.DefaultPaths()
	if f.dev {
		paths = devPaths(f.dataDir)
	}

	// Configuration. OnChange runs after every write; wire it once the
	// coordinator and listener exist.
	var onConfig func()
	var onConfigMu sync.Mutex
	store, err := config.Open(config.Options{
		Dir:    paths.ConfigDir,
		Rules:  config.Rules{Interfaces: config.HostInterfaces{}},
		Logger: log,
		OnChange: func() {
			onConfigMu.Lock()
			fn := onConfig
			onConfigMu.Unlock()
			if fn != nil {
				fn()
			}
		},
	})
	if err != nil {
		return err
	}
	report, err := store.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	log.Info("config loaded", "source", report.Source, "revision", store.Revision(), "dir", paths.ConfigDir)
	if f.dev && f.devFixtures && report.Initialized {
		// Fresh dev config: show the DEMO content rows and DEMO weather so they
		// can be seen and tested.
		if _, err := store.Update(func(c *config.Config) error {
			for i := range c.Sections {
				if c.Sections[i].Kind != "applications" {
					c.Sections[i].Enabled = true
				}
			}
			demo := weather.DemoPlace
			c.Weather = &config.Weather{Enabled: true, Place: &demo, Units: config.UnitsCelsius, Scene: true}
			return nil
		}); err != nil {
			log.Warn("dev: could not enable demo sections", "err", err)
		}
	}

	// Optional home-screen content. Only DEMO fixtures exist so far; the Plex
	// connector is wired once its token flow is live-validated.
	var feed *providers.Feed
	if f.dev && f.devFixtures {
		feed = providers.NewFeed(fixtures.New(filepath.Join(paths.CacheDir, "artwork")), contentSections(store.Current()), providers.FeedOptions{Logger: log})
		go feed.Run(ctx, 5*time.Minute)
	}

	// Local weather. dev never touches the network: the DEMO fixture source.
	// A session contacts Open-Meteo only while config weather.enabled is true.
	var wsrc weather.Source = weather.NewClient(weather.ClientOptions{UserAgent: "bear-den-tv/" + Version})
	if f.dev {
		wsrc = weather.Fixture{}
	}
	wx := weather.New(weather.Options{Source: wsrc, Logger: log})
	wx.Configure(store.Current().WeatherSettings())
	go wx.Run(ctx)

	db, err := storage.Open(filepath.Join(paths.DataDir, "state.db"))
	if err != nil {
		return fmt.Errorf("opening state database: %w", err)
	}
	defer db.Close()

	host := &remoteHost{log: log, store: store, devListen: f.devListen, dev: f.dev}
	pair, err := pairing.New(pairing.Options{
		DB: db, Logger: log,
		Limits: func() pairing.Limits {
			r := store.Current().Remote
			return pairing.Limits{Expiry: time.Duration(r.PairingExpirySeconds) * time.Second, MaxAttempts: r.PairingMaxAttempts}
		},
		BaseURL:  host.baseURL,
		OnChange: func() { host.publish() },
	})
	if err != nil {
		return err
	}

	// Desktop backends.
	var (
		desk     platform.DesktopAdapter
		lockObs  platform.LockObserver
		audioB   platform.AudioBackend
		media    platform.MediaLocator
		launcher applications.Launcher
		display  platform.DisplayPower // sleep timer and display.off
		suspendR *platform.Capability  // what logind says about suspending
	)
	if f.dev {
		fd := fake.New()
		shellWin := fd.AddWindow(platform.WindowInfo{PID: 0, Class: []string{"bear-den-tv-shell", "bear-den-tv-shell"}, Title: session.ShellLabel})
		fd.SetActive(shellWin)
		desk, launcher = fd, fake.NewLauncher(fd)
		display = fake.NewDisplay()
		suspendR = &platform.Capability{Backend: "dev", Reason: "Development session: suspend is never offered."}
		log.Info("dev: fake desktop with a synthetic shell window; nothing touches the real display")
		if f.devFixtures {
			// DEMO players so the phone's Now playing card and media
			// buttons have something to show; never on a real session.
			media = fake.DemoMedia(clock.Real{}, adapters.PlexHTPCFlatpakID, adapters.VacuumTubeFlatpakID)
		}
	} else {
		lo := lock.New(ctx, lock.Options{})
		defer lo.Close()
		lockObs = lo
		desk = detect.NewDesktopAdapter(ctx, detect.Options{Lock: lo})
		launcher = flatpak.New(flatpak.Options{})
		audioB = audio.New(audio.ExecRunner)
		if bus, err := dbusx.ConnectSession(ctx); err == nil {
			media = mpris.NewLocator(bus)
		} else {
			log.Warn("no session bus; media controls unavailable", "err", err)
		}
		if desk.DisplaySession() == detect.SessionX11 {
			// Its own X connection; restores the DPMS state on Close.
			dp := x11.NewDisplayPower("")
			defer dp.Close()
			display = dp
			if cp := dp.Capability(); !cp.Available {
				log.Info("display power unavailable", "reason", cp.Reason)
			}
		}
		// Ask only: Bear Den never suspends and never handles a password.
		var sysBus dbusx.Bus // stays a nil interface without a system bus
		if conn, err := dbusx.ConnectSystem(ctx); err == nil {
			sysBus = conn
			defer conn.Close()
		}
		cp, answer := suspend.Check(ctx, sysBus)
		suspendR = &cp
		log.Info("suspend", "logind_can_suspend", answer, "reason", cp.Reason)
	}
	defer desk.Close()

	// Shell supervisor.
	var coord *session.Coordinator
	var sup *shellipc.Supervisor
	if !f.noShell {
		bin, err := shellipc.LookupBinary(f.shellBinary)
		if err != nil {
			return fmt.Errorf("%w (use --no-shell to run without the TV shell)", err)
		}
		extra := map[string]string{}
		var shellArgs []string
		if f.dev {
			extra["BDTV_SHELL_SOCKET"] = paths.Socket
			shellArgs = append(shellArgs, "--dev")
		}
		sup = shellipc.NewSupervisor(shellipc.SupervisorOptions{
			Binary: bin, Args: shellArgs, Env: shellipc.ShellEnvironment(os.Environ(), extra), Logger: log,
			Stdout: os.Stderr, Stderr: os.Stderr,
			OnState: func(st string) {
				if coord != nil {
					coord.SetShellState(st)
				}
			},
		})
	}

	themeReg, themeProblems := themes.LoadDefault()
	for _, p := range themeProblems {
		log.Warn("themes: skipped a theme package", "problem", p.String())
	}
	host.themeAssets = themeReg.Assets()
	var tuner session.Tuner
	if fl, ok := launcher.(*flatpak.Launcher); ok && !f.dev {
		det := tuning.NewDetector(fl)
		// Settings chosen by hand (config playback.overrides) win over detection.
		det.Overrides = func(adapter string) tuning.Overrides { return store.Current().PlaybackOverrides(adapter) }
		tuner = det
	}
	coord = session.New(session.Options{
		Themes: themeReg, Tuner: tuner,
		Logger: log, Desktop: desk, Lock: lockObs, Audio: audioB, Media: media, Display: display, Suspend: suspendR,
		Launcher: launcher, Adapters: adapters.NewRegistry(), Config: store, Pairing: pair,
		Supervisor: sup, DevMode: f.dev && f.devFixtures, Feed: feed, Weather: wx,
		Diagnostics: func(ctx context.Context) map[string]any {
			return doctor.Report(ctx, doctor.Options{Paths: paths, Version: Version, ShellBinary: f.shellBinary})
		},
	})
	host.coord = coord
	host.pair = pair

	srv, err := shellipc.Listen(shellipc.Options{SocketPath: paths.Socket, Handler: coord.Shell(), Logger: log, CoordinatorVersion: Version})
	if err != nil {
		return fmt.Errorf("shell socket: %w", err)
	}
	defer srv.Close()
	coord.AttachShellServer(srv)
	log.Info("shell socket listening", "path", paths.Socket)

	onConfigMu.Lock()
	onConfig = func() {
		wx.Configure(store.Current().WeatherSettings())
		if feed != nil {
			feed.SetSections(contentSections(store.Current()))
			go feed.Refresh(ctx)
		}
		host.reconcile(ctx)
		coord.SetRemoteStatus(host.status())
	}
	onConfigMu.Unlock()
	host.reconcile(ctx)
	defer host.stop()

	if sup != nil {
		if err := sup.Start(); err != nil {
			return fmt.Errorf("starting shell: %w", err)
		}
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if sh := srv.Shell(); sh != nil {
				_ = sh.Send(shellipc.Shutdown{Type: shellipc.TypeShutdown, Reason: "maintenance"})
			}
			sup.Stop(sctx)
		}()
	}
	return coord.Run(ctx)
}

// remoteHost owns the phone-remote listener and keeps it in line with the
// configuration: nothing listens on the LAN until remote.enabled and
// onboarding.lan_consent are both true. `dev` binds loopback only.
type remoteHost struct {
	themeAssets fs.FS // theme art for phones (/themes/)
	log         *slog.Logger
	store       *config.Store
	coord       *session.Coordinator
	pair        *pairing.Service
	dev         bool
	devListen   string

	mu      sync.Mutex
	desired any
	cancel  context.CancelFunc
	done    chan struct{}
	urls    []string
}

type listenSpec struct {
	Interfaces   []string
	Dev          string
	Port         int
	Transport    string
	LayoutHTTP   bool
	AllowedHosts []string
	Cert, Key    string
}

func (h *remoteHost) spec() (listenSpec, string) {
	cfg := h.store.Current()
	r := cfg.Remote
	if h.devListen != "" {
		// Loopback only (BindDev refuses anything else); dev or session testing.
		return listenSpec{Dev: h.devListen, Transport: remote.TransportTrustedLANHTTP, LayoutHTTP: h.dev}, ""
	}
	if !r.Enabled || !cfg.Onboarding.LANConsent {
		return listenSpec{}, "phone remote disabled until enabled with LAN consent"
	}
	if reason := h.store.RemoteBlocked(); reason != "" {
		return listenSpec{}, reason
	}
	s := listenSpec{Interfaces: append([]string(nil), r.Interfaces...), Port: r.Port, Transport: r.Transport, LayoutHTTP: r.HTTPLayoutEditing, AllowedHosts: append([]string(nil), r.AllowedHosts...)}
	if r.HTTPS.CertificateFile != nil && r.HTTPS.PrivateKeyFile != nil {
		s.Cert, s.Key = *r.HTTPS.CertificateFile, *r.HTTPS.PrivateKeyFile
	}
	return s, ""
}

func (h *remoteHost) reconcile(ctx context.Context) {
	want, why := h.spec()
	h.mu.Lock()
	defer h.mu.Unlock()
	if reflect.DeepEqual(want, h.desired) && (h.cancel != nil || why != "") {
		return
	}
	h.stopLocked()
	h.desired = want
	if why != "" {
		h.log.Info("remote: not listening", "reason", why)
		return
	}
	var (
		ls  []net.Listener
		err error
	)
	if want.Dev != "" {
		ls, _, err = remote.BindDev(want.Dev)
	} else {
		for _, iface := range want.Interfaces {
			l, _, e := remote.BindInterface(iface, want.Port)
			if e != nil {
				err = errors.Join(err, e)
				continue
			}
			ls = append(ls, l...)
		}
	}
	if err != nil || len(ls) == 0 {
		for _, l := range ls {
			_ = l.Close()
		}
		h.log.Error("remote: bind failed", "err", err)
		return
	}
	var tlsCfg *tls.Config
	if want.Transport == remote.TransportHTTPS {
		if tlsCfg, err = remote.LoadTLS(want.Cert, want.Key); err != nil {
			h.log.Error("remote: TLS", "err", err)
			for _, l := range ls {
				_ = l.Close()
			}
			return
		}
	}
	srv, err := remote.New(remote.Options{
		Backend: h.coord.Phones(), Devices: h.pair, Transport: want.Transport,
		HTTPLayoutEditing: want.LayoutHTTP, AllowedHosts: append(remote.ListenerHosts(ls), want.AllowedHosts...),
		DevMode: h.dev, Logger: h.log, ThemeAssets: h.themeAssets,
	})
	if err != nil {
		h.log.Error("remote: server", "err", err)
		for _, l := range ls {
			_ = l.Close()
		}
		return
	}
	sctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	h.cancel, h.done = cancel, done
	h.urls = remote.ListenerURLs(ls, tlsCfg != nil)
	go func() {
		defer close(done)
		if err := srv.ListenAndServe(sctx, ls, tlsCfg); err != nil && !errors.Is(err, context.Canceled) {
			h.log.Error("remote: server stopped", "err", err)
		}
		_ = srv.Close()
	}()
	h.log.Info("remote: listening", "urls", h.urls, "transport", want.Transport)
	go h.coord.SetRemoteStatus(true, append([]string(nil), h.urls...))
}

func (h *remoteHost) stopLocked() {
	if h.cancel != nil {
		h.cancel()
		<-h.done
		h.cancel, h.done = nil, nil
	}
	h.urls = nil
}

func (h *remoteHost) stop() {
	h.mu.Lock()
	h.stopLocked()
	h.mu.Unlock()
}

func (h *remoteHost) status() (bool, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cancel != nil, append([]string(nil), h.urls...)
}

func (h *remoteHost) baseURL() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.urls) == 0 {
		return ""
	}
	return h.urls[0]
}

func (h *remoteHost) publish() {
	if h.coord != nil {
		h.coord.SetRemoteStatus(h.status())
	}
}

// contentSections lists the enabled provider-backed sections of the layout.
func contentSections(cfg config.Config) []providers.SectionConfig {
	var out []providers.SectionConfig
	for _, s := range cfg.Sections {
		if !s.Enabled || s.Kind == "applications" {
			continue
		}
		out = append(out, providers.SectionConfig{ID: s.ID, Kind: s.Kind, LibraryIDs: cfg.PlexContent.LibraryIDs, Limit: 20})
	}
	return out
}
