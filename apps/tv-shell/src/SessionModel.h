#pragma once
// SessionModel: the shell's copy of the coordinator's state.schema.json snapshot.
//
// Contract: `applySnapshot` validates the snapshot structurally (required members, enum
// values, item shapes) and either replaces the whole model or rejects it with an error and
// keeps the previous state; there are no partial updates in protocol 1. Every top-level
// member is exposed as a typed property or QVariantMap, and the home-screen rails are derived
// into `sections` with id-preserving updates so focus never moves on refresh. A layout
// preview (`layout_preview`) overrides the effective `layout` until it ends but is never
// reported as persisted: `layoutJson()` always returns the persisted layout.
#include "SectionsModel.h"

#include <QJsonArray>
#include <QJsonObject>
#include <QObject>
#include <QVariantList>
#include <QVariantMap>
#include <QtQml/qqmlregistration.h>

class QQmlEngine;
class QJSEngine;

class SessionModel : public QObject {
    Q_OBJECT
    QML_NAMED_ELEMENT(Session)
    QML_SINGLETON

    Q_PROPERTY(bool loaded READ loaded NOTIFY snapshotChanged)
    Q_PROPERTY(int contextEpoch READ contextEpoch NOTIFY snapshotChanged)
    Q_PROPERTY(QString deviceName READ deviceName NOTIFY snapshotChanged)
    Q_PROPERTY(bool devMode READ devMode NOTIFY snapshotChanged)
    Q_PROPERTY(int configRevision READ configRevision NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantMap session READ session NOTIFY snapshotChanged)
    Q_PROPERTY(bool locked READ locked NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantMap target READ target NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantMap capabilities READ capabilities NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantMap shellFocus READ shellFocus NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantList applications READ applications NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantMap remote READ remote NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantMap pairing READ pairing NOTIFY snapshotChanged)
    Q_PROPERTY(bool pairingActive READ pairingActive NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantList devices READ devices NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantMap layout READ layout NOTIFY layoutChanged)
    Q_PROPERTY(QVariantMap ui READ ui NOTIFY layoutChanged)
    Q_PROPERTY(QVariantList layoutSections READ layoutSections NOTIFY layoutChanged)
    Q_PROPERTY(bool previewActive READ previewActive NOTIFY layoutChanged)
    Q_PROPERTY(QVariantMap layoutPending READ layoutPending NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantList notifications READ notifications NOTIFY snapshotChanged)
    Q_PROPERTY(QVariantMap content READ content NOTIFY snapshotChanged)
    Q_PROPERTY(bool hasContent READ hasContent NOTIFY snapshotChanged)
    /// The latest playback detection test (state.playback; empty until it has run).
    Q_PROPERTY(QVariantMap playback READ playback NOTIFY snapshotChanged)
    /// Local weather (state.weather; empty when absent: off, locked or an older coordinator).
    Q_PROPERTY(QVariantMap weather READ weather NOTIFY snapshotChanged)
    /// Sleep timer and display (state.power; empty when absent: an older coordinator).
    Q_PROPERTY(QVariantMap power READ power NOTIFY snapshotChanged)
    /// TV control over HDMI-CEC (state.cec; empty when absent: an older coordinator).
    Q_PROPERTY(QVariantMap cec READ cec NOTIFY snapshotChanged)
    Q_PROPERTY(SectionsModel *sections READ sections CONSTANT)
    Q_PROPERTY(QString lastError READ lastError NOTIFY snapshotRejected)

public:
    // Private: QML must obtain the shared instance through create(); a public
    // default constructor would make the engine build its own copy.
private:
    explicit SessionModel(QObject *parent = nullptr);
public:
    static SessionModel *create(QQmlEngine *, QJSEngine *);
    static SessionModel *instance();

    /// Validates and applies a full snapshot. Returns false (and sets `lastError`) without
    /// touching the model when the snapshot violates state.schema.json structurally.
    bool applySnapshot(const QJsonObject &snapshot);
    /// Loads a snapshot JSON file (fixture/dev mode). Returns false and sets `lastError` on failure.
    Q_INVOKABLE bool loadFixture(const QString &path);
    /// Applies a draft layout without persisting it (coordinator `layout_preview`).
    void applyLayoutPreview(const QJsonObject &layout);
    /// Drops the draft and returns to the persisted layout.
    void endLayoutPreview();
    /// The persisted layout as JSON for `settings.update`.
    QJsonObject layoutJson() const { return m_layout; }
    /// The raw snapshot as last applied.
    QJsonObject snapshotJson() const { return m_snapshot; }

