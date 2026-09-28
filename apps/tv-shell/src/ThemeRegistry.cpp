// ThemeRegistry implementation: loads installed theme packages (contract in
// ThemeRegistry.h; spec contracts/theme.schema.json).

#include "ThemeRegistry.h"

#include <QColor>
#include <QDir>
#include <QFile>
#include <QImageReader>
#include <QJSEngine>
#include <QJsonArray>
#include <QJsonDocument>
#include <QRegularExpression>
#include <QStandardPaths>
#include <QtGlobal>

namespace {
ThemeRegistry *g_instance = nullptr;

// Built-in themes in display order; owner themes follow, sorted by id.
const QStringList kBuiltInOrder{QStringLiteral("den"), QStringLiteral("forest"), QStringLiteral("midnight"),
                                QStringLiteral("campfire"), QStringLiteral("winter")};
const QString kFallback = QStringLiteral("den");
const QString kBuiltInOrnaments = QStringLiteral(":/qt/qml/BearDen/assets/ornaments/");

const QRegularExpression kId(QStringLiteral("^[a-z][a-z0-9-]{1,31}$"));
const QRegularExpression kColor(QStringLiteral("^#[0-9A-Fa-f]{6}$"));
const QRegularExpression kFile(QStringLiteral("^[A-Za-z0-9][A-Za-z0-9._-]{0,63}\\.(svg|png|jpg|jpeg|webp)$"));
const QRegularExpression kOrnament(QStringLiteral("^[a-z][a-z0-9-]{0,31}$"));
const QStringList kStyles{QStringLiteral("none"), QStringLiteral("vine"), QStringLiteral("fern"), QStringLiteral("stars"), QStringLiteral("embers")};
const QStringList kParticles{QStringLiteral("none"), QStringLiteral("fireflies"), QStringLiteral("leaves"), QStringLiteral("stars"),
                             QStringLiteral("embers"), QStringLiteral("snow")};
const QStringList kScenes{QStringLiteral("none"), QStringLiteral("den"), QStringLiteral("camp"), QStringLiteral("moon"), QStringLiteral("campfire")};
const QStringList kChases{QStringLiteral("firefly"), QStringLiteral("leaf"), QStringLiteral("star"), QStringLiteral("ember")};
const QStringList kTopKeys{QStringLiteral("schema"), QStringLiteral("id"), QStringLiteral("name"), QStringLiteral("description"),
                           QStringLiteral("aliases"), QStringLiteral("accent"), QStringLiteral("wallpaper"), QStringLiteral("palette"),
                           QStringLiteral("focus"), QStringLiteral("panel"), QStringLiteral("ambient"), QStringLiteral("scene"),
                           QStringLiteral("bears"), QStringLiteral("heading"), QStringLiteral("phone"), QStringLiteral("classic")};
const QStringList kWallpaperKeys{QStringLiteral("image"), QStringLiteral("top"), QStringLiteral("bottom"), QStringLiteral("pixel"),
                                 QStringLiteral("sprites")};
const QStringList kSpriteKeys{QStringLiteral("sheet"), QStringLiteral("frames"), QStringLiteral("fps"), QStringLiteral("x"), QStringLiteral("y")};

// An integer member in [lo, hi]; -1 (and an error) when absent, fractional or out of range.
int intIn(const QJsonObject &o, const QString &key, int lo, int hi, const QString &what, QString *error)
{
    const QJsonValue v = o.value(key);
    const double d = v.toDouble(-1);
    if (!v.isDouble() || d != static_cast<int>(d) || d < lo || d > hi) {
        *error = QStringLiteral("%1 %2 must be a whole number %3–%4").arg(what, key).arg(lo).arg(hi);
        return -1;
    }
    return static_cast<int>(d);
}

QString color(const QJsonObject &o, const QString &key, const QString &fallback, QString *error)
{
    if (!o.contains(key))
        return fallback;
    const QString v = o.value(key).toString();
    if (!kColor.match(v).hasMatch()) {
        *error = QStringLiteral("%1 must be a #RRGGBB colour").arg(key);
        return fallback;
    }
    return v;
}

QString oneOf(const QJsonObject &o, const QString &key, const QStringList &allowed, const QString &fallback, QString *error)
{
    if (!o.contains(key))
        return fallback;
    const QString v = o.value(key).toString();
    if (!allowed.contains(v)) {
        *error = QStringLiteral("%1 must be one of %2").arg(key, allowed.join(QStringLiteral(", ")));
        return fallback;
    }
    return v;
}
} // namespace

