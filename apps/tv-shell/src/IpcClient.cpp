// IpcClient implementation: the shell end of the coordinator socket (contract
// in IpcClient.h; spec contracts/ipc.md).

#include "IpcClient.h"

#include <QCoreApplication>
#include <QDir>
#include <QJsonArray>
#include <QJsonDocument>
#include <QLocalSocket>
#include <QStandardPaths>
#include <QTimer>
#include <QUuid>

#ifdef Q_OS_LINUX
#include <sys/socket.h>
#include <unistd.h>
#endif

IpcClient::IpcClient(QObject *parent) : QObject(parent)
{
    const QString runtimeDir = QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
    m_socketPath = QDir(runtimeDir.isEmpty() ? QDir::tempPath() : runtimeDir).filePath(QStringLiteral("bear-den-tv/shell.sock"));

    m_reconnectTimer = new QTimer(this);
    m_reconnectTimer->setSingleShot(true);
    connect(m_reconnectTimer, &QTimer::timeout, this, &IpcClient::openSocket);

    m_pingTimer = new QTimer(this);
    connect(m_pingTimer, &QTimer::timeout, this, &IpcClient::sendPing);

    m_silenceTimer = new QTimer(this);
    m_silenceTimer->setSingleShot(true);
    connect(m_silenceTimer, &QTimer::timeout, this, [this] { abortConnection(QStringLiteral("keepalive_timeout"), true); });
}

void IpcClient::setSocketPath(const QString &path)
{
    m_socketPath = path;
    emit connectionStateChanged();
}

void IpcClient::setOffline(bool offline)
{
    m_offline = offline;
    setState(offline ? QStringLiteral("offline") : QStringLiteral("disconnected"));
}

void IpcClient::setTimings(int pingMs, int silenceMs, int backoffMinMs, int backoffMaxMs, int requestTimeoutMs)
{
    m_pingMs = pingMs;
    m_silenceMs = silenceMs;
    m_backoffMinMs = backoffMinMs;
    m_backoffMaxMs = backoffMaxMs;
    m_backoffMs = backoffMinMs;
    m_requestTimeoutMs = requestTimeoutMs;
}

void IpcClient::start()
{
    if (m_offline)
        return;
    m_stopped = false;
    m_backoffMs = m_backoffMinMs;
    openSocket();
}

void IpcClient::stop()
{
    m_stopped = true;
    m_reconnectTimer->stop();
    m_pingTimer->stop();
    m_silenceTimer->stop();
    if (m_socket) {
        m_socket->disconnect(this);
        m_socket->abort();
        m_socket->deleteLater();
        m_socket = nullptr;
    }
    m_buffer.clear();
    setState(QStringLiteral("disconnected"));
}

void IpcClient::setState(const QString &state)
{
    if (m_state == state)
        return;
    m_state = state;
    emit connectionStateChanged();
}

void IpcClient::openSocket()
{
    if (m_stopped || m_offline)
        return;
    if (m_socket) {
        m_socket->disconnect(this);
        m_socket->abort();
        m_socket->deleteLater();
    }
    ++m_attempt;
    m_buffer.clear();
    m_socket = new QLocalSocket(this);
    connect(m_socket, &QLocalSocket::connected, this, &IpcClient::onConnected);
    connect(m_socket, &QLocalSocket::readyRead, this, &IpcClient::onReadyRead);
    connect(m_socket, &QLocalSocket::errorOccurred, this, &IpcClient::onSocketError);
    connect(m_socket, &QLocalSocket::disconnected, this, [this] {
        if (m_state != QLatin1String("rejected"))
            abortConnection(QStringLiteral("socket_closed"), true);
    });
    setState(QStringLiteral("connecting"));
    m_socket->connectToServer(m_socketPath);
}

bool IpcClient::peerIsSameUser() const
{
#ifdef Q_OS_LINUX
    if (!m_socket)
        return false;
    struct ucred cred {};
    socklen_t len = sizeof(cred);
    const int fd = int(m_socket->socketDescriptor());
    if (fd < 0 || getsockopt(fd, SOL_SOCKET, SO_PEERCRED, &cred, &len) != 0)
        return false;
    return cred.uid == getuid();
#else
    return true;
#endif
}