    bool loaded() const { return m_loaded; }
    int contextEpoch() const { return m_snapshot.value(QStringLiteral("context_epoch")).toInt(); }
    QString deviceName() const { return m_snapshot.value(QStringLiteral("device_name")).toString(); }
    bool devMode() const { return m_snapshot.value(QStringLiteral("dev_mode")).toBool(); }
    int configRevision() const { return m_snapshot.value(QStringLiteral("config_revision")).toInt(); }
    QVariantMap session() const { return m_snapshot.value(QStringLiteral("session")).toObject().toVariantMap(); }
    bool locked() const { return m_snapshot.value(QStringLiteral("session")).toObject().value(QStringLiteral("locked")).toBool(); }
    QVariantMap target() const { return m_snapshot.value(QStringLiteral("target")).toObject().toVariantMap(); }
    QVariantMap capabilities() const { return m_snapshot.value(QStringLiteral("capabilities")).toObject().toVariantMap(); }
    QVariantMap shellFocus() const;
    QVariantList applications() const { return m_snapshot.value(QStringLiteral("applications")).toArray().toVariantList(); }
    QVariantMap remote() const { return m_snapshot.value(QStringLiteral("remote")).toObject().toVariantMap(); }
    QVariantMap pairing() const { return m_snapshot.value(QStringLiteral("pairing")).toObject().toVariantMap(); }
    bool pairingActive() const { return m_snapshot.value(QStringLiteral("pairing")).toObject().value(QStringLiteral("active")).toBool(); }
    QVariantList devices() const { return m_snapshot.value(QStringLiteral("devices")).toArray().toVariantList(); }
    QVariantMap layout() const { return effectiveLayout().toVariantMap(); }
    QVariantMap ui() const { return effectiveLayout().value(QStringLiteral("ui")).toObject().toVariantMap(); }
    QVariantList layoutSections() const { return effectiveLayout().value(QStringLiteral("sections")).toArray().toVariantList(); }
    bool previewActive() const { return m_previewActive; }
    QVariantMap layoutPending() const { return m_snapshot.value(QStringLiteral("layout_pending")).toObject().toVariantMap(); }
    QVariantList notifications() const { return m_snapshot.value(QStringLiteral("notifications")).toArray().toVariantList(); }
    QVariantMap content() const { return m_snapshot.value(QStringLiteral("content")).toObject().toVariantMap(); }
    bool hasContent() const { return m_snapshot.contains(QStringLiteral("content")); }
    QVariantMap playback() const { return m_snapshot.value(QStringLiteral("playback")).toObject().toVariantMap(); }
    QVariantMap weather() const { return m_snapshot.value(QStringLiteral("weather")).toObject().toVariantMap(); }
    QVariantMap power() const { return m_snapshot.value(QStringLiteral("power")).toObject().toVariantMap(); }
    QVariantMap cec() const { return m_snapshot.value(QStringLiteral("cec")).toObject().toVariantMap(); }
    SectionsModel *sections() const { return m_sections; }
    QString lastError() const { return m_lastError; }

    /// The application record with `id`, or an empty map.
    Q_INVOKABLE QVariantMap application(const QString &id) const;
    /// Deep copy of the persisted layout for editing in QML (settings screen).
    Q_INVOKABLE QVariantMap layoutForEdit() const { return m_layout.toVariantMap(); }
    /// Validates a layout object against layout.schema.json structurally.
    static bool validateLayout(const QJsonObject &layout, QString *error);
    /// Validates a snapshot object against state.schema.json structurally.
    static bool validateSnapshot(const QJsonObject &snapshot, QString *error);

signals:
    void snapshotChanged();
    void layoutChanged();
    void snapshotRejected(const QString &error);

private:
    QJsonObject effectiveLayout() const { return m_previewActive ? m_previewLayout : m_layout; }
    void rebuildSections();

    QJsonObject m_snapshot;
    QJsonObject m_layout;
    QJsonObject m_previewLayout;
    bool m_previewActive = false;
    bool m_loaded = false;
    QString m_lastError;
    SectionsModel *m_sections = nullptr;
};
