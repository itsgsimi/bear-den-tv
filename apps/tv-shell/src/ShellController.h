#pragma once
// ShellController (QML singleton Shell): the bridge between coordinator IPC and
// the QML shell (spec contracts/ipc.md; guide apps/tv-shell/AGENTS.md).

#include <QHash>
#include <QJsonObject>
#include <QPair>
#include <QObject>
#include <QPointer>
#include <QStringList>
#include <QVariantList>
#include <QVariantMap>
#include <QtQml/qqmlregistration.h>

class IpcClient;
class QQmlEngine;
class QJSEngine;
class QQuickWindow;

// ShellController is the single bridge between the coordinator IPC and the QML
// shell: it feeds snapshots into Session, applies coordinator input through Nav
// and answers with input_result, reports focus, and exposes the trusted local
// requests (launch, pairing, devices, remote onboarding, first-run setup and
// "Start with this PC", settings) to QML.
class ShellController : public QObject {
    Q_OBJECT
    QML_NAMED_ELEMENT(Shell)
    QML_SINGLETON
    Q_PROPERTY(QString connectionState READ connectionState NOTIFY connectionChanged)
    Q_PROPERTY(bool connected READ connected NOTIFY connectionChanged)
    Q_PROPERTY(bool offline READ offline CONSTANT)
    Q_PROPERTY(bool devBuild READ devBuild CONSTANT)
    Q_PROPERTY(QString rejectReason READ rejectReason NOTIFY connectionChanged)
    Q_PROPERTY(int attempt READ attempt NOTIFY connectionChanged)
    Q_PROPERTY(QString startScreen READ startScreen CONSTANT)
    Q_PROPERTY(QString version READ version CONSTANT)
    Q_PROPERTY(QString launchingAppId READ launchingAppId NOTIFY launchChanged)
    // Idle seconds on Bear Den before the OLED-safe screensaver (0 = off).
    // Default 300; BDTV_SCREENSAVER_SECONDS overrides (tests, kiosks).
    Q_PROPERTY(int screensaverSeconds READ screensaverSeconds WRITE setScreensaverSeconds NOTIFY screensaverSecondsChanged)
    // Seconds between bear visits; 0 = the normal relaxed, random pace.
    // BDTV_BEARS_SECONDS sets it (renders, live checks).
    Q_PROPERTY(int bearsSeconds READ bearsSeconds CONSTANT)
    // Seconds without input before Home rests (default 45; BDTV_REST_SECONDS, for measurements).
    Q_PROPERTY(int restSeconds READ restSeconds CONSTANT)
    // BDTV_BEARS_ACT forces one visit (walk, peek, hop, parade, chase) for checks.
    Q_PROPERTY(QString bearsAct READ bearsAct CONSTANT)
    // BDTV_MONTH (1–12) pretends it is that month, for checking seasonal art; 0 = the real date.
    Q_PROPERTY(int monthOverride READ monthOverride CONSTANT)
    // BDTV_LIGHTNING_SECONDS fixes the gap between thunder's lightning flashes
    // (0 = flash after flash), for checking the startled bears; -1 = the normal 14–30 s.
    Q_PROPERTY(int lightningSeconds READ lightningSeconds CONSTANT)
    // The latest weather.search answer (weather_places): places [{name, region, country,
    // latitude, longitude}], ok, error; `weatherSearching` while a search is in flight.
    Q_PROPERTY(QVariantList weatherPlaces READ weatherPlaces NOTIFY weatherPlacesChanged)
    Q_PROPERTY(bool weatherSearchOk READ weatherSearchOk NOTIFY weatherPlacesChanged)
    Q_PROPERTY(QString weatherSearchError READ weatherSearchError NOTIFY weatherPlacesChanged)
    Q_PROPERTY(bool weatherSearching READ weatherSearching NOTIFY weatherPlacesChanged)

public:
    struct Options {
        bool dev = false;
        bool offline = false;
        QString socketPath;   // empty = default
        QString fixturePath;  // offline snapshot
        QString startScreen;  // home | settings | pairing | devices | diagnostics | remote-setup
    };