void IpcClient::onConnected()
{
    if (!peerIsSameUser()) {
        abortConnection(QStringLiteral("peer_uid_mismatch"), true);
        return;
    }
    setState(QStringLiteral("handshaking"));
    m_silenceTimer->start(m_silenceMs);
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("hello")},
        {QStringLiteral("protocol"), kProtocol},
        {QStringLiteral("client"), QStringLiteral("shell")},
        {QStringLiteral("version"), QCoreApplication::applicationVersion()},
        {QStringLiteral("pid"), qint64(QCoreApplication::applicationPid())},
    });
}

void IpcClient::onSocketError()
{
    if (m_state == QLatin1String("connecting"))
        abortConnection(QStringLiteral("connect_failed"), true);
}

void IpcClient::abortConnection(const QString &reason, bool reconnect)
{
    m_pingTimer->stop();
    m_silenceTimer->stop();
    if (m_socket) {
        m_socket->disconnect(this);
        m_socket->abort();
        m_socket->deleteLater();
        m_socket = nullptr;
    }
    m_buffer.clear();
    m_pending.clear();
    const bool wasUp = m_state == QLatin1String("connected") || m_state == QLatin1String("handshaking");
    setState(reconnect ? QStringLiteral("disconnected") : QStringLiteral("rejected"));
    if (wasUp)
        emit disconnectedFromCoordinator(reason);
    if (reconnect)
        scheduleReconnect();
}

void IpcClient::scheduleReconnect()
{
    if (m_stopped || m_offline)
        return;
    m_reconnectTimer->start(m_backoffMs);
    m_backoffMs = qMin(m_backoffMs * 2, m_backoffMaxMs);
}

void IpcClient::onReadyRead()
{
    if (!m_socket)
        return;
    m_buffer.append(m_socket->readAll());
    int newline = -1;
    while (m_socket && (newline = int(m_buffer.indexOf('\n'))) >= 0) {
        const QByteArray line = m_buffer.left(newline);
        m_buffer.remove(0, newline + 1);
        if (line.size() > kMaxFrameBytes) {
            abortConnection(QStringLiteral("oversized_frame"), true);
            return;
        }
        handleFrame(line);
    }
    if (m_buffer.size() > kMaxFrameBytes)
        abortConnection(QStringLiteral("oversized_frame"), true);
}

