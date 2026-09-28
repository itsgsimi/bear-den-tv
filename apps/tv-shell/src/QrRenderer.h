#pragma once
// QrRenderer: paints the `pairing.qr_modules` rows (strings of 0/1) as a QR code image.
//
// Contract: the coordinator encodes; the shell only rasterizes. A quiet zone of
// `quietZone` modules surrounds the symbol, modules are drawn as crisp squares scaled to the
// item size, and an empty module list paints nothing. Registered in QML as `QrCode`.
#include <QColor>
#include <QQuickPaintedItem>
#include <QStringList>
#include <QtQml/qqmlregistration.h>

class QrRenderer : public QQuickPaintedItem {
    Q_OBJECT
    QML_NAMED_ELEMENT(QrCode)
    Q_PROPERTY(QStringList modules READ modules WRITE setModules NOTIFY modulesChanged)
    Q_PROPERTY(QColor dark READ dark WRITE setDark NOTIFY styleChanged)
    Q_PROPERTY(QColor light READ light WRITE setLight NOTIFY styleChanged)
    Q_PROPERTY(int quietZone READ quietZone WRITE setQuietZone NOTIFY styleChanged)
    Q_PROPERTY(int moduleCount READ moduleCount NOTIFY modulesChanged)

public:
    explicit QrRenderer(QQuickItem *parent = nullptr);

    QStringList modules() const { return m_modules; }
    void setModules(const QStringList &rows);
    QColor dark() const { return m_dark; }
    void setDark(const QColor &c);
    QColor light() const { return m_light; }
    void setLight(const QColor &c);
    int quietZone() const { return m_quietZone; }
    void setQuietZone(int modules);
    /// Number of modules per side (rows), 0 when empty.
    int moduleCount() const { return int(m_modules.size()); }

    void paint(QPainter *painter) override;

signals:
    void modulesChanged();
    void styleChanged();

private:
    QStringList m_modules;
    QColor m_dark{Qt::black};
    QColor m_light{Qt::white};
    int m_quietZone = 4;
};
