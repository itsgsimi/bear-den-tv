// Theme implementation: design tokens derived from layout.ui and window size
// (contract in Theme.h; guide docs/THEMES.md).

#include "Theme.h"
#include "ThemeRegistry.h"

#include <QFontDatabase>
#include <QHash>
#include <QJSEngine>
#include <QQmlEngine>
#include <cmath>

namespace {
Theme *g_instance = nullptr;
}

Theme::Theme(QObject *parent) : QObject(parent)
{
    m_fontFamily = pickFamily({QStringLiteral("Inter"), QStringLiteral("Lato"), QStringLiteral("Noto Sans"),
                               QStringLiteral("Ubuntu"), QStringLiteral("Cantarell"), QStringLiteral("DejaVu Sans")},
                              QStringLiteral("Sans Serif"));
    m_monoFamily = pickFamily({QStringLiteral("Source Code Pro"), QStringLiteral("Inconsolata"),
                               QStringLiteral("DejaVu Sans Mono"), QStringLiteral("Ubuntu Mono")},
                              QStringLiteral("Monospace"));
    if (!g_instance)
        g_instance = this;
}

Theme *Theme::create(QQmlEngine *, QJSEngine *)
{
    if (!g_instance)
        g_instance = new Theme();
    QJSEngine::setObjectOwnership(g_instance, QJSEngine::CppOwnership);
    return g_instance;
}

Theme *Theme::instance()
{
    return g_instance;
}

QString Theme::pickFamily(const QStringList &preferred, const QString &fallback)
{
    const QStringList available = QFontDatabase::families();
    for (const QString &family : preferred)
        if (available.contains(family))
            return family;
    return fallback;
}

void Theme::applyUi(const QVariantMap &ui)
{
    if (ui.contains(QStringLiteral("accent")))
        m_accent = QColor(ui.value(QStringLiteral("accent")).toString());
    if (ui.contains(QStringLiteral("background")))
        m_background = ui.value(QStringLiteral("background")).toString();
    if (ui.contains(QStringLiteral("theme")))
        m_style = ui.value(QStringLiteral("theme")).toString();
    if (ui.contains(QStringLiteral("text_scale")))
        m_textScale = qBound(0.8, ui.value(QStringLiteral("text_scale")).toDouble(), 2.0);
    if (ui.contains(QStringLiteral("tile_density")))
        m_tileDensity = ui.value(QStringLiteral("tile_density")).toString();
    if (ui.contains(QStringLiteral("safe_margin_percent")))
        m_safeMarginPercent = qBound(0.0, ui.value(QStringLiteral("safe_margin_percent")).toDouble(), 10.0);
    if (ui.contains(QStringLiteral("reduced_motion")))
        m_reducedMotion = ui.value(QStringLiteral("reduced_motion")).toBool()
            || ui.value(QStringLiteral("theme")).toString() == QStringLiteral("performance");
    if (ui.contains(QStringLiteral("high_contrast_focus")))
        m_highContrastFocus = ui.value(QStringLiteral("high_contrast_focus")).toBool();
    if (ui.contains(QStringLiteral("hero_enabled")))
        m_heroEnabled = ui.value(QStringLiteral("hero_enabled")).toBool();
    if (ui.contains(QStringLiteral("clock_enabled")))
        m_clockEnabled = ui.value(QStringLiteral("clock_enabled")).toBool();
    // Always a full ui here, so a missing art_style means pixel.
    m_artStyle = ui.value(QStringLiteral("art_style")).toString() == QLatin1String("classic") ? QStringLiteral("classic") : QStringLiteral("pixel");
    // Missing means app (the app's own icon).
    m_appIcons = ui.value(QStringLiteral("app_icons")).toString() == QLatin1String("bear_den") ? QStringLiteral("bear_den") : QStringLiteral("app");
    emit tokensChanged();
}

void Theme::setForceNoAnimations(bool force)
{
    if (m_forceNoAnimations == force)
        return;
    m_forceNoAnimations = force;
    emit tokensChanged();
}