void IpcClient::handleFrame(const QByteArray &line)
{
    if (line.trimmed().isEmpty())
        return;
    const QJsonDocument doc = QJsonDocument::fromJson(line);
    if (!doc.isObject())
        return;
    const QJsonObject msg = doc.object();
    const QString type = msg.value(QStringLiteral("type")).toString();
    if (m_silenceTimer->isActive() || m_state == QLatin1String("connected"))
        m_silenceTimer->start(m_silenceMs);

    if (m_state == QLatin1String("handshaking") || m_state == QLatin1String("connecting")) {
        if (type == QLatin1String("welcome")) {
            if (msg.value(QStringLiteral("protocol")).toInt() != kProtocol) {
                m_rejectReason = QStringLiteral("unsupported_protocol");
                emit rejected(m_rejectReason);
                abortConnection(m_rejectReason, false);
                return;
            }
            m_sessionId = msg.value(QStringLiteral("session_id")).toString();
            m_coordinatorVersion = msg.value(QStringLiteral("coordinator_version")).toString();
            m_rejectReason.clear();
            m_backoffMs = m_backoffMinMs;
            setState(QStringLiteral("connected"));
            m_pingTimer->start(m_pingMs);
            emit welcomed(m_sessionId, m_coordinatorVersion);
            return;
        }
        if (type == QLatin1String("reject")) {
            m_rejectReason = msg.value(QStringLiteral("reason")).toString();
            emit rejected(m_rejectReason);
            // An unsupported protocol is final; any other rejection (another shell is
            // connected) is retried at the slowest backoff.
            const bool retry = m_rejectReason != QLatin1String("unsupported_protocol");
            m_backoffMs = m_backoffMaxMs;
            abortConnection(m_rejectReason, retry);
            if (!retry)
                setState(QStringLiteral("rejected"));
            return;
        }
        return; // nothing else is valid before welcome
    }
    if (m_state != QLatin1String("connected") && !m_offline)
        return;

    if (type == QLatin1String("ping")) {
        sendPong();
    } else if (type == QLatin1String("pong")) {
        // silence timer already refreshed
    } else if (type == QLatin1String("state")) {
        emit stateReceived(msg.value(QStringLiteral("state")).toObject());
    } else if (type == QLatin1String("input")) {
        emit inputReceived(msg.value(QStringLiteral("request_id")).toString(), msg.value(QStringLiteral("action")).toString(),
                           msg.value(QStringLiteral("args")).toObject().toVariantMap(), msg.value(QStringLiteral("context_epoch")).toInt(-1));
    } else if (type == QLatin1String("home")) {
        emit homeReceived(msg.value(QStringLiteral("request_id")).toString(), msg.value(QStringLiteral("restore_focus")).toBool(true));
    } else if (type == QLatin1String("layout_preview")) {
        emit layoutPreviewReceived(msg.value(QStringLiteral("layout")).toObject(), msg.value(QStringLiteral("expires_in_s")).toInt(60));
    } else if (type == QLatin1String("layout_preview_end")) {
        emit layoutPreviewEnded();
    } else if (type == QLatin1String("confirm_request")) {
        emit confirmRequested(msg.value(QStringLiteral("confirm_id")).toString(), msg.value(QStringLiteral("kind")).toString(),
                              msg.value(QStringLiteral("summary")).toString(), msg.value(QStringLiteral("expires_in_s")).toInt(30));
    } else if (type == QLatin1String("notify")) {
        emit notifyReceived(msg.value(QStringLiteral("id")).toString(), msg.value(QStringLiteral("kind")).toString(), msg.value(QStringLiteral("text")).toString());
    } else if (type == QLatin1String("shutdown")) {
        emit shutdownReceived(msg.value(QStringLiteral("reason")).toString());
    } else if (type == QLatin1String("action_result")) {
        // The coordinator wraps the action contract result: {"type":"action_result","result":{...}}.
        const QJsonObject res = msg.contains(QStringLiteral("result")) ? msg.value(QStringLiteral("result")).toObject() : msg;
        const QString requestId = res.value(QStringLiteral("request_id")).toString();
        const Pending pending = m_pending.value(requestId);
        const QString outcome = res.value(QStringLiteral("outcome")).toString();
        if (outcome != QLatin1String("accepted"))
            m_pending.remove(requestId);
        emit actionResultReceived(requestId, pending.action, outcome, res.value(QStringLiteral("code")).toString(),
                                  res.value(QStringLiteral("message")).toString(), res.value(QStringLiteral("detail")).toObject().toVariantMap());
        emit replyReceived(requestId, type, res);
    } else if (type == QLatin1String("settings_result")) {
        const QString requestId = msg.value(QStringLiteral("request_id")).toString();
        m_pending.remove(requestId);
        emit settingsResultReceived(requestId, msg.value(QStringLiteral("ok")).toBool(), msg.value(QStringLiteral("revision")).toInt(),
                                    msg.value(QStringLiteral("error")).toString());
        emit replyReceived(requestId, type, msg);
    } else if (msg.contains(QStringLiteral("request_id"))) {
        // Generic administrative reply ({"type":"result",...}); report the type
        // of the request it answers so callers can tell pair.issue from devices.revoke.
        const QString requestId = msg.value(QStringLiteral("request_id")).toString();
        const Pending pending = m_pending.take(requestId);
        emit replyReceived(requestId, pending.type.isEmpty() ? type : pending.type, msg);
    }
}

bool IpcClient::send(const QJsonObject &message)
{
    if (m_offline) {
        m_sent.append(message);
        emit messageSent(message);
        return true;
    }
    if (!m_socket || m_socket->state() != QLocalSocket::ConnectedState)
        return false;
    QByteArray frame = QJsonDocument(message).toJson(QJsonDocument::Compact);
    frame.append('\n');
    m_socket->write(frame);
    emit messageSent(message);
    return true;
}

