// QrRenderer implementation: rasterizes pairing.qr_modules into a QR image
// (contract in QrRenderer.h).

#include "QrRenderer.h"

#include <QPainter>

QrRenderer::QrRenderer(QQuickItem *parent) : QQuickPaintedItem(parent)
{
    setAntialiasing(false);
}

void QrRenderer::setModules(const QStringList &rows)
{
    if (m_modules == rows)
        return;
    m_modules = rows;
    emit modulesChanged();
    update();
}

void QrRenderer::setDark(const QColor &c)
{
    if (m_dark == c)
        return;
    m_dark = c;
    emit styleChanged();
    update();
}

void QrRenderer::setLight(const QColor &c)
{
    if (m_light == c)
        return;
    m_light = c;
    emit styleChanged();
    update();
}

void QrRenderer::setQuietZone(int modules)
{
    modules = qMax(0, modules);
    if (m_quietZone == modules)
        return;
    m_quietZone = modules;
    emit styleChanged();
    update();
}

void QrRenderer::paint(QPainter *painter)
{
    const int n = moduleCount();
    if (n <= 0)
        return;
    const int total = n + 2 * m_quietZone;
    const qreal module = qMin(width(), height()) / total;
    const qreal side = module * total;
    const qreal ox = (width() - side) / 2;
    const qreal oy = (height() - side) / 2;
    painter->setPen(Qt::NoPen);
    painter->fillRect(QRectF(ox, oy, side, side), m_light);
    painter->setBrush(m_dark);
    for (int y = 0; y < n; ++y) {
        const QString &row = m_modules.at(y);
        for (int x = 0; x < row.size() && x < n; ++x) {
            if (row.at(x) != QLatin1Char('1'))
                continue;
            // Snap each module to whole pixels so neighbours share edges without seams.
            const qreal left = ox + (x + m_quietZone) * module;
            const qreal top = oy + (y + m_quietZone) * module;
            const qreal right = ox + (x + 1 + m_quietZone) * module;
            const qreal bottom = oy + (y + 1 + m_quietZone) * module;
            painter->drawRect(QRectF(qRound(left), qRound(top), qRound(right) - qRound(left), qRound(bottom) - qRound(top)));
        }
    }
}
