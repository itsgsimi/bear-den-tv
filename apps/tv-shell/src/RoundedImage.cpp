// RoundedImage implementation: loads and paints rounded, faded artwork,
// smooth or pixelated, composing it once per source and size (contract in
// RoundedImage.h).

#include "RoundedImage.h"

#include <QLinearGradient>
#include <QPainter>
#include <QPainterPath>

RoundedImage::RoundedImage(QQuickItem *parent) : QQuickPaintedItem(parent)
{
    setAntialiasing(true);
}

void RoundedImage::setSource(const QUrl &url)
{
    if (url == m_source)
        return;
    m_source = url;
    if (url.isLocalFile())
        m_image = QImage(url.toLocalFile());
    else if (url.scheme() == QLatin1String("qrc"))
        m_image = QImage(QLatin1Char(':') + url.path());
    else
        m_image = QImage();
    m_layer = QImage();
    emit sourceChanged();
    update();
}

void RoundedImage::setRadius(qreal r)
{
    if (qFuzzyCompare(r, m_radius))
        return;
    m_radius = r;
    emit styleChanged();
    update();
}

void RoundedImage::setCoverage(qreal c)
{
    c = qBound<qreal>(0.1, c, 1.0);
    if (qFuzzyCompare(c, m_coverage))
        return;
    m_coverage = c;
    m_layer = QImage();
    emit styleChanged();
    update();
}

void RoundedImage::setFade(qreal f)
{
    f = qBound<qreal>(0.0, f, 1.0);
    if (qFuzzyCompare(f, m_fade))
        return;
    m_fade = f;
    m_layer = QImage();
    emit styleChanged();
    update();
}

void RoundedImage::setPixelSize(int p)
{
    p = qBound(1, p, 32);
    if (p == m_pixelSize)
        return;
    m_pixelSize = p;
    m_layer = QImage();
    emit styleChanged();
    update();
}

// compose crops the source to size's aspect ratio (PreserveAspectCrop) and
// multiplies it by a horizontal alpha ramp. With pixelSize > 1 both happen at
// 1/pixelSize resolution and the result is enlarged with nearest-neighbour
// scaling, so the picture and its fade are made of whole art pixels.
QImage RoundedImage::compose(const QSize &size) const
{
    const int p = qMax(1, m_pixelSize);
    const QSize small((size.width() + p - 1) / p, (size.height() + p - 1) / p);
    const qreal targetAspect = qreal(small.width()) / qreal(small.height());
    QRectF src(QPointF(0, 0), m_image.size());
    if (src.width() / src.height() > targetAspect) {
        const qreal w = src.height() * targetAspect;
        src = QRectF((src.width() - w) / 2, 0, w, src.height());
    } else {
        const qreal h = src.width() / targetAspect;
        src = QRectF(0, (src.height() - h) / 2, src.width(), h);
    }
    QImage layer(small, QImage::Format_ARGB32_Premultiplied);
    layer.fill(Qt::transparent);
    {
        QPainter p(&layer);
        p.setRenderHint(QPainter::SmoothPixmapTransform);
        p.drawImage(QRectF(QPointF(0, 0), QSizeF(small)), m_image, src);
        p.setCompositionMode(QPainter::CompositionMode_DestinationIn);
        QLinearGradient ramp(0, 0, small.width(), 0);
        ramp.setColorAt(0, QColor(0, 0, 0, 0));
        ramp.setColorAt(m_fade, QColor(0, 0, 0, 255));
        ramp.setColorAt(1, QColor(0, 0, 0, 255));
        p.fillRect(layer.rect(), ramp);
    }
    if (p > 1)
        layer = layer.scaled(small * p, Qt::IgnoreAspectRatio, Qt::FastTransformation).copy(QRect(QPoint(0, 0), size));
    return layer;
}

void RoundedImage::paint(QPainter *painter)
{
    if (m_image.isNull() || width() <= 0 || height() <= 0)
        return;
    const QRectF target(width() * (1 - m_coverage), 0, width() * m_coverage, height());
    const QSize size = target.size().toSize();
    if (size.isEmpty())
        return;
    if (m_layer.isNull() || m_layerSize != size) {
        m_layer = compose(size);
        m_layerSize = size;
    }
    QPainterPath shape;
    shape.addRoundedRect(QRectF(0, 0, width(), height()), m_radius, m_radius);
    painter->setRenderHint(QPainter::Antialiasing);
    painter->setClipPath(shape);
    painter->drawImage(target.topLeft(), m_layer);
}