    // Private: QML must obtain the shared instance through create(); a public
    // default constructor would make the engine build its own copy.
private:
    explicit ShellController(QObject *parent = nullptr);
public:
    static ShellController *create(QQmlEngine *, QJSEngine *);
    static ShellController *instance();

    void configure(const Options &options);
    void start();
    IpcClient *ipc() const { return m_ipc; }

    QString connectionState() const;
    bool connected() const;
    bool offline() const { return m_options.offline; }
    bool devBuild() const { return m_options.dev; }
    QString rejectReason() const;
    int attempt() const;
    QString startScreen() const { return m_options.startScreen; }
    QString version() const;
    QString launchingAppId() const { return m_launchingAppId; }
    int screensaverSeconds() const { return m_screensaverSeconds; }
    int bearsSeconds() const { return m_bearsSeconds; }
    int restSeconds() const { return m_restSeconds; }
    QString bearsAct() const { return m_bearsAct; }
    int monthOverride() const { return m_monthOverride; }
    int lightningSeconds() const { return m_lightningSeconds; }
    QVariantList weatherPlaces() const { return m_weatherPlaces; }
    bool weatherSearchOk() const { return m_weatherOk; }
    QString weatherSearchError() const { return m_weatherError; }
    bool weatherSearching() const { return m_weatherSearching; }
    void setScreensaverSeconds(int s)
    {
        if (s == m_screensaverSeconds)
            return;
        m_screensaverSeconds = s;
        emit screensaverSecondsChanged();
    }

