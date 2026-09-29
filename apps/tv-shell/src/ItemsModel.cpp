// ItemsModel implementation: id-keyed reconcile of one section's items
// (contract in ItemsModel.h).

#include "ItemsModel.h"

ItemsModel::ItemsModel(QObject *parent) : QAbstractListModel(parent) {}

int ItemsModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : int(m_items.size());
}

QVariant ItemsModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() < 0 || index.row() >= m_items.size())
        return {};
    const Item &it = m_items.at(index.row());
    switch (role) {
    case ItemIdRole: return it.id;
    case KindRole: return it.kind;
    case TitleRole: return it.title;
    case SubtitleRole: return it.subtitle;
    case ProgressRole: return it.progress;
    case DemoRole: return it.demo;
    case InstalledRole: return it.installed;
    case RunningRole: return it.running;
    case LaunchStateRole: return it.launchState;
    case AppIdRole: return it.appId;
    case TintRole: return it.tint;
    case OpenActionRole: return it.openAction;
    case ArtworkRole: return it.artwork;
    case InstallStateRole: return it.installState;
    case InstallProgressRole: return it.installProgress;
    default: return {};
    }
}

QHash<int, QByteArray> ItemsModel::roleNames() const
{
    return {
        {ItemIdRole, "itemId"},   {KindRole, "kind"},           {TitleRole, "title"},
        {SubtitleRole, "subtitle"}, {ProgressRole, "progress"}, {DemoRole, "demo"},
        {InstalledRole, "installed"}, {RunningRole, "running"}, {LaunchStateRole, "launchState"},
        {AppIdRole, "appId"},     {TintRole, "tint"},           {OpenActionRole, "openAction"},
        {ArtworkRole, "artwork"}, {InstallStateRole, "installState"}, {InstallProgressRole, "installProgress"},
    };
}

void ItemsModel::reconcile(const QList<Item> &items)
{
    // 1. Remove rows whose id is gone (from the back so indexes stay valid).
    QSet<QString> incoming;
    for (const Item &it : items)
        incoming.insert(it.id);
    for (int row = int(m_items.size()) - 1; row >= 0; --row) {
        if (!incoming.contains(m_items.at(row).id)) {
            beginRemoveRows({}, row, row);
            m_items.removeAt(row);
            endRemoveRows();
        }
    }
    // 2. Walk the target order: reuse (move) existing rows, insert new ones, refresh data.
    for (int target = 0; target < items.size(); ++target) {
        const Item &wanted = items.at(target);
        int current = -1;
        for (int row = target; row < m_items.size(); ++row) {
            if (m_items.at(row).id == wanted.id) {
                current = row;
                break;
            }
        }
        if (current < 0) {
            beginInsertRows({}, target, target);
            m_items.insert(target, wanted);
            endInsertRows();
            continue;
        }
        if (current != target) {
            beginMoveRows({}, current, current, {}, target);
            m_items.move(current, target);
            endMoveRows();
        }
        m_items[target] = wanted;
        emit dataChanged(index(target), index(target));
    }
    emit countChanged();
    emit reconciled();
}

int ItemsModel::indexOfId(const QString &id) const
{
    for (int row = 0; row < m_items.size(); ++row)
        if (m_items.at(row).id == id)
            return row;
    return -1;
}

QString ItemsModel::idAt(int row) const
{
    return row >= 0 && row < m_items.size() ? m_items.at(row).id : QString();
}

QVariantMap ItemsModel::get(int row) const
{
    QVariantMap map;
    if (row < 0 || row >= m_items.size())
        return map;
    const QModelIndex idx = index(row);
    const auto names = roleNames();
    for (auto it = names.cbegin(); it != names.cend(); ++it)
        map.insert(QString::fromUtf8(it.value()), data(idx, it.key()));
    return map;
}
