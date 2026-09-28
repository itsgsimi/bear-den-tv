#pragma once
// Themes (QML singleton): the installed theme packages — a folder <id>/ with a
// theme.json manifest (contracts/theme.schema.json) and its art.
//
// Contract:
//  - Built-in themes are compiled in at :/themes/<id>/ (from the repository's
//    themes/ folder). The owner's themes load from $BDTV_THEMES_DIR, else
//    $XDG_DATA_HOME/bear-den-tv/themes/<id>/, and may override a built-in with
//    the same id (logged).
//  - Each manifest is checked against the schema's rules (required keys, id =
//    folder name, colours, enums, file names that exist). A theme that fails is
//    skipped with a warning — never half-applied.
//  - get() returns the theme fully resolved: defaults filled in, files and
//    ornament names turned into URLs (theme folder first, then the built-in
//    ornaments; <name>.png then .svg in Pixel, .svg then .png in Classic).
//    Classic uses the theme's classic wallpaper and backdrop when it has them. wallpaper carries
//    pixel (draw at a whole-number scale, unsmoothed) and sprites (animated
//    sheet layers in the image's pixels). Unknown ids (and old aliases such as "den-gradient") map to
//    the right theme; anything else falls back to "den".
// Guide: docs/THEMES.md. Go twin (phones, validation CLI): internal/themes.
#include <QHash>
#include <QJsonObject>
#include <QObject>
#include <QStringList>
#include <QUrl>
#include <QVariantList>
#include <QVariantMap>
#include <QtQml/qqmlregistration.h>

class QQmlEngine;
class QJSEngine;

class ThemeRegistry : public QObject {
    Q_OBJECT
    QML_NAMED_ELEMENT(Themes)
    QML_SINGLETON
    /// [{id, name, description, accent, builtIn}] in display order.
    Q_PROPERTY(QVariantList list READ list NOTIFY changed)

private:
    explicit ThemeRegistry(QObject *parent = nullptr);

public:
    static ThemeRegistry *create(QQmlEngine *, QJSEngine *);
    static ThemeRegistry *instance();

    QVariantList list() const;
    /// The resolved theme for a layout.ui.background value (see the header comment),
    /// in an art style: "pixel" (default) or "classic" (its classic art, SVG ornaments first).
    Q_INVOKABLE QVariantMap get(const QString &id, const QString &artStyle = QStringLiteral("pixel")) const;
    /// The canonical id for an id or alias ("den-gradient" → "den"); "den" when unknown.
    Q_INVOKABLE QString canonical(const QString &id) const;
    /// URL of an ornament for a theme: <theme>/<name>.png|svg, else the built-in; empty if none.
    /// Pixel looks for the PNG first, Classic for the SVG.
    Q_INVOKABLE QUrl ornament(const QString &themeId, const QString &name, const QString &artStyle = QStringLiteral("pixel")) const;
    /// Re-reads every theme folder (Settings → Theme picks up newly added themes).
    Q_INVOKABLE void reload();

    /// Loads from explicit roots, in order (later roots override earlier ids). Tests use it.
    void loadFrom(const QStringList &roots);
    /// Problems found by the last load, one line per skipped theme.
    QStringList problems() const { return m_problems; }

signals:
    void changed();

private:
    struct Source {
        QString dirPath; // ":/themes/den" or "/home/…/themes/den"
        QString dirUrl;  // "qrc:/themes/den" or "file:///home/…/themes/den"
        bool builtIn = false;
    };
    QVariantMap resolve(const QJsonObject &m, const Source &src, bool classic, QString *error) const;
    QString fileUrl(const Source &src, const QString &file) const;
    QString ornamentUrl(const Source &src, const QString &name, bool classic) const;
    QVariantMap decor(const QJsonObject &d, const Source &src, bool classic, QString *error) const;

    QHash<QString, QVariantMap> m_themes;        // pixel art style
    QHash<QString, QVariantMap> m_classicThemes; // classic art style
    QHash<QString, Source> m_sources;
    QHash<QString, QString> m_aliases;
    QStringList m_order;
    QStringList m_problems;
};
