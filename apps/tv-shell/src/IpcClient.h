#pragma once
// IpcClient: the shell side of contracts/ipc.md (protocol 1) over a Unix-domain socket.
//
// Contract: newline-delimited JSON frames, 256 KiB maximum (an oversized frame closes the
// connection); hello/welcome/reject handshake — a `reject` for an unsupported protocol is
// final, the shell never falls back; ping every 5 s and a disconnect after 15 s of silence;
// automatic reconnection with 0.5 s → 5 s backoff. Every coordinator→shell message type is
// surfaced as a typed signal and every shell→coordinator message has a typed sender.
// Shell-originated requests are correlated by request_id and time out after
// `requestTimeoutMs`. In offline mode (fixtures, tests) nothing touches a socket and every
// outgoing message is recorded in `sentMessages()`.
#include <QJsonObject>
#include <QList>
#include <QObject>
#include <QVariantMap>
#include <QtQml/qqmlregistration.h>

class QLocalSocket;
class QTimer;

class IpcClient : public QObject {
    Q_OBJECT
    QML_ELEMENT
    QML_UNCREATABLE("Owned by ShellController")
    Q_PROPERTY(QString connectionState READ connectionState NOTIFY connectionStateChanged)
    Q_PROPERTY(QString rejectReason READ rejectReason NOTIFY connectionStateChanged)
    Q_PROPERTY(QString socketPath READ socketPath NOTIFY connectionStateChanged)
    Q_PROPERTY(QString sessionId READ sessionId NOTIFY connectionStateChanged)
    Q_PROPERTY(QString coordinatorVersion READ coordinatorVersion NOTIFY connectionStateChanged)
    Q_PROPERTY(int attempt READ attempt NOTIFY connectionStateChanged)

public:
    static constexpr int kProtocol = 1;
    static constexpr qint64 kMaxFrameBytes = 262144;
    static constexpr int kPingIntervalMs = 5000;
    static constexpr int kSilenceTimeoutMs = 15000;

    explicit IpcClient(QObject *parent = nullptr);

    /// Socket path; defaults to $XDG_RUNTIME_DIR/bear-den-tv/shell.sock.
    void setSocketPath(const QString &path);
    /// Offline mode records outgoing messages instead of connecting.
    void setOffline(bool offline);
    bool offline() const { return m_offline; }
    /// Overrides for tests: keepalive and backoff timing.
    void setTimings(int pingMs, int silenceMs, int backoffMinMs, int backoffMaxMs, int requestTimeoutMs);

    /// Starts connecting (no-op offline).
    void start();
    /// Closes the socket and stops reconnecting.
    void stop();

    /// "offline" | "disconnected" | "connecting" | "handshaking" | "connected" | "rejected".
    QString connectionState() const { return m_state; }
    QString rejectReason() const { return m_rejectReason; }
    QString socketPath() const { return m_socketPath; }
    QString sessionId() const { return m_sessionId; }
    QString coordinatorVersion() const { return m_coordinatorVersion; }
    int attempt() const { return m_attempt; }
    bool isConnected() const { return m_state == QLatin1String("connected"); }

    /// Sends one frame; offline mode records it. Returns false when not connected.
    bool send(const QJsonObject &message);
    /// Messages recorded in offline mode, oldest first.
    QList<QJsonObject> sentMessages() const { return m_sent; }
    Q_INVOKABLE void clearSent() { m_sent.clear(); }
    /// Processes one received frame as if it came from the socket (tests).
    void handleFrame(const QByteArray &line);

