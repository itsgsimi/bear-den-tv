// ShellController implementation: the bridge between coordinator IPC and QML
// (contract in ShellController.h; spec contracts/ipc.md).

#include "ShellController.h"

#include "IpcClient.h"
#include "Navigator.h"
#include "SessionModel.h"

#include <QCoreApplication>
#include <QDateTime>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QStandardPaths>
#include <QUrl>
#include <QJSEngine>
#include <QJsonArray>
#include <QJsonObject>
#include <QNetworkInterface>

#include <algorithm>

namespace {
ShellController *g_instance = nullptr;

bool refusedInterface(const QString &name)
{
    static const QStringList prefixes{QStringLiteral("docker"), QStringLiteral("br-"), QStringLiteral("veth"),
                                      QStringLiteral("tun"), QStringLiteral("wg"), QStringLiteral("virbr")};
    for (const QString &p : prefixes)
        if (name.startsWith(p))
            return true;
    return false;
}
} // namespace

ShellController::ShellController(QObject *parent) : QObject(parent)
{
    if (!g_instance)
        g_instance = this;
    m_ipc = new IpcClient(this);
    bool ok = false;
    const int envSeconds = qEnvironmentVariableIntValue("BDTV_SCREENSAVER_SECONDS", &ok);
    if (ok && envSeconds >= 0)
        m_screensaverSeconds = envSeconds;
    const int bears = qEnvironmentVariableIntValue("BDTV_BEARS_SECONDS", &ok);
    if (ok && bears > 0)
        m_bearsSeconds = bears;
    m_bearsAct = qEnvironmentVariable("BDTV_BEARS_ACT");
    const int month = qEnvironmentVariableIntValue("BDTV_MONTH", &ok);
    if (ok && month >= 1 && month <= 12)
        m_monthOverride = month;
    const int lightning = qEnvironmentVariableIntValue("BDTV_LIGHTNING_SECONDS", &ok);
    if (ok && lightning >= 0)
        m_lightningSeconds = lightning;
    const int rest = qEnvironmentVariableIntValue("BDTV_REST_SECONDS", &ok);
    if (ok && rest > 0)
        m_restSeconds = rest;
    connect(m_ipc, &IpcClient::connectionStateChanged, this, &ShellController::connectionChanged);
    connect(m_ipc, &IpcClient::stateReceived, this, [](const QJsonObject &state) {
        if (SessionModel *s = SessionModel::instance())
            s->applySnapshot(state);
    });
    connect(m_ipc, &IpcClient::inputReceived, this, &ShellController::onInput);
    connect(m_ipc, &IpcClient::homeReceived, this, &ShellController::onHome);
    connect(m_ipc, &IpcClient::layoutPreviewReceived, this, [](const QJsonObject &layout, int) {
        if (SessionModel *s = SessionModel::instance())
            s->applyLayoutPreview(layout);
    });
    connect(m_ipc, &IpcClient::layoutPreviewEnded, this, [] {
        if (SessionModel *s = SessionModel::instance())
            s->endLayoutPreview();
    });
    connect(m_ipc, &IpcClient::confirmRequested, this, &ShellController::confirmRequested);
    connect(m_ipc, &IpcClient::notifyReceived, this, [this](const QString &, const QString &kind, const QString &text) {
        emit notify(kind, text);
    });
    connect(m_ipc, &IpcClient::shutdownReceived, this, [](const QString &) { QCoreApplication::exit(0); });
    connect(m_ipc, &IpcClient::actionResultReceived, this, &ShellController::onActionResult);
    connect(m_ipc, &IpcClient::replyReceived, this, &ShellController::onReply);
    connect(m_ipc, &IpcClient::settingsResultReceived, this, [this](const QString &, bool ok, int, const QString &error) {
        if (!ok)
            emit requestFailed(tr("Settings"), error);
    });
    connect(m_ipc, &IpcClient::requestTimedOut, this, [this](const QString &requestId, const QString &) {
        if (requestId == m_launchRequestId)
            setLaunching(QString());
        if (requestId == m_weatherSearchId) {
            m_weatherSearchId.clear();
            m_weatherSearching = false;
            m_weatherOk = false;
            m_weatherError = tr("The place search did not answer in time.");
            emit weatherPlacesChanged();
            return;
        }
        emit requestFailed(tr("Request"), tr("Bear Den did not answer in time."));
    });
}

ShellController *ShellController::create(QQmlEngine *, QJSEngine *)
{
    if (!g_instance)
        g_instance = new ShellController();
    QJSEngine::setObjectOwnership(g_instance, QJSEngine::CppOwnership);
    return g_instance;
}

