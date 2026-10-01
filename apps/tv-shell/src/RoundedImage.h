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
//
// Memory and time: setting the source reads only the image header. The
// pixels are decoded when the item first paints, at the size it is drawn
// (JPEGs decode straight at that size), so a hidden item (a screen not
// opened yet) costs nothing and a 2560×1440 wallpaper on a 360-pixel theme
// card is decoded at card size, not 15 MB.
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
    // The size of the pixels decoded so far (empty before the first paint):
    // for checks that artwork is decoded at the size drawn, not its own.
    Q_PROPERTY(QSize decodedSize READ decodedSize)

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
    // The source exists and its header is readable (the pixels are decoded on
    // first paint; a file that then fails to decode turns this false).
    bool ready() const { return m_sourceSize.isValid(); }
    QSize decodedSize() const { return m_image.size(); }

    void paint(QPainter *painter) override;

signals:
    void sourceChanged();
    void styleChanged();

private:
    QUrl m_source;
    // The source as a QImageReader path (local file or ":/" resource), its
    // full size from the header (invalid: nothing to paint), and whether it is
    // vector art (SVG renders sharp at any size, so it may decode above it).
    QString m_path;
    QSize m_sourceSize;
    bool m_vector = false;
    // The decoded pixels, at m_decodedScale of the source size: only as many
    // as the largest size drawn so far needs.
    QImage m_image;
    qreal m_decodedScale = 0;
    qreal m_radius = 0;
    qreal m_coverage = 0.68;
    qreal m_fade = 0.55;
    int m_pixelSize = 1;
    // The composed image × fade layer for m_layerSize (null: recompose).
    QImage m_layer;
    QSize m_layerSize;
    QImage compose(const QSize &size) const;
    bool decodeFor(const QSize &small);
    void decodeFailed();
};
