// SectionsModel implementation: visible home rails in layout order (contract in
// SectionsModel.h).

#include "SectionsModel.h"

#include <QQmlEngine>

SectionsModel::SectionsModel(QObject *parent) : QAbstractListModel(parent) {}

int SectionsModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : int(m_rows.size());
}

QVariant SectionsModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() < 0 || index.row() >= m_rows.size())
        return {};
    const Row &row = m_rows.at(index.row());
    switch (role) {
    case SectionIdRole: return row.section.id;
    case TitleRole: return row.section.title;
    case KindRole: return row.section.kind;
    case ItemsRole: return QVariant::fromValue<QObject *>(row.items);
    case IsEmptyRole: return row.section.isEmpty;
    case EmptyMessageRole: return row.section.emptyMessage;
    case IsApplicationsRole: return row.section.kind == QLatin1String("applications");
    default: return {};
    }
}

QHash<int, QByteArray> SectionsModel::roleNames() const
{
    return {
        {SectionIdRole, "sectionId"}, {TitleRole, "title"},   {KindRole, "kind"},
        {ItemsRole, "items"},         {IsEmptyRole, "isEmpty"}, {EmptyMessageRole, "emptyMessage"},
        {IsApplicationsRole, "isApplications"},
    };
}

void SectionsModel::reconcile(const QList<Section> &sections)
{
    QSet<QString> incoming;
    for (const Section &s : sections)
        incoming.insert(s.id);
    for (int row = int(m_rows.size()) - 1; row >= 0; --row) {
        if (!incoming.contains(m_rows.at(row).section.id)) {
            beginRemoveRows({}, row, row);
            m_rows.removeAt(row);
            endRemoveRows();
        }
    }
    for (int target = 0; target < sections.size(); ++target) {
        const Section &wanted = sections.at(target);
        int current = -1;
        for (int row = target; row < m_rows.size(); ++row) {
            if (m_rows.at(row).section.id == wanted.id) {
                current = row;
                break;
            }
        }
        ItemsModel *&model = m_models[wanted.id];
        if (!model) {
            model = new ItemsModel(this);
            QQmlEngine::setObjectOwnership(model, QQmlEngine::CppOwnership);
        }
        if (current < 0) {
            beginInsertRows({}, target, target);
            m_rows.insert(target, Row{wanted, model});
            endInsertRows();
        } else {
            if (current != target) {
                beginMoveRows({}, current, current, {}, target);
                m_rows.move(current, target);
                endMoveRows();
            }
            m_rows[target].section = wanted;
            emit dataChanged(index(target), index(target));
        }
        model->reconcile(wanted.items);
    }
    emit countChanged();
    emit reconciled();
}

int SectionsModel::indexOfSection(const QString &id) const
{
    for (int row = 0; row < m_rows.size(); ++row)
        if (m_rows.at(row).section.id == id)
            return row;
    return -1;
}

QString SectionsModel::sectionIdAt(int row) const
{
    return row >= 0 && row < m_rows.size() ? m_rows.at(row).section.id : QString();
}

ItemsModel *SectionsModel::itemsFor(const QString &id) const
{
    const int row = indexOfSection(id);
    return row < 0 ? nullptr : m_rows.at(row).items;
}