void Theme::setWindowWidth(qreal w)
{
    if (qFuzzyCompare(m_windowWidth, w) || w <= 0)
        return;
    m_windowWidth = w;
    emit geometryChanged();
    emit tokensChanged();
}

void Theme::setWindowHeight(qreal h)
{
    if (qFuzzyCompare(m_windowHeight, h) || h <= 0)
        return;
    m_windowHeight = h;
    emit geometryChanged();
    emit tokensChanged();
}

QColor Theme::accentSoft() const
{
    return alpha(m_accent, 0.22);
}

// WCAG 2 relative luminance of an opaque colour.
static double luminance(const QColor &c)
{
    auto channel = [](double v) { return v <= 0.03928 ? v / 12.92 : std::pow((v + 0.055) / 1.055, 2.4); };
    return 0.2126 * channel(c.redF()) + 0.7152 * channel(c.greenF()) + 0.0722 * channel(c.blueF());
}

double Theme::contrastRatio(const QColor &a, const QColor &b)
{
    const double la = luminance(a), lb = luminance(b);
    return (std::max(la, lb) + 0.05) / (std::min(la, lb) + 0.05);
}

QColor Theme::onAccent() const
{
    const QColor dark = pillActiveText(), light = textPrimary();
    return contrastRatio(m_accent, dark) >= contrastRatio(m_accent, light) ? dark : light;
}

QColor Theme::accentText() const
{
    // Lightened accent that stays readable on dark surfaces.
    return m_accent.lighter(125);
}

// The background comes from the chosen theme package (ThemeRegistry) in the
// chosen art style: its wallpaper image and the gradient behind it.
static QVariantMap wallpaperOf(const QString &background, const QString &artStyle)
{
    if (const ThemeRegistry *r = ThemeRegistry::instance())
        return r->get(background, artStyle).value(QStringLiteral("wallpaper")).toMap();
    return {};
}

QColor Theme::bgTop() const
{
    const QString c = wallpaperOf(m_background, m_artStyle).value(QStringLiteral("top")).toString();
    return c.isEmpty() ? QColor(0x1F, 0x2A, 0x26) : QColor(c);
}

QColor Theme::bgBottom() const
{
    const QString c = wallpaperOf(m_background, m_artStyle).value(QStringLiteral("bottom")).toString();
    return c.isEmpty() ? QColor(0x0C, 0x0E, 0x10) : QColor(c);
}

QUrl Theme::wallpaperSource() const
{
    return QUrl(wallpaperOf(m_background, m_artStyle).value(QStringLiteral("image")).toString());
}

QColor Theme::focusColor() const
{
    return m_highContrastFocus ? QColor(Qt::white) : m_accent;
}

qreal Theme::focusWidth() const
{
    return px(m_highContrastFocus ? 7 : 4);
}

qreal Theme::tileWidth() const
{
    return px(368) * densityFactor();
}

qreal Theme::tileHeight() const
{
    return px(207) * densityFactor();
}

qreal Theme::cardWidth() const
{
    return px(392) * densityFactor();
}

qreal Theme::cardHeight() const
{
    return px(220) * densityFactor();
}

QColor Theme::tintFor(const QString &id) const
{
    // Two product apps get fixed, distinguishable hues; everything else hashes to a hue.
    if (id == QLatin1String("plex-htpc"))
        return QColor(0xC7, 0x8A, 0x2B);
    if (id == QLatin1String("youtube"))
        return QColor(0xB8, 0x3A, 0x3A);
    if (id == QLatin1String("moonlight"))
        return QColor(0x3A, 0x6E, 0xA8);
    const uint h = qHash(id) % 360;
    return QColor::fromHsl(int(h), 90, 80);
}

QColor Theme::alpha(const QColor &c, qreal a) const
{
    QColor out = c;
    out.setAlphaF(qBound(0.0, a, 1.0));
    return out;
}