QString IpcClient::newRequestId()
{
    return QUuid::createUuid().toString(QUuid::WithoutBraces);
}

QString IpcClient::track(const QString &requestId, const QString &type, const QString &action)
{
    m_pending.insert(requestId, Pending{type, action});
    QTimer::singleShot(m_requestTimeoutMs, this, [this, requestId, type] {
        if (m_pending.remove(requestId) > 0)
            emit requestTimedOut(requestId, type);
    });
    return requestId;
}

void IpcClient::sendFocus(const QString &screen, const QString &sectionId, const QString &itemId, qreal scrollX, bool textField)
{
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("focus")},
        {QStringLiteral("screen"), screen},
        {QStringLiteral("section_id"), sectionId.isEmpty() ? QJsonValue() : QJsonValue(sectionId)},
        {QStringLiteral("item_id"), itemId.isEmpty() ? QJsonValue() : QJsonValue(itemId)},
        {QStringLiteral("scroll_x"), scrollX},
        {QStringLiteral("text_field"), textField},
    });
}

void IpcClient::sendInputResult(const QString &requestId, const QString &outcome, const QString &code, const QVariantMap &detail)
{
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("input_result")},
        {QStringLiteral("request_id"), requestId},
        {QStringLiteral("outcome"), outcome},
        {QStringLiteral("code"), code},
        {QStringLiteral("detail"), QJsonObject::fromVariantMap(detail)},
    });
}

void IpcClient::sendConfirmResult(const QString &confirmId, bool accepted)
{
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("confirm_result")},
        {QStringLiteral("confirm_id"), confirmId},
        {QStringLiteral("accepted"), accepted},
    });
}

QString IpcClient::sendRequest(const QString &action, const QJsonObject &args)
{
    const QString id = newRequestId();
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("request")},
        {QStringLiteral("request_id"), id},
        {QStringLiteral("action"), action},
        {QStringLiteral("args"), args},
    });
    return track(id, QStringLiteral("request"), action);
}

QString IpcClient::sendSettingsUpdate(int baseRevision, const QJsonObject &layout)
{
    const QString id = newRequestId();
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("settings.update")},
        {QStringLiteral("request_id"), id},
        {QStringLiteral("base_revision"), baseRevision},
        {QStringLiteral("layout"), layout},
    });
    return track(id, QStringLiteral("settings.update"));
}

QString IpcClient::sendPairIssue(const QString &pass)
{
    const QString id = newRequestId();
    QJsonObject msg{{QStringLiteral("type"), QStringLiteral("pair.issue")}, {QStringLiteral("request_id"), id}};
    if (!pass.isEmpty())
        msg.insert(QStringLiteral("pass"), pass); // a guest pass: tonight | 24h | 7d (contracts/ipc.md)
    send(msg);
    return track(id, QStringLiteral("pair.issue"));
}

QString IpcClient::sendPairCancel()
{
    const QString id = newRequestId();
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("pair.cancel")}, {QStringLiteral("request_id"), id}});
    return track(id, QStringLiteral("pair.cancel"));
}

QString IpcClient::sendDevicesRevoke(const QString &deviceId)
{
    const QString id = newRequestId();
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("devices.revoke")}, {QStringLiteral("request_id"), id}, {QStringLiteral("device_id"), deviceId}});
    return track(id, QStringLiteral("devices.revoke"));
}

QString IpcClient::sendDevicesGrant(const QString &deviceId, const QStringList &permissions)
{
    const QString id = newRequestId();
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("devices.grant")}, {QStringLiteral("request_id"), id},
                     {QStringLiteral("device_id"), deviceId}, {QStringLiteral("permissions"), QJsonArray::fromStringList(permissions)}});
    return track(id, QStringLiteral("devices.grant"));
}

QString IpcClient::sendRemoteConfigure(bool enabled, const QString &transport, const QString &interface, int port, bool httpLayoutEditing)
{
    const QString id = newRequestId();
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("remote.configure")},
        {QStringLiteral("request_id"), id},
        {QStringLiteral("enabled"), enabled},
        {QStringLiteral("transport"), transport},
        {QStringLiteral("interface"), interface},
        {QStringLiteral("port"), port},
        {QStringLiteral("http_layout_editing"), httpLayoutEditing},
        {QStringLiteral("lan_consent"), true},
    });
    return track(id, QStringLiteral("remote.configure"));
}

