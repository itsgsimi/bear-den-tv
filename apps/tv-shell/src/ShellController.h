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
// requests (launch, pairing, devices, remote onboarding, settings) to QML.
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
    Q_INVOKABLE void issuePairing();
    Q_INVOKABLE void cancelPairing();
    Q_INVOKABLE void revokeDevice(const QString &deviceId);
    Q_INVOKABLE void configureRemote(bool enabled, const QString &interfaceName);
    Q_INVOKABLE void updateLayout(const QVariantMap &layout);
    // Settings → Advanced playback: choose one app's setting by hand
    // (playback.set); value "" returns it to automatic.
    Q_INVOKABLE void setPlayback(const QString &adapter, const QString &setting, const QString &value);
    // Settings → Weather (contracts/ipc.md): weather.search finds places for a
    // query (answer in weatherPlaces); weather.configure stores the whole block
    // (place: a weatherPlaces entry replaces the stored place; null/empty keeps it;
    // units celsius|fahrenheit).
    Q_INVOKABLE void weatherSearch(const QString &query);
    Q_INVOKABLE void weatherConfigure(bool enabled, const QVariant &place, const QString &units, bool scene);
    Q_INVOKABLE void answerConfirm(const QString &confirmId, bool accepted);
    Q_INVOKABLE void exitShell();
    // Flatpak id for a registered adapter ("" when unknown); the closed set
    // mirrors internal/applications/adapters.
    Q_INVOKABLE QString flatpakIdFor(const QString &adapter) const;
    // Official artwork for an app as file:// URLs: {icon, logo, background}
    // (each "" when absent). Bear Den bundles no brand artwork; sources, in
    // order: the owner's brand folder ($XDG_DATA_HOME/bear-den-tv/brand/<adapter>/
    // logo|icon|background.{svg,png,jpg}), the icon the installed Flatpak
    // exports, then icons cached by `bear-den-tv artwork fetch`.
    Q_INVOKABLE QVariantMap appArt(const QString &adapter) const;
    // Non-loopback, up, non-virtual interfaces the remote could bind to.
    Q_INVOKABLE QVariantList lanInterfaces() const;

signals:
    void connectionChanged();
    void launchChanged();
    void screensaverSecondsChanged();
    void notify(const QString &kind, const QString &text);
    void confirmRequested(const QString &confirmId, const QString &kind, const QString &summary, int expiresInS);
    void homeRequested();
    void requestFailed(const QString &what, const QString &message);
    void weatherPlacesChanged();
    /// The coordinator's `result` for weather.configure.
    void weatherConfigured(bool ok, const QString &error);

private:
    void onInput(const QString &requestId, const QString &action, const QVariantMap &args, int contextEpoch);
    void onHome(const QString &requestId, bool restoreFocus);
    void onActionResult(const QString &requestId, const QString &action, const QString &outcome, const QString &code,
                        const QString &message, const QVariantMap &detail);
    void onReply(const QString &requestId, const QString &type, const QJsonObject &payload);
    void setLaunching(const QString &appId);
    QVariantMap lookupArt(const QString &adapter) const;

    Options m_options;
    IpcClient *m_ipc = nullptr;
    QString m_launchingAppId;
    QString m_launchRequestId;
    int m_screensaverSeconds = 300;
    int m_bearsSeconds = 0;
    int m_restSeconds = 45;
    QString m_bearsAct;
    int m_monthOverride = 0;
    QString m_weatherSearchId;
    QVariantList m_weatherPlaces;
    bool m_weatherOk = true;
    bool m_weatherSearching = false;
    QString m_weatherError;
    mutable QHash<QString, QPair<qint64, QVariantMap>> m_artCache;
};
