#pragma once
// ItemsModel: the items of one home-screen section, keyed by stable item id.
//
// Contract: `reconcile` diffs the incoming list against the current rows by id and emits
// row removals, moves, inserts, and data changes instead of a reset, so a QML ListView keeps
// its current item across refreshes and reorders. Rows are application tiles, provider
// content cards, or a single synthetic "setup" card for an enabled-but-empty provider section.
#include <QAbstractListModel>
#include <QColor>
#include <QList>
#include <QtQml/qqmlregistration.h>

class ItemsModel : public QAbstractListModel {
    Q_OBJECT
    QML_ELEMENT
    QML_UNCREATABLE("Owned by SectionsModel")
    Q_PROPERTY(int count READ rowCount NOTIFY countChanged)

public:
    enum Role {
        ItemIdRole = Qt::UserRole + 1,
        KindRole,      // "app" | "content" | "setup"
        TitleRole,
        SubtitleRole,
        ProgressRole,  // -1 when none
        DemoRole,
        InstalledRole,
        RunningRole,
        LaunchStateRole,
        AppIdRole,
        TintRole,
        OpenActionRole,
        ArtworkRole,
        InstallStateRole,    // state.applications[].install.state, "" when absent
        InstallProgressRole, // 0..100
    };

    struct Item {
        QString id;
        QString kind;
        QString title;
        QString subtitle;
        qreal progress = -1;
        bool demo = false;
        bool installed = true;
        bool running = false;
        QString launchState;
        QString appId;
        QColor tint;
        QString openAction;
        QString artwork;
        QString installState;
        int installProgress = 0;
    };

    explicit ItemsModel(QObject *parent = nullptr);

    int rowCount(const QModelIndex &parent = QModelIndex()) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QHash<int, QByteArray> roleNames() const override;

    /// Replaces the rows with `items` using id-preserving structural updates, then emits
    /// `reconciled()` so views can re-apply focus by id.
    void reconcile(const QList<Item> &items);

    /// Row of the item with `id`, or -1.
    Q_INVOKABLE int indexOfId(const QString &id) const;
    /// Id of the row, or empty when out of range.
    Q_INVOKABLE QString idAt(int row) const;
    /// Full row as a map (tests and dialogs).
    Q_INVOKABLE QVariantMap get(int row) const;

signals:
    void countChanged();
    void reconciled();

private:
    QList<Item> m_items;
};
