// FocusMemory implementation: per-section focus and scroll memory (contract in
// FocusMemory.h).

#include "FocusMemory.h"

#include <QJSEngine>
#include <QQmlEngine>

namespace {
FocusMemory *g_instance = nullptr;
}

FocusMemory::FocusMemory(QObject *parent) : QObject(parent)
{
    if (!g_instance)
        g_instance = this;
}

FocusMemory *FocusMemory::create(QQmlEngine *, QJSEngine *)
{
    if (!g_instance)
        g_instance = new FocusMemory();
    QJSEngine::setObjectOwnership(g_instance, QJSEngine::CppOwnership);
    return g_instance;
}

FocusMemory *FocusMemory::instance()
{
    return g_instance;
}

void FocusMemory::remember(const QString &sectionId, const QString &itemId, int index, qreal scrollX)
{
    if (sectionId.isEmpty())
        return;
    m_entries.insert(sectionId, Entry{itemId, index, scrollX});
    m_lastSectionId = sectionId;
    emit changed();
}

QString FocusMemory::itemFor(const QString &sectionId) const
{
    return m_entries.value(sectionId).itemId;
}

int FocusMemory::indexFor(const QString &sectionId) const
{
    return m_entries.value(sectionId).index;
}

qreal FocusMemory::scrollFor(const QString &sectionId) const
{
    return m_entries.value(sectionId).scrollX;
}

void FocusMemory::seed(const QString &sectionId, const QString &itemId)
{
    if (sectionId.isEmpty() || m_entries.contains(sectionId))
        return;
    m_entries.insert(sectionId, Entry{itemId, -1, 0});
    if (m_lastSectionId.isEmpty())
        m_lastSectionId = sectionId;
    emit changed();
}

void FocusMemory::clear()
{
    m_entries.clear();
    m_lastSectionId.clear();
    emit changed();
}
