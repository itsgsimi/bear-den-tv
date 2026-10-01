// RoundedImage implementation: loads and paints rounded, faded artwork,
// smooth or pixelated, composing it once per source and size (contract in
// RoundedImage.h).

#include "RoundedImage.h"

#include <QImageReader>
#include <QLinearGradient>
#include <QPainter>
#include <QPainterPath>

#include <cmath>

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
        m_path = url.toLocalFile();
    else if (url.scheme() == QLatin1String("qrc"))
        m_path = QLatin1Char(':') + url.path();
    else
        m_path.clear();
    m_sourceSize = QSize();
    m_vector = false;
    if (!m_path.isEmpty()) {
        QImageReader reader(m_path);
        const QSize size = reader.size();
        if (reader.canRead() && size.width() > 0 && size.height() > 0) {
            m_sourceSize = size;
            m_vector = reader.format().startsWith("svg");
        }
    }
    m_image = QImage();
    m_decodedScale = 0;
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

// decodeFor makes sure m_image holds enough pixels to cover `small` (the
// composed layer's size): the source scaled so both sides cover it, with 25%
// headroom so an item that grows a little is not decoded again. Raster images
// never decode above their own size (enlarging happens when drawing, as
// before); vector art renders at the size needed, so it stays sharp. A
// smaller size reuses the pixels already decoded. Returns false when the
// source can't be decoded.
bool RoundedImage::decodeFor(const QSize &small)
{
    const qreal need = qMax(qreal(small.width()) / m_sourceSize.width(), qreal(small.height()) / m_sourceSize.height());
    if (!m_image.isNull() && need <= m_decodedScale)
        return true;
    qreal scale = need * 1.25;
    if (!m_vector)
        scale = qMin<qreal>(1, scale);
    QImageReader reader(m_path);
    const QSize target(qMax(1, int(std::ceil(m_sourceSize.width() * scale))), qMax(1, int(std::ceil(m_sourceSize.height() * scale))));
    if (target != m_sourceSize)
        reader.setScaledSize(target);
    QImage decoded = reader.read();
    if (decoded.isNull())
        return false;
    m_image = std::move(decoded);
    m_decodedScale = scale;
    return true;
}

// decodeFailed: the header read but the pixels did not. paint() runs on the
// scene graph thread, so `ready` turns false on the item's own thread.
void RoundedImage::decodeFailed()
{
    m_sourceSize = QSize();
    m_image = QImage();
    m_decodedScale = 0;
    QMetaObject::invokeMethod(this, [this] { emit sourceChanged(); }, Qt::QueuedConnection);
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
    if (!m_sourceSize.isValid() || width() <= 0 || height() <= 0)
        return;
    const QRectF target(width() * (1 - m_coverage), 0, width() * m_coverage, height());
    const QSize size = target.size().toSize();
    if (size.isEmpty())
        return;
    if (m_layer.isNull() || m_layerSize != size) {
        const int p = qMax(1, m_pixelSize);
        if (!decodeFor(QSize((size.width() + p - 1) / p, (size.height() + p - 1) / p))) {
            decodeFailed();
            return;
        }
        m_layer = compose(size);
        m_layerSize = size;
    }
    QPainterPath shape;
    shape.addRoundedRect(QRectF(0, 0, width(), height()), m_radius, m_radius);
    painter->setRenderHint(QPainter::Antialiasing);
    painter->setClipPath(shape);
    painter->drawImage(target.topLeft(), m_layer);
}
