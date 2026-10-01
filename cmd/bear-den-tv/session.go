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

	"bear-den-tv/internal/achievements"
	"bear-den-tv/internal/appicons"
	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/flatpak"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/applications/tuning"
	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/doctor"
	"bear-den-tv/internal/pairing"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/audio"
	"bear-den-tv/internal/platform/autostart"
	"bear-den-tv/internal/platform/cec"
	"bear-den-tv/internal/platform/dbusx"
	"bear-den-tv/internal/platform/detect"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/platform/lock"
	"bear-den-tv/internal/platform/mpris"
	"bear-den-tv/internal/platform/proc"
	"bear-den-tv/internal/platform/suspend"
	"bear-den-tv/internal/platform/x11"
	"bear-den-tv/internal/providers"
	"bear-den-tv/internal/providers/fixtures"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/remote/mdns"
	"bear-den-tv/internal/session"
	"bear-den-tv/internal/shellipc"
	"bear-den-tv/internal/storage"
	"bear-den-tv/internal/themes"
	"bear-den-tv/internal/weather"
)

type sessionFlags struct {
	dev         bool
	devFixtures bool
	devPlexFake bool
	devListen   string
	devBrowser  string
	devInstalls bool
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
		fs.BoolVar(&f.devPlexFake, "dev-plex-fake", false, "Plex sign-in and rows against a local fake plex.tv and server (DEMO titles; links on the 3rd poll)")
		fs.StringVar(&f.devListen, "dev-listen", "127.0.0.1:8787", "loopback address for the phone remote (no LAN exposure)")
		fs.StringVar(&f.dataDir, "data-dir", "", "isolated config/data root (default: a per-user temp directory)")
		fs.StringVar(&f.devBrowser, "dev-browser", "", "run web apps in this Chromium-engine binary (a real browser window; default: a pretend page on the fake desktop)")
		fs.BoolVar(&f.devInstalls, "dev-installs", false, "Moonlight, RetroArch, Jellyfin, Google Chrome and Brave start missing and a pretend Flathub installs them (DEMO; no network)")
	} else {
		// Testing affordance: while set, the phone remote listens only on this
		// loopback address (reached over an SSH tunnel) and the consent-gated
		// LAN listener stays off.
		fs.StringVar(&f.devListen, "dev-listen", "", "serve the phone remote only on this loopback address (testing; no LAN exposure)")
	}
	_ = fs.Parse(args)
	if !dev {
		ignoreJobControl() // a daemon: never stopped by a terminal (daemon.go)
	}
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
	if !f.dev {
		// Web links an app opens (a sign-in) show on the TV (links.go).
		go ensureLinks(ctx, log)
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
	// A config.json of the Chromium era moves to Chrome (streaming) and
	// Brave (the Browser tile), written and logged once (config/upgrade.go).
	if _, err := store.UpgradeBrowsers(); err != nil {
		log.Warn("config: could not move the web apps off Chromium", "err", err)
	}
	// A config.json from an older version gains the apps this version knows
	// (hidden or off as the defaults have them; config/upgrade.go).
	if _, err := store.UpgradeApps(); err != nil {
		log.Warn("config: could not add the apps this version knows", "err", err)
	}
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

	// Optional home-screen content: DEMO fixtures behind --dev-fixtures, else
	// the Plex connector (internal/plexlink). Plex sends nothing anywhere
	// until the owner signs in from Settings → Plex; dev has it only with
	// --dev-plex-fake (a local fake, never plex.tv).
	var feed *providers.Feed
	if f.dev && f.devFixtures {
		feed = providers.NewFeed(fixtures.New(filepath.Join(paths.CacheDir, "artwork")), contentSections(store.Current()), providers.FeedOptions{Logger: log})
		go feed.Run(ctx, 5*time.Minute)
	}
	plexLink, stopPlexFake, err := newPlexLink(f, paths, store, log)
	if err != nil {
		return fmt.Errorf("plex: %w", err)
	}
	defer stopPlexFake()

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
		OnPaired: func() { host.paired() },
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
		tv       platform.TVControl    // HDMI-CEC, used only while config cec.enabled
		suspendR *platform.Capability  // what logind says about suspending
	)
	if f.dev {
		fd := fake.New()
		// What a key press reached (apps/remote-web/tests/e2e reads this line).
		fd.OnKey = func(d fake.Delivery) {
			log.Info("dev: key delivered", "key", string(d.Key), "window", fmt.Sprintf("%#x", uint64(d.Window)))
		}
		shellWin := fd.AddWindow(platform.WindowInfo{PID: 0, Class: []string{"bear-den-tv-shell", "bear-den-tv-shell"}, Title: session.ShellLabel})
		fd.SetActive(shellWin)
		desk, launcher = fd, fake.NewLauncher(fd)
		display = fake.NewDisplay()
		tv = fake.NewTV() // a pretend HDMI-CEC TV; nothing touches a real bus
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
		// HDMI-CEC through the kernel API (ADR 0008): nothing is sent unless
		// an adapter exists and the owner turned on cec.enabled.
		cecA := cec.New(cec.Options{})
		defer cecA.Close()
		tv = cecA
		if bus, err := dbusx.ConnectSession(ctx); err == nil {
			media = mpris.NewLocator(bus).WithProcesses(proc.Host()) // ownership by process (docs/security.md)
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
	// Web apps (streaming sites, the Browser tile): Google Chrome and Brave
	// from Flathub over the DevTools pipe on the TV; in dev a pretend page on
	// the fake desktop, or a real Chromium-engine binary with --dev-browser.
	var webApps session.WebApps
	dataHome := filepath.Dir(paths.DataDir) // $XDG_DATA_HOME; profiles in bear-den-tv/web-<browser>/<app-id>
	switch {
	case !f.dev:
		webApps = web.NewManager(web.Options{DataHome: dataHome, Starter: web.FlatpakStarter(dataHome), Logger: log})
	case f.devBrowser != "":
		fd := desk.(*fake.Desktop)
		webApps = devBrowser{Manager: web.NewManager(web.Options{DataHome: dataHome, Logger: log,
			Starter: web.ExecStarter{Env: flatpak.PassthroughEnv(os.Environ()), Prefix: []string{f.devBrowser}}}), desk: fd}
		log.Info("dev: web apps run in a real Chromium-engine binary", "binary", f.devBrowser)
	default:
		webApps = fake.NewWeb(desk.(*fake.Desktop))
	}
	// App installs from Flathub, per user (ADR 0011): the real installer on
	// the TV; with dev --dev-installs a pretend one; otherwise none.
	var appInstaller session.AppInstaller
	var webDRM session.WebDRM // the streaming sites' Widevine (on the TV only)
	if !f.dev {
		webDRM = &web.Widevine{DataHome: dataHome, Starter: web.FlatpakStarter(dataHome)}
	}
	var onInstallChange func()
	// Every Flatpak the adapter table names: each app's own and each
	// browser the web apps may run in (adapters.Browsers).
	tableIDs := adapters.NewRegistry().InstallableFlatpakIDs()
	switch {
	case !f.dev:
		appInstaller = install.New(install.Options{Allowed: tableIDs, OnChange: func() {
			if onInstallChange != nil {
				onInstallChange()
			}
		}})
	case f.devInstalls:
		fl := launcher.(*fake.Launcher)
		fl.SetMissing(fake.DemoMissing...)
		fi := fake.NewInstaller(fl, tableIDs)
		fi.OnChange = func() {
			if onInstallChange != nil {
				onInstallChange()
			}
		}
		appInstaller = fi
		log.Info("dev: some apps start missing; a pretend Flathub installs them (DEMO)")
	}
	var plexOpt session.PlexLink // stays nil without a connector (a nil *Manager would not)
	var plexPlaying session.PlexPlaying
	if plexLink != nil {
		plexOpt = plexLink
		plexPlaying = plexLink // Now playing for Plex HTPC from the server (session/plexplaying.go)
	}
	iconFinder := appicons.DefaultFinder() // phones: the apps' own icons
	// The TV's "Start with this PC" toggle writes the same entry as
	// `bear-den-tv autostart enable`. Not in `dev`: a development session
	// must not change the login of the machine it runs on.
	var autostartOpt *session.Autostart
	if !f.dev {
		autostartOpt = &session.Autostart{Path: autostart.File(), Script: autostart.StartScript}
	}
	coord = session.New(session.Options{
		Themes: themeReg, Tuner: tuner,
		Logger: log, Desktop: desk, Lock: lockObs, Audio: audioB, Media: media, Display: display, TV: tv, Suspend: suspendR,
		Launcher: launcher, Adapters: adapters.NewRegistry(), Config: store, Pairing: pair,
		Supervisor: sup, DevMode: f.dev && (f.devFixtures || f.devPlexFake || f.devInstalls), Feed: feed, Weather: wx, Plex: plexOpt, PlexPlaying: plexPlaying, Web: webApps,
		Installer: appInstaller, DRM: webDRM, IconFinder: &iconFinder, Autostart: autostartOpt,
		// Den badges: local counters in state.db; nothing counted while
		// config achievements.enabled is false (docs/security.md#den-badges).
		Achievements: achievements.New(achievements.Options{DB: db, Logger: log, Enabled: func() bool { return store.Current().AchievementsEnabled() }}),
		Diagnostics: func(ctx context.Context) map[string]any {
			return doctor.Report(ctx, doctor.Options{Paths: paths, Version: Version, ShellBinary: f.shellBinary})
		},
	})
	onInstallChange = coord.InstallChanged
	if plexLink != nil {
		plexLink.SetOnChange(coord.PlexChanged)
		go plexLink.Run(ctx)
	}
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
		if plexLink != nil {
			plexLink.Reconfigure()
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
// onboarding.lan_consent are both true. `dev` binds loopback only. While
// the LAN listener is up and remote.mdns is on, it is advertised over mDNS
// on the selected interfaces only (mdnsPlan, internal/remote/mdns), and the
// advertisement stops with the listener.
type remoteHost struct {
	themeAssets fs.FS // theme art for phones (/themes/)
	log         *slog.Logger
	store       *config.Store
	coord       *session.Coordinator
	pair        *pairing.Service
	dev         bool
	devListen   string

	// mdns advertises the LAN listener (nil: a real Avahi advertiser).
	mdns interface {
		Start(svc mdns.Service, ifaces []string) error
		Stop()
	}

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
	// MDNS and Name: advertise the listener (config remote.mdns) under the
	// device's display name.
	MDNS bool
	Name string
}

// mdnsPlan is the advertisement for a listener: the service and the
// interfaces to publish it on, or false. Never for the dev loopback
// listener or with remote.mdns off; only the selected LAN interfaces.
func mdnsPlan(s listenSpec) (mdns.Service, []string, bool) {
	if s.Dev != "" || !s.MDNS || len(s.Interfaces) == 0 || s.Port <= 0 || s.Port > 65535 {
		return mdns.Service{}, nil, false
	}
	typ := mdns.ServiceTypeHTTP
	if s.Transport == remote.TransportHTTPS {
		typ = mdns.ServiceTypeHTTPS
	}
	return mdns.Service{Name: s.Name, Type: typ, Port: uint16(s.Port), TXT: []string{"protocol=1"}}, append([]string(nil), s.Interfaces...), true
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
	s := listenSpec{Interfaces: append([]string(nil), r.Interfaces...), Port: r.Port, Transport: r.Transport, LayoutHTTP: r.HTTPLayoutEditing, AllowedHosts: append([]string(nil), r.AllowedHosts...),
		MDNS: r.MDNS, Name: cfg.Device.DisplayName}
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
		DevMode: h.dev, Logger: h.log, ThemeAssets: h.themeAssets, AppIcons: h.coord,
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
	if svc, ifaces, ok := mdnsPlan(want); ok {
		if err := h.advertiser().Start(svc, ifaces); err != nil {
			h.log.Warn("remote: mDNS advertisement", "err", err)
		} else {
			h.log.Info("remote: advertised over mDNS", "interfaces", ifaces, "type", svc.Type)
		}
	}
	go h.coord.SetRemoteStatus(true, append([]string(nil), h.urls...))
}

func (h *remoteHost) advertiser() interface {
	Start(svc mdns.Service, ifaces []string) error
	Stop()
} {
	if h.mdns == nil {
		h.mdns = &mdns.Advertiser{}
	}
	return h.mdns
}

func (h *remoteHost) stopLocked() {
	// The advertisement never outlives the listener.
	if h.mdns != nil {
		h.mdns.Stop()
	}
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

// paired counts a newly paired phone for Den badges.
func (h *remoteHost) paired() {
	if h.coord != nil {
		h.coord.NotePaired()
	}
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
