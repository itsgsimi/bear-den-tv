#pragma once
// Theme: QML singleton exposing the design tokens derived from layout.ui and the window size.
//
// Contract: every visual value used by QML (colors, type sizes, tile sizes, focus treatment,
// animation durations, safe margins) comes from here so a layout change re-themes the whole
// shell without touching components. Reference geometry is 1920x1080 logical; `scale` is
// window.height / 1080 and text additionally multiplies by `textScale`. `reducedMotion`
// collapses every duration to 0; `highContrastFocus` swaps the accent outline for a thicker
// white one. Tokens never encode product text.
#include <QColor>
#include <QObject>
#include <QUrl>
#include <QVariantMap>
#include <QtQml/qqmlregistration.h>

class QQmlEngine;
class QJSEngine;

class Theme : public QObject {
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON

    // layout.ui inputs
    Q_PROPERTY(QColor accent READ accent NOTIFY tokensChanged)
    Q_PROPERTY(QString background READ background NOTIFY tokensChanged)
    // Layout ui.theme: "den-dark" (Bear Den: themed decorations), "plain-dark", or
    // "performance" (Plain, and reducedMotion forced on: nothing animates).
    Q_PROPERTY(QString style READ style NOTIFY tokensChanged)
    // Art style (layout.ui.art_style): "pixel" or "classic" (smooth art).
    Q_PROPERTY(QString artStyle READ artStyle NOTIFY tokensChanged)
    // App icons (layout.ui.app_icons): "app" (the app's own icon, the
    // default) or "bear_den" (Bear Den's drawings); Shell.appArt's third input.
    Q_PROPERTY(QString appIcons READ appIcons NOTIFY tokensChanged)
    Q_PROPERTY(qreal textScale READ textScale NOTIFY tokensChanged)
    Q_PROPERTY(QString tileDensity READ tileDensity NOTIFY tokensChanged)
    Q_PROPERTY(qreal safeMarginPercent READ safeMarginPercent NOTIFY tokensChanged)
    Q_PROPERTY(bool reducedMotion READ reducedMotion NOTIFY tokensChanged)
    Q_PROPERTY(bool highContrastFocus READ highContrastFocus NOTIFY tokensChanged)
    Q_PROPERTY(bool heroEnabled READ heroEnabled NOTIFY tokensChanged)
    Q_PROPERTY(bool clockEnabled READ clockEnabled NOTIFY tokensChanged)
    // True after a while without input: ambient loops stop so the scene graph
    // stops rendering (small TV boxes idle at ~0% CPU). Any input wakes it.
    Q_PROPERTY(bool resting READ resting WRITE setResting NOTIFY restingChanged)
    // True while the OLED screensaver covers the screen: everything stops.
    Q_PROPERTY(bool screensaver READ screensaver WRITE setScreensaver NOTIFY restingChanged)

    // window geometry (set by Main.qml)
    Q_PROPERTY(qreal windowWidth READ windowWidth WRITE setWindowWidth NOTIFY geometryChanged)
    Q_PROPERTY(qreal windowHeight READ windowHeight WRITE setWindowHeight NOTIFY geometryChanged)
    Q_PROPERTY(qreal scale READ scale NOTIFY geometryChanged)
    // Pixels per reference font pixel (window scale × user text size). QML uses
    // `N * Theme.fontUnit` rather than fs(N) so bindings follow text-size changes.
    Q_PROPERTY(qreal fontUnit READ fontUnit NOTIFY tokensChanged)
    Q_PROPERTY(qreal safeX READ safeX NOTIFY tokensChanged)
    Q_PROPERTY(qreal safeY READ safeY NOTIFY tokensChanged)