QString IpcClient::sendInstallRequest(const QString &appId)
{
    const QString id = newRequestId();
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("applications.install_request")}, {QStringLiteral("request_id"), id}, {QStringLiteral("app_id"), appId}});
    return track(id, QStringLiteral("applications.install_request"));
}

QString IpcClient::sendWeatherSearch(const QString &query)
{
    const QString id = newRequestId();
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("weather.search")}, {QStringLiteral("request_id"), id}, {QStringLiteral("query"), query}});
    return track(id, QStringLiteral("weather.search"));
}

QString IpcClient::sendWeatherConfigure(bool enabled, const QJsonValue &place, const QString &units, bool scene)
{
    const QString id = newRequestId();
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("weather.configure")},
        {QStringLiteral("request_id"), id},
        {QStringLiteral("enabled"), enabled},
        {QStringLiteral("place"), place},
        {QStringLiteral("units"), units},
        {QStringLiteral("scene"), scene},
    });
    return track(id, QStringLiteral("weather.configure"));
}

QString IpcClient::sendPlaybackSet(const QString &adapter, const QString &setting, const QString &value)
{
    const QString id = newRequestId();
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("playback.set")},
        {QStringLiteral("request_id"), id},
        {QStringLiteral("adapter"), adapter},
        {QStringLiteral("setting"), setting},
        {QStringLiteral("value"), value},
    });
    return track(id, QStringLiteral("playback.set"));
}

QString IpcClient::sendRemoteNowPlaying(bool enabled)
{
    const QString id = newRequestId();
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("remote.now_playing")},
        {QStringLiteral("request_id"), id},
        {QStringLiteral("enabled"), enabled},
    });
    return track(id, QStringLiteral("remote.now_playing"));
}

QString IpcClient::sendAppEnable(const QString &appId, bool enabled)
{
    const QString id = newRequestId();
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("app.enable")},
        {QStringLiteral("request_id"), id},
        {QStringLiteral("app_id"), appId},
        {QStringLiteral("enabled"), enabled},
    });
    return track(id, QStringLiteral("app.enable"));
}

void IpcClient::sendPowerActivity()
{
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("power.activity")}});
}

QString IpcClient::sendCECConfigure(bool enabled, const QString &volumeTarget)
{
    const QString id = newRequestId();
    send(QJsonObject{
        {QStringLiteral("type"), QStringLiteral("cec.configure")},
        {QStringLiteral("request_id"), id},
        {QStringLiteral("enabled"), enabled},
        {QStringLiteral("volume_target"), volumeTarget},
    });
    return track(id, QStringLiteral("cec.configure"));
}

QString IpcClient::sendPlex(const QString &type, const QJsonObject &fields)
{
    const QString id = newRequestId();
    QJsonObject message = fields;
    message.insert(QStringLiteral("type"), type);
    message.insert(QStringLiteral("request_id"), id);
    send(message);
    return track(id, type);
}

QString IpcClient::sendAchievements(const QString &type, const QJsonObject &fields)
{
    const QString id = newRequestId();
    QJsonObject message = fields;
    message.insert(QStringLiteral("type"), type);
    message.insert(QStringLiteral("request_id"), id);
    send(message);
    return track(id, type);
}

void IpcClient::sendAchievementsCelebrated(const QStringList &ids)
{
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("achievements.celebrated")}, {QStringLiteral("ids"), QJsonArray::fromStringList(ids)}});
}

void IpcClient::sendAchievementsEvent(const QString &event)
{
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("achievements.event")}, {QStringLiteral("event"), event}});
}

void IpcClient::sendShellExit(const QString &reason)
{
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("shell.exit")}, {QStringLiteral("reason"), reason}});
    if (m_socket)
        m_socket->waitForBytesWritten(500);
}

void IpcClient::sendPing()
{
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("ping")}});
}

void IpcClient::sendPong()
{
    send(QJsonObject{{QStringLiteral("type"), QStringLiteral("pong")}});
}