ShellController *ShellController::instance()
{
    return g_instance;
}

void ShellController::configure(const Options &options)
{
    m_options = options;
    if (m_options.startScreen.isEmpty())
        m_options.startScreen = QStringLiteral("home");
    if (!m_options.socketPath.isEmpty())
        m_ipc->setSocketPath(m_options.socketPath);
    m_ipc->setOffline(m_options.offline);
    if (Navigator *nav = Navigator::instance()) {
        connect(nav, &Navigator::focusReported, this,
                [this](const QString &screen, const QString &sectionId, const QString &itemId, qreal scrollX, bool textField) {
                    if (m_ipc->isConnected())
                        m_ipc->sendFocus(screen, sectionId, itemId, scrollX, textField);
                });
    }
}

void ShellController::start()
{
    if (m_options.offline) {
        if (!m_options.fixturePath.isEmpty() && SessionModel::instance())
            SessionModel::instance()->loadFixture(m_options.fixturePath);
        return;
    }
    m_ipc->start();
}

QString ShellController::connectionState() const { return m_ipc->connectionState(); }
bool ShellController::connected() const { return m_ipc->isConnected(); }
QString ShellController::rejectReason() const { return m_ipc->rejectReason(); }
int ShellController::attempt() const { return m_ipc->attempt(); }
QString ShellController::version() const { return QCoreApplication::applicationVersion(); }

void ShellController::onInput(const QString &requestId, const QString &action, const QVariantMap &args, int contextEpoch)
{
    SessionModel *session = SessionModel::instance();
    Navigator *nav = Navigator::instance();
    // The shell applies input only against the epoch it knows (contracts/ipc.md).
    if (!session || !nav || contextEpoch != session->contextEpoch()) {
        m_ipc->sendInputResult(requestId, QStringLiteral("failed"), QStringLiteral("stale_epoch"),
                               {{QStringLiteral("shell_epoch"), session ? session->contextEpoch() : -1}});
        return;
    }
    const QVariantMap res = nav->apply(action, args);
    m_ipc->sendInputResult(requestId, res.value(QStringLiteral("outcome")).toString(),
                           res.value(QStringLiteral("code")).toString(), res.value(QStringLiteral("detail")).toMap());
}

void ShellController::onHome(const QString &requestId, bool)
{
    Navigator *nav = Navigator::instance();
    if (!nav) {
        m_ipc->sendInputResult(requestId, QStringLiteral("failed"), QStringLiteral("internal"), {});
        return;
    }
    // Nav.apply("home") emits homeRequested (QML closes dialogs, returns home,
    // restores focus) and asks the window manager to activate the window.
    const QVariantMap res = nav->apply(QStringLiteral("home"));
    m_ipc->sendInputResult(requestId, res.value(QStringLiteral("outcome")).toString(),
                           res.value(QStringLiteral("code")).toString(), res.value(QStringLiteral("detail")).toMap());
}

void ShellController::setLaunching(const QString &appId)
{
    if (m_launchingAppId == appId)
        return;
    m_launchingAppId = appId;
    if (appId.isEmpty())
        m_launchRequestId.clear();
    emit launchChanged();
}

void ShellController::launchApp(const QString &appId)
{
    if (!m_ipc->isConnected()) {
        emit requestFailed(tr("Open app"), tr("Bear Den is not connected, so apps cannot be opened right now."));
        return;
    }
    m_launchRequestId = m_ipc->sendRequest(QStringLiteral("app.launch"), QJsonObject{{QStringLiteral("app_id"), appId}});
    setLaunching(appId);
}

void ShellController::closeApp(const QString &appId)
{
    m_ipc->sendRequest(QStringLiteral("app.close"), QJsonObject{{QStringLiteral("app_id"), appId}, {QStringLiteral("force"), false}});
}

void ShellController::onActionResult(const QString &requestId, const QString &action, const QString &outcome,
                                     const QString &, const QString &message, const QVariantMap &)
{
    Q_UNUSED(action)
    if (m_launchRequestId.isEmpty() || requestId != m_launchRequestId)
        return;
    if (outcome == QLatin1String("observed")) {
        setLaunching(QString());
    } else if (outcome == QLatin1String("failed")) {
        setLaunching(QString());
        emit requestFailed(tr("Open app"), message.isEmpty() ? tr("The app could not be opened.") : message);
    }
}

