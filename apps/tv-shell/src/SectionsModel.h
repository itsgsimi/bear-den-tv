#pragma once
// SectionsModel: the visible home-screen rails in layout order, keyed by section id.
//
// Contract: a section is a row when it is enabled and not (hide_when_empty and empty).
// Each row owns an ItemsModel that survives rebuilds, so QML rails keep the same model
// object (and therefore their current item) across state snapshots. Rebuilds are
// id-preserving like ItemsModel::reconcile.
#include "ItemsModel.h"

#include <QAbstractListModel>
#include <QHash>
#include <QtQml/qqmlregistration.h>

class SectionsModel : public QAbstractListModel {
    Q_OBJECT
    QML_ELEMENT
    QML_UNCREATABLE("Owned by SessionModel")
    Q_PROPERTY(int count READ rowCount NOTIFY countChanged)

public:
    enum Role {
        SectionIdRole = Qt::UserRole + 1,
        TitleRole,
        KindRole,
        ItemsRole,       // ItemsModel*
        IsEmptyRole,     // true when only the synthetic setup card is present
        EmptyMessageRole,
        IsApplicationsRole,
    };

    struct Section {
        QString id;
        QString title;
        QString kind;
        bool isEmpty = false;
        QString emptyMessage;
        QList<ItemsModel::Item> items;
    };

    explicit SectionsModel(QObject *parent = nullptr);

    int rowCount(const QModelIndex &parent = QModelIndex()) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QHash<int, QByteArray> roleNames() const override;

    /// Rebuilds rows from `sections` (already filtered to visible) preserving ItemsModel objects.
    void reconcile(const QList<Section> &sections);

    /// Row of the section, or -1.
    Q_INVOKABLE int indexOfSection(const QString &id) const;
    /// Section id at the row, or empty.
    Q_INVOKABLE QString sectionIdAt(int row) const;
    /// The section's ItemsModel, or null.
    Q_INVOKABLE ItemsModel *itemsFor(const QString &id) const;

signals:
    void countChanged();
    void reconciled();

private:
    struct Row {
        Section section;
        ItemsModel *items = nullptr;
    };
    QList<Row> m_rows;
    QHash<QString, ItemsModel *> m_models;
};