    // derived tokens
    Q_PROPERTY(QString fontFamily READ fontFamily CONSTANT)
    Q_PROPERTY(QString monoFamily READ monoFamily CONSTANT)
    Q_PROPERTY(QColor accentSoft READ accentSoft NOTIFY tokensChanged)
    Q_PROPERTY(QColor accentText READ accentText NOTIFY tokensChanged)
    // Text on an accent fill (a focused primary button): dark or light,
    // whichever reads better on the accent the owner chose (UX-01).
    Q_PROPERTY(QColor onAccent READ onAccent NOTIFY tokensChanged)
    Q_PROPERTY(QColor bgTop READ bgTop NOTIFY tokensChanged)
    Q_PROPERTY(QColor bgBottom READ bgBottom NOTIFY tokensChanged)
    Q_PROPERTY(QUrl wallpaperSource READ wallpaperSource NOTIFY tokensChanged)
    Q_PROPERTY(QColor surface READ surface CONSTANT)
    Q_PROPERTY(QColor surfaceRaised READ surfaceRaised CONSTANT)
    Q_PROPERTY(QColor surfaceBorder READ surfaceBorder CONSTANT)
    Q_PROPERTY(QColor scrim READ scrim CONSTANT)
    Q_PROPERTY(QColor textPrimary READ textPrimary CONSTANT)
    Q_PROPERTY(QColor textSecondary READ textSecondary CONSTANT)
    Q_PROPERTY(QColor textMuted READ textMuted CONSTANT)
    Q_PROPERTY(QColor pillActiveBg READ pillActiveBg CONSTANT)
    Q_PROPERTY(QColor pillActiveText READ pillActiveText CONSTANT)
    Q_PROPERTY(QColor danger READ danger CONSTANT)
    Q_PROPERTY(QColor warning READ warning CONSTANT)
    Q_PROPERTY(QColor success READ success CONSTANT)
    Q_PROPERTY(QColor demoBadge READ demoBadge CONSTANT)
    Q_PROPERTY(QColor focusColor READ focusColor NOTIFY tokensChanged)
    Q_PROPERTY(qreal focusWidth READ focusWidth NOTIFY tokensChanged)
    Q_PROPERTY(qreal focusScale READ focusScale CONSTANT)
    Q_PROPERTY(int duration READ duration NOTIFY tokensChanged)
    Q_PROPERTY(int durationFast READ durationFast NOTIFY tokensChanged)
    Q_PROPERTY(qreal tileWidth READ tileWidth NOTIFY tokensChanged)
    Q_PROPERTY(qreal tileHeight READ tileHeight NOTIFY tokensChanged)
    Q_PROPERTY(qreal cardWidth READ cardWidth NOTIFY tokensChanged)
    Q_PROPERTY(qreal cardHeight READ cardHeight NOTIFY tokensChanged)
    Q_PROPERTY(qreal railGap READ railGap NOTIFY tokensChanged)
    Q_PROPERTY(qreal radius READ radius NOTIFY tokensChanged)

public:
    // Private: QML must obtain the shared instance through create(); a public
    // default constructor would make the engine build its own copy.
private:
    explicit Theme(QObject *parent = nullptr);
public:

    /// QML singleton factory: returns the host-created instance (or lazily creates one).
    static Theme *create(QQmlEngine *, QJSEngine *);
    /// The process-wide instance; null until constructed.
    static Theme *instance();

    /// Replaces every layout.ui-derived token from a layout.ui object; unknown keys are ignored.
    /// @param ui the `ui` member of a layout.schema.json object (QVariantMap)
    void applyUi(const QVariantMap &ui);
    /// Overrides reduced motion regardless of layout.ui (the `--no-animations` flag).
    void setForceNoAnimations(bool force);

    QColor accent() const { return m_accent; }
    QString background() const { return m_background; }
    QString style() const { return m_style; }
    QString artStyle() const { return m_artStyle; }
    QString appIcons() const { return m_appIcons; }
    qreal textScale() const { return m_textScale; }
    QString tileDensity() const { return m_tileDensity; }
    qreal safeMarginPercent() const { return m_safeMarginPercent; }
    bool reducedMotion() const { return m_reducedMotion || m_forceNoAnimations; }
    bool highContrastFocus() const { return m_highContrastFocus; }
    bool heroEnabled() const { return m_heroEnabled; }
    bool clockEnabled() const { return m_clockEnabled; }
    bool resting() const { return m_resting; }
    bool screensaver() const { return m_screensaver; }
    void setScreensaver(bool on)
    {
        if (m_screensaver == on)
            return;
        m_screensaver = on;
        emit restingChanged();
    }
    void setResting(bool resting)
    {
        if (m_resting == resting)
            return;
        m_resting = resting;
        emit restingChanged();
    }

