#pragma once
// FocusMemory: per-section memory of the focused item, its index, and the rail scroll offset.
//
// Contract: keyed by stable section id; `itemFor` restores by item id, `indexFor` gives the
// nearest-index fallback when the id vanished, and `scrollFor` the horizontal offset. The
// home screen also records which section was focused last (`lastSectionId`) so `home` and a
// return from an app land on the same tile. Seeded once from state.shell.focus.
#include <QHash>
#include <QObject>
#include <QtQml/qqmlregistration.h>

class QQmlEngine;
class QJSEngine;

class FocusMemory : public QObject {
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON
    Q_PROPERTY(QString lastSectionId READ lastSectionId NOTIFY changed)

public:
    struct Entry {
        QString itemId;
        int index = -1;
        qreal scrollX = 0;
    };

    // Private: QML must obtain the shared instance through create(); a public
    // default constructor would make the engine build its own copy.
private:
    explicit FocusMemory(QObject *parent = nullptr);
public:
    static FocusMemory *create(QQmlEngine *, QJSEngine *);
    static FocusMemory *instance();

    QString lastSectionId() const { return m_lastSectionId; }

    /// Records the focused item of a section; also marks the section as last focused.
    Q_INVOKABLE void remember(const QString &sectionId, const QString &itemId, int index, qreal scrollX);
    /// Remembered item id for a section, empty when none.
    Q_INVOKABLE QString itemFor(const QString &sectionId) const;
    /// Remembered index for a section, -1 when none.
    Q_INVOKABLE int indexFor(const QString &sectionId) const;
    /// Remembered horizontal scroll offset, 0 when none.
    Q_INVOKABLE qreal scrollFor(const QString &sectionId) const;
    /// Seeds memory from the coordinator's stored focus when nothing is remembered yet.
    Q_INVOKABLE void seed(const QString &sectionId, const QString &itemId);
    /// Forgets everything (tests).
    Q_INVOKABLE void clear();

signals:
    void changed();

private:
    QHash<QString, Entry> m_entries;
    QString m_lastSectionId;
};