    // Trusted local requests from the TV UI.
    Q_INVOKABLE void launchApp(const QString &appId);
    Q_INVOKABLE void closeApp(const QString &appId);
    /// A family phone invitation, or with pass (tonight, 24h, 7d) a guest pass.
    Q_INVOKABLE void issuePairing(const QString &pass = QString());
    Q_INVOKABLE void cancelPairing();
    Q_INVOKABLE void revokeDevice(const QString &deviceId);
    // Paired phones: a family phone's permissions (IPC devices.grant, the
    // only path to owner and layout_editor; the TV is trusted).
    Q_INVOKABLE void grantDevice(const QString &deviceId, const QStringList &permissions);
    Q_INVOKABLE void configureRemote(bool enabled, const QString &interfaceName);
    Q_INVOKABLE void updateLayout(const QVariantMap &layout);
    /// The layout to edit from: the last one sent while the coordinator has
    /// not yet published it, else the persisted one. Quick successive edits
    /// (a theme, then Art style) build on each other instead of each starting
    /// from the old snapshot, and each is sent on the revision the previous
    /// one will create (the coordinator applies the TV's updates in order).
    Q_INVOKABLE QVariantMap layoutForEdit() const;
    // Settings → Advanced playback: choose one app's setting by hand
    // (playback.set); value "" returns it to automatic.
    Q_INVOKABLE void setPlayback(const QString &adapter, const QString &setting, const QString &value);
    // Settings → Now playing on phones (remote.now_playing): whether paired
    // phones see what the app in front is playing.
    Q_INVOKABLE void setNowPlaying(bool enabled);
    // Apps → Streaming sites (app.enable): turn one web app on or off.
    Q_INVOKABLE void setAppEnabled(const QString &appId, bool enabled);
    // App installs from Flathub (contracts/ipc.md app.install*): the install
    // card asks for the size, Install starts it, Cancel stops it. Progress is
    // state.applications[].install; replies arrive as installReplied.
    Q_INVOKABLE void installInfo(const QString &appId);
    // enable: the owner pressed Install on this app's own card, so the
    // coordinator turns it on once installed (IPC app.install "enable").
    Q_INVOKABLE void installApp(const QString &appId, bool enable = false);
    Q_INVOKABLE void cancelInstall(const QString &appId);
    // Apps → Remove apps, confirmed on the RemoveCard (IPC app.uninstall):
    // removes the app's Flatpak for this user; deleteData also deletes its
    // data. The reply arrives as installReplied (type "app.uninstall").
    Q_INVOKABLE void uninstallApp(const QString &appId, bool deleteData);
    // Apps → Keep apps up to date (apps.configure).
    Q_INVOKABLE void setAutoUpdate(bool enabled);
    // Apps → Streaming sites: the Browser tile's and the streaming
    // sites' browsers (apps.browser; ids from Session.apps.browsers).
    Q_INVOKABLE void setBrowsers(const QString &browser, const QString &streamingBrowser);
    // Settings → TV control over HDMI (CEC) and its volume row
    // (cec.configure): on/off, and whether the phone's volume buttons drive
    // the PC ("pc") or the TV ("tv").
    Q_INVOKABLE void setCEC(bool enabled, const QString &volumeTarget);
    // First-run setup (contracts/ipc.md): completeOnboarding marks it done
    // (onboarding.complete → Session.onboarding.completed); setAutostart
    // writes or removes the user's autostart entry, the same one
    // `bear-den-tv autostart enable` writes (autostart.configure →
    // Session.autostart.enabled). The owner's toggle on the TV is the
    // consent. Replies arrive as setupReplied; a refusal also raises
    // requestFailed with the reason.
    Q_INVOKABLE void completeOnboarding();
    Q_INVOKABLE void setAutostart(bool enabled);
    // Settings → Sleep timer and Screen off: the power.sleep_timer (0 cancels)
    // and display.off actions; powerActivity tells the coordinator a TV key
    // was swallowed to wake the display or stay awake (contracts/ipc.md).
    Q_INVOKABLE void setSleepTimer(int minutes);
    Q_INVOKABLE void screenOff();
    Q_INVOKABLE void powerActivity();
    // Settings → Weather (contracts/ipc.md): weather.search finds places for a
    // query (answer in weatherPlaces); weather.configure stores the whole block
    // (place: a weatherPlaces entry replaces the stored place; null/empty keeps it;
    // units celsius|fahrenheit).
    Q_INVOKABLE void weatherSearch(const QString &query);
    Q_INVOKABLE void weatherConfigure(bool enabled, const QVariant &place, const QString &units, bool scene);
    // Settings → Plex (contracts/ipc.md plex.*): sign in, pick a server and
    // libraries, cancel, sign out. The flow itself is state.plex
    // (Session.plex); each reply arrives as plexReplied.
    Q_INVOKABLE void plexSignIn();
    Q_INVOKABLE void plexCancel();
    Q_INVOKABLE void plexChooseServer(const QString &serverId);
    Q_INVOKABLE void plexChooseLibraries(const QStringList &libraryIds);
    Q_INVOKABLE void plexSignOut();
    // Themes → Den badges (contracts/ipc.md achievements.*): turn counting on
    // or off, reset every badge; achievementsCelebrated says Home showed the
    // celebration for these ids (state.achievements.celebrate);
    // achievementEvent reports a shell-only event ("parade").
    Q_INVOKABLE void setAchievements(bool enabled);
    Q_INVOKABLE void resetAchievements();
    Q_INVOKABLE void achievementsCelebrated(const QStringList &ids);
    Q_INVOKABLE void achievementEvent(const QString &event);
    Q_INVOKABLE void answerConfirm(const QString &confirmId, bool accepted);
    Q_INVOKABLE void exitShell();
    // Flatpak id for a registered adapter ("" when unknown); the closed set
    // mirrors internal/applications/adapters. A web adapter's is its
    // browser's: Session.apps.browser (the Browser tile) or
    // streaming_browser (the streaming sites) looked up in
    // Session.apps.browsers; without one, the defaults (Google Chrome for
    // the streaming sites, Brave for the Browser tile).
    Q_INVOKABLE QString flatpakIdFor(const QString &adapter) const;
    // The Flatpak whose exported icon is this adapter's own icon: the app's
    // Flatpak, or "" for the streaming sites (they run in a browser and never
    // show its icon; the Browser tile is its browser and does). Mirrors
    // adapters.OwnFlatpakIcon in Go.
    Q_INVOKABLE QString ownIconFlatpakIdFor(const QString &adapter) const;
    // An app's artwork: {icon, logo, background, iconSource} (URLs, "" when
    // absent). Bear Den bundles its own original icons, never third-party
    // logos (docs/THEMES.md → App icons). The icon comes, in order, from:
    //   "brand"   the owner's brand folder $XDG_DATA_HOME/bear-den-tv/brand/<adapter>/
    //             icon.{svg,png,jpg,webp} (logo and background come only from there);
    //   "flatpak" only when `icons` is "app" (layout ui.app_icons, the default):
    //             the icon the installed Flatpak exports (ownIconFlatpakIdFor; an
    //             export exists only while the Flatpak is installed);
    //   "bundled" Bear Den's own icon for the adapter, qrc assets/pixel/app-<adapter>.png
    //             or, with `classic`, assets/classic/app-<adapter>.svg (tools/pixelart
    //             and tools/classicart appicons.py);
    //   ""        none: AppIcon draws a monogram.
    // Cached for a minute per adapter, style and choice.
    Q_INVOKABLE QVariantMap appArt(const QString &adapter, bool classic = false, const QString &icons = QStringLiteral("app")) const;
    // Forget cached artwork (tests; a new brand folder shows within a minute anyway).
    void forgetArt() { m_artCache.clear(); }
    // The home-network interfaces the remote could bind to, as
    // [{name, label ("home network (wired)"), addresses}]: see lanCandidate.
    Q_INVOKABLE QVariantList lanInterfaces() const;
    /// One interface as lanInterfaces offers it, or empty when it is never
    /// offered: down, loopback, a container/bridge/VPN by name
    /// (config.VirtualInterfacePrefixes), point-to-point, or only
    /// 100.64.0.0/10 addresses (overlay VPNs such as Tailscale).
    static QVariantMap lanCandidate(const QString &name, bool up, bool loopback, bool pointToPoint, bool wireless, const QStringList &ipv4);

signals:
    void connectionChanged();
    void launchChanged();
    /// The coordinator saw appId come to the front (the launch's `observed`
    /// result), emitted just before launchingAppId clears. LaunchOverlay keeps
    /// covering Bear Den until the next snapshot says the shell is in front.
    void launchObserved(const QString &appId);
    void screensaverSecondsChanged();
    void notify(const QString &kind, const QString &text);
    void confirmRequested(const QString &confirmId, const QString &kind, const QString &summary, int expiresInS);
    void homeRequested();
    void requestFailed(const QString &what, const QString &message);
    void weatherPlacesChanged();
    /// The coordinator's `result` for weather.configure.
    void weatherConfigured(bool ok, const QString &error);
    /// The coordinator's `result` for a plex.* message (type is its name).
    void plexReplied(const QString &type, bool ok, const QString &error);
    /// The coordinator's `result` for app.install, app.install_info or
    /// app.install_cancel (type), for appId; data carries the sizes.
    void installReplied(const QString &type, const QString &appId, bool ok, const QString &error, const QVariantMap &data);
    /// The coordinator's `result` for onboarding.complete or autostart.configure (type).
    void setupReplied(const QString &type, bool ok, const QString &error);

private:
    void onInput(const QString &requestId, const QString &action, const QVariantMap &args, int contextEpoch);
    void onHome(const QString &requestId, bool restoreFocus);
    void onActionResult(const QString &requestId, const QString &action, const QString &outcome, const QString &code,
                        const QString &message, const QVariantMap &detail);
    void onReply(const QString &requestId, const QString &type, const QJsonObject &payload);
    void setLaunching(const QString &appId);
    QVariantMap lookupArt(const QString &adapter, bool classic, bool appsOwn) const;

    Options m_options;
    IpcClient *m_ipc = nullptr;
    QString m_launchingAppId;
    QString m_launchRequestId;
    int m_screensaverSeconds = 300;
    int m_bearsSeconds = 0;
    int m_restSeconds = 45;
    QString m_bearsAct;
    int m_monthOverride = 0;
    int m_lightningSeconds = -1;
    QString m_weatherSearchId;
    QVariantList m_weatherPlaces;
    bool m_weatherOk = true;
    bool m_weatherSearching = false;
    QString m_weatherError;
    QHash<QString, QString> m_installRequests; // request id → app id
    QVariantMap m_pendingLayout;  // last layout sent, until a snapshot carries it
    int m_pendingRevision = 0;    // the revision that layout will have; 0 = none pending
    bool m_watchingSnapshots = false;
    void layoutUpdateFailed(const QString &error);
    mutable QHash<QString, QPair<qint64, QVariantMap>> m_artCache;
};