void ShellController::onReply(const QString &requestId, const QString &type, const QJsonObject &payload)
{
    if (type == QLatin1String("weather.search") || type == QLatin1String("weather_places")) {
        if (requestId != m_weatherSearchId)
            return; // an older search answered late
        m_weatherSearchId.clear();
        m_weatherSearching = false;
        m_weatherPlaces = payload.value(QStringLiteral("places")).toArray().toVariantList();
        m_weatherOk = payload.value(QStringLiteral("ok")).toBool();
        m_weatherError = payload.value(QStringLiteral("error")).toString();
        emit weatherPlacesChanged();
        return;
    }
    if (type == QLatin1String("weather.configure")) {
        const bool ok = payload.value(QStringLiteral("ok")).toBool();
        const QString error = payload.value(QStringLiteral("error")).toString();
        emit weatherConfigured(ok, error);
        if (!ok)
            emit requestFailed(tr("Weather"), error);
        return;
    }
    if (payload.value(QStringLiteral("ok")).toBool(true))
        return;
    const QString error = payload.value(QStringLiteral("error")).toString();
    if (type == QLatin1String("pair.issue"))
        emit requestFailed(tr("Pairing"), error);
    else if (type == QLatin1String("remote.configure"))
        emit requestFailed(tr("Phone remote"), error);
    else if (type == QLatin1String("devices.revoke"))
        emit requestFailed(tr("Paired phones"), error);
    else if (type == QLatin1String("remote.now_playing"))
        emit requestFailed(tr("Now playing on phones"), error);
    else if (type == QLatin1String("playback.set"))
        emit requestFailed(tr("Advanced playback"), error);
    else
        emit requestFailed(tr("Request"), error);
}

void ShellController::issuePairing() { m_ipc->sendPairIssue(); }
void ShellController::cancelPairing() { m_ipc->sendPairCancel(); }
void ShellController::revokeDevice(const QString &deviceId) { m_ipc->sendDevicesRevoke(deviceId); }

void ShellController::configureRemote(bool enabled, const QString &interfaceName)
{
    m_ipc->sendRemoteConfigure(enabled, QStringLiteral("trusted-lan-http"), interfaceName, 0, false);
}

void ShellController::updateLayout(const QVariantMap &layout)
{
    SessionModel *session = SessionModel::instance();
    if (!session)
        return;
    m_ipc->sendSettingsUpdate(session->configRevision(), QJsonObject::fromVariantMap(layout));
}

void ShellController::weatherSearch(const QString &query)
{
    const QString q = query.trimmed();
    const bool clean = !q.isEmpty() && q.size() <= 80
        && std::none_of(q.cbegin(), q.cend(), [](QChar c) { return c.category() == QChar::Other_Control; });
    if (!clean) {
        m_weatherSearchId.clear();
        m_weatherSearching = false;
        m_weatherPlaces.clear();
        m_weatherOk = false;
        m_weatherError = tr("Type a town or city (up to 80 characters).");
        emit weatherPlacesChanged();
        return;
    }
    m_weatherSearchId = m_ipc->sendWeatherSearch(q);
    m_weatherSearching = true;
    m_weatherError.clear();
    emit weatherPlacesChanged();
}

void ShellController::weatherConfigure(bool enabled, const QVariant &place, const QString &units, bool scene)
{
    QJsonValue placeJson; // null
    const QVariantMap p = place.toMap();
    if (!p.isEmpty()) {
        placeJson = QJsonObject{
            {QStringLiteral("name"), p.value(QStringLiteral("name")).toString()},
            {QStringLiteral("region"), p.value(QStringLiteral("region")).toString()},
            {QStringLiteral("country"), p.value(QStringLiteral("country")).toString()},
            {QStringLiteral("latitude"), p.value(QStringLiteral("latitude")).toDouble()},
            {QStringLiteral("longitude"), p.value(QStringLiteral("longitude")).toDouble()},
        };
    }
    m_ipc->sendWeatherConfigure(enabled, placeJson, units, scene);
}

void ShellController::setPlayback(const QString &adapter, const QString &setting, const QString &value)
{
    m_ipc->sendPlaybackSet(adapter, setting, value);
}

void ShellController::setNowPlaying(bool enabled)
{
    m_ipc->sendRemoteNowPlaying(enabled);
}

void ShellController::setSleepTimer(int minutes)
{
    m_ipc->sendRequest(QStringLiteral("power.sleep_timer"), QJsonObject{{QStringLiteral("minutes"), minutes}});
}

void ShellController::screenOff()
{
    m_ipc->sendRequest(QStringLiteral("display.off"), QJsonObject{});
}

void ShellController::powerActivity()
{
    m_ipc->sendPowerActivity();
}

void ShellController::answerConfirm(const QString &confirmId, bool accepted)
{
    m_ipc->sendConfirmResult(confirmId, accepted);
}