    qreal windowWidth() const { return m_windowWidth; }
    qreal windowHeight() const { return m_windowHeight; }
    void setWindowWidth(qreal w);
    void setWindowHeight(qreal h);
    qreal scale() const { return m_windowHeight / 1080.0; }
    qreal fontUnit() const { return scale() * m_textScale; }
    qreal safeX() const { return m_windowWidth * m_safeMarginPercent / 100.0; }
    qreal safeY() const { return m_windowHeight * m_safeMarginPercent / 100.0; }

    QString fontFamily() const { return m_fontFamily; }
    QString monoFamily() const { return m_monoFamily; }
    QColor accentSoft() const;
    QColor accentText() const;
    QColor onAccent() const;
    /// WCAG 2 contrast ratio of two opaque colours (1 to 21).
    static double contrastRatio(const QColor &a, const QColor &b);
    QColor bgTop() const;
    QColor bgBottom() const;
    QUrl wallpaperSource() const;
    QColor surface() const { return QColor(0x1A, 0x1E, 0x22, 0xE6); }
    QColor surfaceRaised() const { return QColor(0x26, 0x2B, 0x30); }
    QColor surfaceBorder() const { return QColor(0xFF, 0xFF, 0xFF, 0x1C); }
    QColor scrim() const { return QColor(0x06, 0x08, 0x0A, 0xC8); }
    QColor textPrimary() const { return QColor(0xF4, 0xF6, 0xF5); }
    QColor textSecondary() const { return QColor(0xB4, 0xBC, 0xB8); }
    QColor textMuted() const { return QColor(0x7E, 0x88, 0x83); }
    QColor pillActiveBg() const { return QColor(0xF4, 0xF6, 0xF5); }
    QColor pillActiveText() const { return QColor(0x12, 0x15, 0x17); }
    QColor danger() const { return QColor(0xE0, 0x7A, 0x6A); }
    QColor warning() const { return QColor(0xE3, 0xB3, 0x5C); }
    QColor success() const { return QColor(0x8C, 0xD1, 0x9E); }
    QColor demoBadge() const { return QColor(0xE3, 0xB3, 0x5C); }
    QColor focusColor() const;
    qreal focusWidth() const;
    qreal focusScale() const { return 1.05; }
    int duration() const { return reducedMotion() ? 0 : 150; }
    int durationFast() const { return reducedMotion() ? 0 : 90; }
    qreal tileWidth() const;
    qreal tileHeight() const;
    qreal cardWidth() const;
    qreal cardHeight() const;
    qreal railGap() const { return px(34); }
    qreal radius() const { return px(20); }

    /// Reference pixels (1080p design value) scaled to the window.
    Q_INVOKABLE qreal px(qreal ref) const { return ref * scale(); }
    /// Reference font size scaled to the window and the user's text scale.
    Q_INVOKABLE qreal fs(qreal ref) const { return qMax(8.0, ref * scale() * m_textScale); }
    /// Animation duration honoring reduced motion.
    Q_INVOKABLE int ms(int ref) const { return reducedMotion() ? 0 : ref; }
    /// Deterministic tint for an application or content id (used where no artwork exists).
    Q_INVOKABLE QColor tintFor(const QString &id) const;
    /// Color with replaced alpha (0..1).
    Q_INVOKABLE QColor alpha(const QColor &c, qreal a) const;

signals:
    void tokensChanged();
    void restingChanged();
    void geometryChanged();

private:
    static QString pickFamily(const QStringList &preferred, const QString &fallback);
    qreal densityFactor() const { return m_tileDensity == QLatin1String("large") ? 1.25 : 1.0; }

    QColor m_accent{0x79, 0xA8, 0x89};
    QString m_background{QStringLiteral("den-gradient")};
    QString m_style{QStringLiteral("den-dark")};
    QString m_artStyle{QStringLiteral("pixel")};
    QString m_appIcons{QStringLiteral("app")};
    qreal m_textScale = 1.0;
    QString m_tileDensity{QStringLiteral("comfortable")};
    qreal m_safeMarginPercent = 3.0;
    bool m_reducedMotion = false;
    bool m_highContrastFocus = false;
    bool m_heroEnabled = true;
    bool m_clockEnabled = true;
    bool m_resting = false;
    bool m_screensaver = false;
    bool m_forceNoAnimations = false;
    qreal m_windowWidth = 1920;
    qreal m_windowHeight = 1080;
    QString m_fontFamily;
    QString m_monoFamily;
};