    // --- shell → coordinator ---------------------------------------------------------
    void sendFocus(const QString &screen, const QString &sectionId, const QString &itemId, qreal scrollX, bool textField = false);
    void sendInputResult(const QString &requestId, const QString &outcome, const QString &code, const QVariantMap &detail);
    void sendConfirmResult(const QString &confirmId, bool accepted);
    /// Shell-originated action; returns the request id.
    QString sendRequest(const QString &action, const QJsonObject &args);
    QString sendSettingsUpdate(int baseRevision, const QJsonObject &layout);
    /// pair.issue; a non-empty pass (tonight, 24h, 7d) issues a guest pass.
    QString sendPairIssue(const QString &pass = QString());
    QString sendPairCancel();
    QString sendDevicesRevoke(const QString &deviceId);
    QString sendDevicesGrant(const QString &deviceId, const QStringList &permissions);
    QString sendRemoteConfigure(bool enabled, const QString &transport, const QString &interface, int port, bool httpLayoutEditing);
    QString sendInstallRequest(const QString &appId);
    /// One app's playback setting chosen by hand; value "" returns it to automatic.
    /// weather.search: a city query; the coordinator answers with `weather_places`.
    QString sendWeatherSearch(const QString &query);
    /// weather.configure: the whole weather block (place null or {name, region, country, latitude, longitude}).
    QString sendWeatherConfigure(bool enabled, const QJsonValue &place, const QString &units, bool scene);
    QString sendPlaybackSet(const QString &adapter, const QString &setting, const QString &value);
    QString sendRemoteNowPlaying(bool enabled);
    /// plex.sign_in | plex.cancel | plex.choose_server | plex.choose_libraries | plex.sign_out
    /// (contracts/ipc.md) with the given extra fields; answered with result.
    QString sendPlex(const QString &type, const QJsonObject &fields = {});
    /// achievements.configure | achievements.reset (contracts/ipc.md) with
    /// the given extra fields; answered with result.
    QString sendAchievements(const QString &type, const QJsonObject &fields = {});
    // achievements.celebrated / achievements.event: no reply (shell only).
    void sendAchievementsCelebrated(const QStringList &ids);
    void sendAchievementsEvent(const QString &event);
    void sendShellExit(const QString &reason);
    // power.activity: a TV key the shell swallowed while the display was off
    // or the sleep warning showed (contracts/ipc.md). No reply.
    void sendPowerActivity();
    void sendPing();
    void sendPong();

signals:
    void connectionStateChanged();
    void welcomed(const QString &sessionId, const QString &coordinatorVersion);
    void rejected(const QString &reason);
    void disconnectedFromCoordinator(const QString &reason);
    void stateReceived(const QJsonObject &state);
    void inputReceived(const QString &requestId, const QString &action, const QVariantMap &args, int contextEpoch);
    void homeReceived(const QString &requestId, bool restoreFocus);
    void layoutPreviewReceived(const QJsonObject &layout, int expiresInS);
    void layoutPreviewEnded();
    void confirmRequested(const QString &confirmId, const QString &kind, const QString &summary, int expiresInS);
    void notifyReceived(const QString &id, const QString &kind, const QString &text);
    void shutdownReceived(const QString &reason);
    void actionResultReceived(const QString &requestId, const QString &action, const QString &outcome, const QString &code, const QString &message, const QVariantMap &detail);
    void settingsResultReceived(const QString &requestId, bool ok, int revision, const QString &error);
    void replyReceived(const QString &requestId, const QString &type, const QJsonObject &payload);
    void requestTimedOut(const QString &requestId, const QString &type);
    void messageSent(const QJsonObject &message);

private:
    struct Pending {
        QString type;
        QString action;
    };

    void openSocket();
    void onConnected();
    void onReadyRead();
    void onSocketError();
    void abortConnection(const QString &reason, bool reconnect);
    void scheduleReconnect();
    void setState(const QString &state);
    QString track(const QString &requestId, const QString &type, const QString &action = QString());
    bool peerIsSameUser() const;
    static QString newRequestId();

    QLocalSocket *m_socket = nullptr;
    QTimer *m_reconnectTimer = nullptr;
    QTimer *m_pingTimer = nullptr;
    QTimer *m_silenceTimer = nullptr;
    QByteArray m_buffer;
    QString m_socketPath;
    QString m_state{QStringLiteral("disconnected")};
    QString m_rejectReason;
    QString m_sessionId;
    QString m_coordinatorVersion;
    bool m_offline = false;
    bool m_stopped = true;
    int m_attempt = 0;
    int m_backoffMs = 500;
    int m_backoffMinMs = 500;
    int m_backoffMaxMs = 5000;
    int m_pingMs = kPingIntervalMs;
    int m_silenceMs = kSilenceTimeoutMs;
    int m_requestTimeoutMs = 10000;
    QList<QJsonObject> m_sent;
    QHash<QString, Pending> m_pending;
};