void ShellController::exitShell()
{
    m_ipc->sendShellExit(QStringLiteral("maintenance"));
    QCoreApplication::exit(0);
}

QVariantList ShellController::lanInterfaces() const
{
    QVariantList out;
    const auto ifaces = QNetworkInterface::allInterfaces();
    for (const QNetworkInterface &i : ifaces) {
        const auto flags = i.flags();
        if (!(flags & QNetworkInterface::IsUp) || (flags & QNetworkInterface::IsLoopBack) || refusedInterface(i.name()))
            continue;
        QStringList addrs;
        for (const QNetworkAddressEntry &e : i.addressEntries())
            if (e.ip().protocol() == QAbstractSocket::IPv4Protocol)
                addrs << e.ip().toString();
        if (addrs.isEmpty())
            continue;
        const bool wireless = i.type() == QNetworkInterface::Wifi || i.name().startsWith(QLatin1String("wl"));
        out << QVariantMap{{QStringLiteral("name"), i.name()},
                           {QStringLiteral("label"), wireless ? tr("Wi-Fi") : tr("Wired")},
                           {QStringLiteral("addresses"), addrs.join(QStringLiteral(", "))}};
    }
    return out;
}

QString ShellController::flatpakIdFor(const QString &adapter) const
{
    if (adapter == QLatin1String("plex-htpc"))
        return QStringLiteral("tv.plex.PlexHTPC");
    if (adapter == QLatin1String("vacuumtube"))
        return QStringLiteral("rocks.shy.VacuumTube");
    if (adapter == QLatin1String("moonlight"))
        return QStringLiteral("com.moonlight_stream.Moonlight");
    return {};
}

namespace {
QString firstExisting(const QString &base, const QStringList &exts)
{
    for (const QString &ext : exts)
        if (QFileInfo::exists(base + ext))
            return QUrl::fromLocalFile(base + ext).toString();
    return {};
}

QString flatpakExportedIcon(const QString &id)
{
    QStringList roots{QDir::home().filePath(QStringLiteral(".local/share/flatpak/exports/share")),
                      QStringLiteral("/var/lib/flatpak/exports/share")};
    static const QStringList sizes{QStringLiteral("512x512"), QStringLiteral("256x256"), QStringLiteral("scalable"), QStringLiteral("128x128")};
    for (const QString &root : roots)
        for (const QString &size : sizes) {
            const QString hit = firstExisting(QStringLiteral("%1/icons/hicolor/%2/apps/%3").arg(root, size, id), {QStringLiteral(".png"), QStringLiteral(".svg")});
            if (!hit.isEmpty())
                return hit;
        }
    return {};
}
} // namespace

QVariantMap ShellController::appArt(const QString &adapter) const
{
    // Bindings call this on every state update; the answer only changes when
    // apps are installed or brand files added, so cache it for a minute.
    const qint64 now = QDateTime::currentMSecsSinceEpoch();
    auto hit = m_artCache.constFind(adapter);
    if (hit != m_artCache.constEnd() && now - hit->first < 60000)
        return hit->second;
    const QVariantMap art = lookupArt(adapter);
    m_artCache.insert(adapter, {now, art});
    return art;
}

QVariantMap ShellController::lookupArt(const QString &adapter) const
{
    static const QStringList exts{QStringLiteral(".svg"), QStringLiteral(".png"), QStringLiteral(".jpg"), QStringLiteral(".webp")};
    QVariantMap art{{QStringLiteral("icon"), QString()}, {QStringLiteral("logo"), QString()}, {QStringLiteral("background"), QString()}};
    if (adapter.isEmpty())
        return art;
    const QString brand = QStandardPaths::writableLocation(QStandardPaths::GenericDataLocation)
                          + QStringLiteral("/bear-den-tv/brand/") + adapter + QLatin1Char('/');
    for (const QString &key : {QStringLiteral("icon"), QStringLiteral("logo"), QStringLiteral("background")})
        art.insert(key, firstExisting(brand + key, exts));
    const QString id = flatpakIdFor(adapter);
    if (art.value(QStringLiteral("icon")).toString().isEmpty() && !id.isEmpty()) {
        QString icon = flatpakExportedIcon(id);
        if (icon.isEmpty())
            icon = firstExisting(QStandardPaths::writableLocation(QStandardPaths::GenericCacheLocation)
                                     + QStringLiteral("/bear-den-tv/brand/") + id, {QStringLiteral(".png"), QStringLiteral(".svg")});
        art.insert(QStringLiteral("icon"), icon);
    }
    return art;
}
