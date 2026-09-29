#pragma once
// RoundedImage: a QML item that paints an image with rounded corners, cover
// crop and edge fade, smooth or pixelated (implementation in RoundedImage.cpp).

#include <QImage>
#include <QQuickPaintedItem>
#include <QUrl>
#include <QtQml/qqmlregistration.h>

// Artwork backdrop painted without shaders: the image covers the right
// `coverage` fraction of the item, is clipped to the item's rounded rect,
// and fades to transparent towards its left edge. Sources: local files
// (coordinator-cached artwork) and bundled qrc: images; anything else paints nothing.
class RoundedImage : public QQuickPaintedItem {
    Q_OBJECT
    QML_ELEMENT
    Q_PROPERTY(QUrl source READ source WRITE setSource NOTIFY sourceChanged)
    Q_PROPERTY(qreal radius READ radius WRITE setRadius NOTIFY styleChanged)
    Q_PROPERTY(qreal coverage READ coverage WRITE setCoverage NOTIFY styleChanged)
    Q_PROPERTY(qreal fade READ fade WRITE setFade NOTIFY styleChanged)
    // Pixel art style: > 1 composes the artwork at 1/pixelSize resolution and
    // enlarges it with nearest-neighbour scaling (once per source and size;
    // the composed layer is cached, never redone per frame). 1 = smooth.
    Q_PROPERTY(int pixelSize READ pixelSize WRITE setPixelSize NOTIFY styleChanged)
    Q_PROPERTY(bool ready READ ready NOTIFY sourceChanged)

public:
    explicit RoundedImage(QQuickItem *parent = nullptr);

    QUrl source() const { return m_source; }
    void setSource(const QUrl &url);
    qreal radius() const { return m_radius; }
    void setRadius(qreal r);
    qreal coverage() const { return m_coverage; }
    void setCoverage(qreal c);
    qreal fade() const { return m_fade; }
    void setFade(qreal f);
    int pixelSize() const { return m_pixelSize; }
    void setPixelSize(int p);
    bool ready() const { return !m_image.isNull(); }

    void paint(QPainter *painter) override;

signals:
    void sourceChanged();
    void styleChanged();

private:
    QUrl m_source;
    QImage m_image;
    qreal m_radius = 0;
    qreal m_coverage = 0.68;
    qreal m_fade = 0.55;
    int m_pixelSize = 1;
    // The composed image × fade layer for m_layerSize (null: recompose).
    QImage m_layer;
    QSize m_layerSize;
    QImage compose(const QSize &size) const;
};