ThemeRegistry::ThemeRegistry(QObject *parent) : QObject(parent)
{
    if (!g_instance)
        g_instance = this;
    reload();
}

ThemeRegistry *ThemeRegistry::create(QQmlEngine *, QJSEngine *)
{
    if (!g_instance)
        g_instance = new ThemeRegistry();
    QJSEngine::setObjectOwnership(g_instance, QJSEngine::CppOwnership);
    return g_instance;
}

ThemeRegistry *ThemeRegistry::instance()
{
    return g_instance;
}

void ThemeRegistry::reload()
{
    QString user = qEnvironmentVariable("BDTV_THEMES_DIR");
    if (user.isEmpty())
        user = QStandardPaths::writableLocation(QStandardPaths::GenericDataLocation) + QStringLiteral("/bear-den-tv/themes");
    loadFrom({QStringLiteral(":/themes"), user});
}

void ThemeRegistry::loadFrom(const QStringList &roots)
{
    m_themes.clear();
    m_classicThemes.clear();
    m_sources.clear();
    m_aliases.clear();
    m_problems.clear();
    QStringList others;
    for (const QString &root : roots) {
        const bool builtIn = root.startsWith(QLatin1Char(':'));
        const QDir dir(root);
        if (!dir.exists())
            continue;
        for (const QString &id : dir.entryList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
            Source src;
            src.dirPath = dir.filePath(id);
            src.dirUrl = builtIn ? QStringLiteral("qrc") + src.dirPath : QUrl::fromLocalFile(src.dirPath).toString();
            src.builtIn = builtIn;
            QFile f(src.dirPath + QStringLiteral("/theme.json"));
            if (!f.open(QIODevice::ReadOnly))
                continue;
            QJsonParseError perr;
            const QJsonDocument doc = QJsonDocument::fromJson(f.readAll(), &perr);
            QString error;
            QVariantMap theme;
            if (!doc.isObject())
                error = QStringLiteral("theme.json is not a JSON object (%1)").arg(perr.errorString());
            else if (doc.object().value(QStringLiteral("id")).toString() != id)
                error = QStringLiteral("id must equal the folder name \"%1\"").arg(id);
            else
                theme = resolve(doc.object(), src, false, &error);
            QVariantMap classicTheme;
            if (error.isEmpty())
                classicTheme = resolve(doc.object(), src, true, &error);
            if (!error.isEmpty()) {
                m_problems << QStringLiteral("%1: %2").arg(src.dirPath, error);
                qWarning("themes: skipping %s: %s", qPrintable(src.dirPath), qPrintable(error));
                continue;
            }
            if (m_themes.contains(id))
                qInfo("themes: %s overrides the built-in \"%s\"", qPrintable(src.dirPath), qPrintable(id));
            else if (!kBuiltInOrder.contains(id))
                others << id;
            m_themes.insert(id, theme);
            m_classicThemes.insert(id, classicTheme);
            m_sources.insert(id, src);
            for (const QVariant &alias : theme.value(QStringLiteral("aliases")).toList())
                m_aliases.insert(alias.toString(), id);
        }
    }
    m_order.clear();
    for (const QString &id : kBuiltInOrder)
        if (m_themes.contains(id))
            m_order << id;
    others.sort();
    for (const QString &id : others)
        if (!m_order.contains(id))
            m_order << id;
    emit changed();
}

QVariantList ThemeRegistry::list() const
{
    QVariantList out;
    for (const QString &id : m_order) {
        const QVariantMap &t = m_themes.value(id);
        out.append(QVariantMap{{QStringLiteral("id"), id},
                               {QStringLiteral("name"), t.value(QStringLiteral("name"))},
                               {QStringLiteral("description"), t.value(QStringLiteral("description"))},
                               {QStringLiteral("accent"), t.value(QStringLiteral("accent"))},
                               {QStringLiteral("builtIn"), t.value(QStringLiteral("builtIn"))}});
    }
    return out;
}

QString ThemeRegistry::canonical(const QString &id) const
{
    if (m_themes.contains(id))
        return id;
    if (m_aliases.contains(id))
        return m_aliases.value(id);
    return m_themes.contains(kFallback) ? kFallback : (m_order.isEmpty() ? QString() : m_order.first());
}

QVariantMap ThemeRegistry::get(const QString &id, const QString &artStyle) const
{
    return (artStyle == QLatin1String("classic") ? m_classicThemes : m_themes).value(canonical(id));
}

QUrl ThemeRegistry::ornament(const QString &themeId, const QString &name, const QString &artStyle) const
{
    const QString id = canonical(themeId);
    const bool classic = artStyle == QLatin1String("classic");
    if (m_sources.contains(id))
        return QUrl(ornamentUrl(m_sources.value(id), name, classic));
    return QUrl(ornamentUrl(Source{}, name, classic));
}

QString ThemeRegistry::fileUrl(const Source &src, const QString &file) const
{
    return src.dirUrl + QLatin1Char('/') + file;
}

QString ThemeRegistry::ornamentUrl(const Source &src, const QString &name, bool classic) const
{
    if (name.isEmpty() || !kOrnament.match(name).hasMatch())
        return {};
    // The art style picks which kind comes first: the pixel PNG or the Classic SVG.
    const QStringList exts = classic ? QStringList{QStringLiteral(".svg"), QStringLiteral(".png")}
                                     : QStringList{QStringLiteral(".png"), QStringLiteral(".svg")};
    if (!src.dirPath.isEmpty()) {
        for (const QString &ext : exts)
            if (QFile::exists(src.dirPath + QLatin1Char('/') + name + ext))
                return fileUrl(src, name + ext);
    }
    for (const QString &ext : exts)
        if (QFile::exists(kBuiltInOrnaments + name + ext))
            return QStringLiteral("qrc") + kBuiltInOrnaments + name + ext;
    return {};
}

QVariantMap ThemeRegistry::decor(const QJsonObject &d, const Source &src, bool classic, QString *error) const
{
    const QString style = oneOf(d, QStringLiteral("style"), kStyles, QStringLiteral("none"), error);
    const QString tip = d.value(QStringLiteral("tip")).toString();
    auto urls = [&](const QString &key) {
        QVariantList out;
        for (const QJsonValue &v : d.value(key).toArray()) {
            const QString url = ornamentUrl(src, v.toString(), classic);
            if (url.isEmpty())
                *error = QStringLiteral("unknown ornament \"%1\" in %2").arg(v.toString(), key);
            else
                out << url;
        }
        return out;
    };
    const QVariantList extras = urls(QStringLiteral("extras"));
    const QVariantList bottom = d.contains(QStringLiteral("extras_bottom")) ? urls(QStringLiteral("extras_bottom")) : extras;
    QString tipUrl;
    if (!tip.isEmpty()) {
        tipUrl = ornamentUrl(src, tip, classic);
        if (tipUrl.isEmpty())
            *error = QStringLiteral("unknown tip ornament \"%1\"").arg(tip);
    }
    return {{QStringLiteral("style"), style == QLatin1String("none") ? QString() : style},
            {QStringLiteral("tip"), tipUrl},
            {QStringLiteral("tipUpright"), d.value(QStringLiteral("tip_upright")).toBool()},
            {QStringLiteral("extras"), extras},
            {QStringLiteral("extrasBottom"), bottom},
            {QStringLiteral("extrasUpright"), d.value(QStringLiteral("extras_upright")).toBool()}};
}

QVariantMap ThemeRegistry::resolve(const QJsonObject &m, const Source &src, bool classic, QString *error) const
{
    for (auto it = m.begin(); it != m.end(); ++it)
        if (!kTopKeys.contains(it.key())) {
            *error = QStringLiteral("unknown key \"%1\"").arg(it.key());
            return {};
        }
    if (m.value(QStringLiteral("schema")).toInt() != 1) {
        *error = QStringLiteral("schema must be 1");
        return {};
    }
    const QString id = m.value(QStringLiteral("id")).toString();
    const QString name = m.value(QStringLiteral("name")).toString();
    if (!kId.match(id).hasMatch() || name.isEmpty()) {
        *error = QStringLiteral("id and name are required");
        return {};
    }
    const QString accent = color(m, QStringLiteral("accent"), QString(), error);
    if (accent.isEmpty() && error->isEmpty())
        *error = QStringLiteral("accent is required");
    const QJsonObject wp = m.value(QStringLiteral("wallpaper")).toObject();
    if (!m.contains(QStringLiteral("wallpaper")))
        *error = QStringLiteral("wallpaper is required");
    QString image;
    if (wp.contains(QStringLiteral("image"))) {
        const QString file = wp.value(QStringLiteral("image")).toString();
        if (!kFile.match(file).hasMatch() || !QFile::exists(src.dirPath + QLatin1Char('/') + file))
            *error = QStringLiteral("wallpaper image \"%1\" is missing or badly named").arg(file);
        else
            image = fileUrl(src, file);
    }
    for (auto it = wp.begin(); it != wp.end(); ++it)
        if (!kWallpaperKeys.contains(it.key()))
            *error = QStringLiteral("unknown wallpaper key \"%1\"").arg(it.key());
    if (wp.contains(QStringLiteral("pixel")) && !wp.value(QStringLiteral("pixel")).isBool())
        *error = QStringLiteral("wallpaper pixel must be true or false");
    auto spritesOf = [&](const QJsonObject &owner, const QString &where) {
        const QJsonArray spriteList = owner.value(QStringLiteral("sprites")).toArray();
        if (owner.contains(QStringLiteral("sprites")) && !owner.value(QStringLiteral("sprites")).isArray())
            *error = QStringLiteral("%1 sprites must be a list").arg(where);
        if (spriteList.size() > 16)
            *error = QStringLiteral("%1 sprites: at most 16").arg(where);
        QVariantList out;
        for (const QJsonValue &v : spriteList) {
            const QJsonObject sp = v.toObject();
            for (auto it = sp.begin(); it != sp.end(); ++it)
                if (!kSpriteKeys.contains(it.key()))
                    *error = QStringLiteral("unknown sprite key \"%1\"").arg(it.key());
            const QString sheet = sp.value(QStringLiteral("sheet")).toString();
            QString sheetUrl;
            if (!kFile.match(sheet).hasMatch() || !QFile::exists(src.dirPath + QLatin1Char('/') + sheet))
                *error = QStringLiteral("sprite sheet \"%1\" is missing or badly named").arg(sheet);
            else
                sheetUrl = fileUrl(src, sheet);
            const QString what = QStringLiteral("sprite");
            out << QVariantMap{{QStringLiteral("sheet"), sheetUrl},
                               {QStringLiteral("frames"), intIn(sp, QStringLiteral("frames"), 1, 32, what, error)},
                               {QStringLiteral("fps"), intIn(sp, QStringLiteral("fps"), 1, 20, what, error)},
                               {QStringLiteral("x"), intIn(sp, QStringLiteral("x"), 0, 3840, what, error)},
                               {QStringLiteral("y"), intIn(sp, QStringLiteral("y"), 0, 3840, what, error)}};
        }
        return out;
    };
    auto existingFile = [&](const QJsonObject &owner, const QString &key, const QString &what) {
        const QString file = owner.value(key).toString();
        if (!kFile.match(file).hasMatch() || !QFile::exists(src.dirPath + QLatin1Char('/') + file)) {
            *error = QStringLiteral("%1 \"%2\" is missing or badly named").arg(what, file);
            return QString();
        }
        return fileUrl(src, file);
    };
    QVariantList sprites = spritesOf(wp, QStringLiteral("wallpaper"));
    bool pixel = wp.value(QStringLiteral("pixel")).toBool();

    // Classic art (optional): its own wallpaper and phone backdrop. Checked in
    // both art styles, so a bad block fails the theme either way; used in Classic.
    const QJsonObject cl = m.value(QStringLiteral("classic")).toObject();
    if (m.contains(QStringLiteral("classic")) && !m.value(QStringLiteral("classic")).isObject())
        *error = QStringLiteral("classic must be an object");
    for (auto it = cl.begin(); it != cl.end(); ++it)
        if (it.key() != QLatin1String("wallpaper") && it.key() != QLatin1String("phone"))
            *error = QStringLiteral("unknown classic key \"%1\"").arg(it.key());
    QString classicImage, classicBackdrop;
    QVariantList classicSprites;
    if (cl.contains(QStringLiteral("wallpaper"))) {
        const QJsonObject cw = cl.value(QStringLiteral("wallpaper")).toObject();
        for (auto it = cw.begin(); it != cw.end(); ++it)
            if (it.key() != QLatin1String("image") && it.key() != QLatin1String("sprites"))
                *error = QStringLiteral("unknown classic wallpaper key \"%1\"").arg(it.key());
        classicImage = existingFile(cw, QStringLiteral("image"), QStringLiteral("classic wallpaper image"));
        classicSprites = spritesOf(cw, QStringLiteral("classic wallpaper"));
    }
    if (cl.contains(QStringLiteral("phone"))) {
        const QJsonObject cp = cl.value(QStringLiteral("phone")).toObject();
        for (auto it = cp.begin(); it != cp.end(); ++it)
            if (it.key() != QLatin1String("backdrop"))
                *error = QStringLiteral("unknown classic phone key \"%1\"").arg(it.key());
        classicBackdrop = existingFile(cp, QStringLiteral("backdrop"), QStringLiteral("classic phone backdrop"));
    }
    if (classic && !classicImage.isEmpty()) {
        image = classicImage;
        sprites = classicSprites;
        pixel = false;
    }
    // The picture's own size (read from its header), so animated layers placed
    // in its pixels line up however it is scaled and cropped on screen.
    QSize imageSize;
    if (!image.isEmpty()) {
        const QString path = image.startsWith(QLatin1String("qrc:")) ? image.mid(3) : QUrl(image).toLocalFile();
        imageSize = QImageReader(path).size();
    }
    const QVariantMap wallpaper{{QStringLiteral("image"), image},
                                {QStringLiteral("width"), imageSize.width()},
                                {QStringLiteral("height"), imageSize.height()},
                                {QStringLiteral("top"), color(wp, QStringLiteral("top"), QStringLiteral("#1F2A26"), error)},
                                {QStringLiteral("bottom"), color(wp, QStringLiteral("bottom"), QStringLiteral("#0C0E10"), error)},
                                {QStringLiteral("pixel"), pixel},
                                {QStringLiteral("sprites"), sprites}};

    const QJsonObject pal = m.value(QStringLiteral("palette")).toObject();
    const QColor a(accent.isEmpty() ? QStringLiteral("#79A889") : accent);
    const QVariantMap palette{{QStringLiteral("stem"), color(pal, QStringLiteral("stem"), a.darker(135).name(), error)},
                              {QStringLiteral("light"), color(pal, QStringLiteral("light"), a.lighter(135).name(), error)},
                              {QStringLiteral("dark"), color(pal, QStringLiteral("dark"), a.darker(110).name(), error)},
                              {QStringLiteral("bloom"), color(pal, QStringLiteral("bloom"), QStringLiteral("#F7F0E2"), error)},
                              {QStringLiteral("glow"), color(pal, QStringLiteral("glow"), QStringLiteral("#E3B35C"), error)}};

    const QVariantMap focus = decor(m.value(QStringLiteral("focus")).toObject(), src, classic, error);
    const QVariantMap panel = m.contains(QStringLiteral("panel")) ? decor(m.value(QStringLiteral("panel")).toObject(), src, classic, error) : focus;

    const QJsonObject amb = m.value(QStringLiteral("ambient")).toObject();
    QVariantList ambientColors;
    for (const QJsonValue &v : amb.value(QStringLiteral("colors")).toArray()) {
        if (!kColor.match(v.toString()).hasMatch())
            *error = QStringLiteral("ambient colours must be #RRGGBB");
        ambientColors << v.toString();
    }
    const QString ambientKind = oneOf(amb, QStringLiteral("kind"), kParticles, QStringLiteral("none"), error);
    const int count = amb.contains(QStringLiteral("count")) ? amb.value(QStringLiteral("count")).toInt(-1) : 14;
    if (count < 0 || count > 40)
        *error = QStringLiteral("ambient count must be 0–40");

    const QJsonObject bears = m.value(QStringLiteral("bears")).toObject();
    const QString carry = bears.value(QStringLiteral("carry")).toString();
    const QString chase = bears.value(QStringLiteral("chase")).toString();
    auto optionalOrnament = [&](const QString &n, const QString &what) {
        if (n.isEmpty())
            return QString();
        const QString url = ornamentUrl(src, n, classic);
        if (url.isEmpty())
            *error = QStringLiteral("unknown %1 ornament \"%2\"").arg(what, n);
        return url;
    };
    const QVariantMap bearMap{
        {QStringLiteral("hat"), optionalOrnament(bears.value(QStringLiteral("hat")).toString(), QStringLiteral("hat"))},
        {QStringLiteral("carry"), carry == QLatin1String("stick") ? carry : optionalOrnament(carry, QStringLiteral("carry"))},
        {QStringLiteral("chase"), chase.isEmpty() || kChases.contains(chase) ? chase : optionalOrnament(chase, QStringLiteral("chase"))}};

    const QJsonObject phone = m.value(QStringLiteral("phone")).toObject();
    QString backdrop;
    if (phone.contains(QStringLiteral("backdrop"))) {
        const QString file = phone.value(QStringLiteral("backdrop")).toString();
        if (!kFile.match(file).hasMatch() || !QFile::exists(src.dirPath + QLatin1Char('/') + file))
            *error = QStringLiteral("phone backdrop \"%1\" is missing or badly named").arg(file);
        else
            backdrop = fileUrl(src, file);
    }
    if (classic && !classicBackdrop.isEmpty())
        backdrop = classicBackdrop;
    else if (classic && !classicImage.isEmpty())
        backdrop.clear(); // the classic wallpaper (image, above) stands in
    const QVariantMap phoneMap{{QStringLiteral("backdrop"), backdrop.isEmpty() ? image : backdrop},
                               {QStringLiteral("veil"), color(phone, QStringLiteral("veil"), wallpaper.value(QStringLiteral("bottom")).toString(), error)},
                               {QStringLiteral("particles"), oneOf(phone, QStringLiteral("particles"), kParticles, ambientKind, error)}};

    QVariantList aliases;
    for (const QJsonValue &v : m.value(QStringLiteral("aliases")).toArray())
        aliases << v.toString();

    return {{QStringLiteral("id"), id},
            {QStringLiteral("name"), name},
            {QStringLiteral("description"), m.value(QStringLiteral("description")).toString()},
            {QStringLiteral("aliases"), aliases},
            {QStringLiteral("accent"), accent},
            {QStringLiteral("builtIn"), src.builtIn},
            {QStringLiteral("wallpaper"), wallpaper},
            {QStringLiteral("palette"), palette},
            {QStringLiteral("focus"), focus},
            {QStringLiteral("panel"), panel},
            {QStringLiteral("ambient"), QVariantMap{{QStringLiteral("kind"), ambientKind == QLatin1String("none") ? QString() : ambientKind},
                                                    {QStringLiteral("count"), count},
                                                    {QStringLiteral("colors"), ambientColors}}},
            {QStringLiteral("scene"), oneOf(m, QStringLiteral("scene"), kScenes, QStringLiteral("none"), error) == QLatin1String("none")
                                          ? QString() : m.value(QStringLiteral("scene")).toString()},
            {QStringLiteral("bears"), bearMap},
            {QStringLiteral("heading"), optionalOrnament(m.value(QStringLiteral("heading")).toString(), QStringLiteral("heading"))},
            {QStringLiteral("phone"), phoneMap}};
}
