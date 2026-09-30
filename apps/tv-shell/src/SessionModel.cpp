// SessionModel implementation: validates and applies coordinator state
// snapshots (contract in SessionModel.h; spec contracts/state.schema.json).

#include "SessionModel.h"

#include "Theme.h"

#include <QFile>
#include <cmath>
#include <QUrl>
#include <QJSEngine>
#include <QJsonArray>
#include <QRegularExpression>
#include <QJsonDocument>
#include <QQmlEngine>

namespace {
SessionModel *g_instance = nullptr;

bool requireKeys(const QJsonObject &obj, const QStringList &keys, const QString &where, QString *error)
{
    for (const QString &key : keys) {
        if (!obj.contains(key)) {
            if (error)
                *error = QStringLiteral("%1: missing required member '%2'").arg(where, key);
            return false;
        }
    }
    return true;
}

bool requireEnum(const QJsonObject &obj, const QString &key, const QStringList &allowed, const QString &where, QString *error)
{
    const QString value = obj.value(key).toString();
    if (!allowed.contains(value)) {
        if (error)
            *error = QStringLiteral("%1.%2: '%3' is not one of [%4]").arg(where, key, value, allowed.join(QLatin1String(", ")));
        return false;
    }
    return true;
}

bool requireType(const QJsonObject &obj, const QString &key, QJsonValue::Type type, const QString &where, QString *error)
{
    if (obj.value(key).type() != type) {
        if (error)
            *error = QStringLiteral("%1.%2: wrong JSON type").arg(where, key);
        return false;
    }
    return true;
}
} // namespace

SessionModel::SessionModel(QObject *parent) : QObject(parent), m_sections(new SectionsModel(this))
{
    QQmlEngine::setObjectOwnership(m_sections, QQmlEngine::CppOwnership);
    if (!g_instance)
        g_instance = this;
}

SessionModel *SessionModel::create(QQmlEngine *, QJSEngine *)
{
    if (!g_instance)
        g_instance = new SessionModel();
    QJSEngine::setObjectOwnership(g_instance, QJSEngine::CppOwnership);
    return g_instance;
}

SessionModel *SessionModel::instance()
{
    return g_instance;
}

bool SessionModel::validateLayout(const QJsonObject &layout, QString *error)
{
    if (!requireKeys(layout, {QStringLiteral("ui"), QStringLiteral("sections")}, QStringLiteral("layout"), error))
        return false;
    const QJsonObject ui = layout.value(QStringLiteral("ui")).toObject();
    if (!requireKeys(ui, {QStringLiteral("theme"), QStringLiteral("accent"), QStringLiteral("background"), QStringLiteral("text_scale"),
                          QStringLiteral("tile_density"), QStringLiteral("safe_margin_percent"), QStringLiteral("reduced_motion"),
                          QStringLiteral("high_contrast_focus"), QStringLiteral("hero_enabled"), QStringLiteral("clock_enabled")},
                     QStringLiteral("layout.ui"), error))
        return false;
    if (!requireEnum(ui, QStringLiteral("theme"), {QStringLiteral("den-dark"), QStringLiteral("plain-dark"), QStringLiteral("performance")}, QStringLiteral("layout.ui"), error))
        return false;
    // A theme id (contracts/theme.schema.json); unknown ids fall back to Den in ThemeRegistry.
    static const QRegularExpression themeId(QStringLiteral("^[a-z][a-z0-9-]{1,31}$"));
    if (!themeId.match(ui.value(QStringLiteral("background")).toString()).hasMatch()) {
        if (error)
            *error = QStringLiteral("layout.ui.background: not a theme id");
        return false;
    }
    if (!requireEnum(ui, QStringLiteral("tile_density"), {QStringLiteral("comfortable"), QStringLiteral("large")}, QStringLiteral("layout.ui"), error))
        return false;
    // Optional (missing means pixel).
    if (ui.contains(QStringLiteral("art_style"))
        && !requireEnum(ui, QStringLiteral("art_style"), {QStringLiteral("pixel"), QStringLiteral("classic")}, QStringLiteral("layout.ui"), error))
        return false;
    // Optional (missing means app: the app's own icon).
    if (ui.contains(QStringLiteral("app_icons"))
        && !requireEnum(ui, QStringLiteral("app_icons"), {QStringLiteral("app"), QStringLiteral("bear_den")}, QStringLiteral("layout.ui"), error))
        return false;
    const double textScale = ui.value(QStringLiteral("text_scale")).toDouble(-1);
    if (textScale < 0.8 || textScale > 2.0) {
        if (error)
            *error = QStringLiteral("layout.ui.text_scale out of range");
        return false;
    }
    const double margin = ui.value(QStringLiteral("safe_margin_percent")).toDouble(-1);
    if (margin < 0 || margin > 10) {
        if (error)
            *error = QStringLiteral("layout.ui.safe_margin_percent out of range");
        return false;
    }
    const QJsonArray sections = layout.value(QStringLiteral("sections")).toArray();
    QSet<QString> ids;
    for (const QJsonValue &v : sections) {
        const QJsonObject s = v.toObject();
        if (!requireKeys(s, {QStringLiteral("id"), QStringLiteral("title"), QStringLiteral("kind"), QStringLiteral("enabled"), QStringLiteral("hide_when_empty")},
                         QStringLiteral("layout.sections[]"), error))
            return false;
        if (!requireEnum(s, QStringLiteral("kind"), {QStringLiteral("applications"), QStringLiteral("plex-continue-watching"), QStringLiteral("plex-recently-added"), QStringLiteral("plex-collection")},
                         QStringLiteral("layout.sections[]"), error))
            return false;
        const bool isApps = s.value(QStringLiteral("kind")).toString() == QLatin1String("applications");
        if (isApps != s.contains(QStringLiteral("application_ids"))) {
            if (error)
                *error = QStringLiteral("layout.sections[%1]: application_ids only allowed for kind applications").arg(s.value(QStringLiteral("id")).toString());
            return false;
        }
        const QString id = s.value(QStringLiteral("id")).toString();
        if (ids.contains(id)) {
            if (error)
                *error = QStringLiteral("layout.sections: duplicate id '%1'").arg(id);
            return false;
        }
        ids.insert(id);
    }
    return true;
}

bool SessionModel::validateSnapshot(const QJsonObject &snapshot, QString *error)
{
    if (!requireKeys(snapshot, {QStringLiteral("protocol"), QStringLiteral("context_epoch"), QStringLiteral("generated_at_ms"), QStringLiteral("device_name"),
                                QStringLiteral("dev_mode"), QStringLiteral("config_revision"), QStringLiteral("session"), QStringLiteral("target"),
                                QStringLiteral("capabilities"), QStringLiteral("shell"), QStringLiteral("applications"), QStringLiteral("remote"),
                                QStringLiteral("notifications")},
                     QStringLiteral("state"), error))
        return false;
    if (snapshot.value(QStringLiteral("protocol")).toInt() != 1) {
        if (error)
            *error = QStringLiteral("state.protocol must be 1");
        return false;
    }
    const QJsonObject session = snapshot.value(QStringLiteral("session")).toObject();
    if (!requireKeys(session, {QStringLiteral("locked"), QStringLiteral("display_session"), QStringLiteral("desktop_adapter"), QStringLiteral("shell_connected"), QStringLiteral("shell_state")},
                     QStringLiteral("state.session"), error))
        return false;
    if (!requireEnum(session, QStringLiteral("display_session"), {QStringLiteral("x11"), QStringLiteral("wayland"), QStringLiteral("unknown")}, QStringLiteral("state.session"), error))
        return false;
    if (!requireEnum(session, QStringLiteral("shell_state"), {QStringLiteral("stopped"), QStringLiteral("starting"), QStringLiteral("running"), QStringLiteral("crashed"),
                                                              QStringLiteral("restarting"), QStringLiteral("circuit_open"), QStringLiteral("exited")},
                     QStringLiteral("state.session"), error))
        return false;
    const QJsonObject target = snapshot.value(QStringLiteral("target")).toObject();
    if (!requireKeys(target, {QStringLiteral("kind"), QStringLiteral("app_id"), QStringLiteral("label"), QStringLiteral("observed")}, QStringLiteral("state.target"), error))
        return false;
    if (!requireEnum(target, QStringLiteral("kind"), {QStringLiteral("shell"), QStringLiteral("app"), QStringLiteral("unknown"), QStringLiteral("locked"), QStringLiteral("none")},
                     QStringLiteral("state.target"), error))
        return false;
    const QJsonObject capabilities = snapshot.value(QStringLiteral("capabilities")).toObject();
    for (auto it = capabilities.constBegin(); it != capabilities.constEnd(); ++it) {
        if (!it.value().isObject() || !it.value().toObject().contains(QStringLiteral("available"))) {
            if (error)
                *error = QStringLiteral("state.capabilities.%1: missing 'available'").arg(it.key());
            return false;
        }
    }
    const QJsonObject shell = snapshot.value(QStringLiteral("shell")).toObject();
    if (!requireKeys(shell, {QStringLiteral("screen"), QStringLiteral("focus")}, QStringLiteral("state.shell"), error))
        return false;
    if (!requireEnum(shell, QStringLiteral("screen"), {QStringLiteral("home"), QStringLiteral("settings"), QStringLiteral("pairing"), QStringLiteral("devices"), QStringLiteral("diagnostics"),
                                                       QStringLiteral("dialog"), QStringLiteral("setup"), QStringLiteral("error"), QStringLiteral("unknown")},
                     QStringLiteral("state.shell"), error))
        return false;
    if (!requireKeys(shell.value(QStringLiteral("focus")).toObject(), {QStringLiteral("section_id"), QStringLiteral("item_id")}, QStringLiteral("state.shell.focus"), error))
        return false;
    if (!requireType(snapshot, QStringLiteral("applications"), QJsonValue::Array, QStringLiteral("state"), error))
        return false;
    for (const QJsonValue &v : snapshot.value(QStringLiteral("applications")).toArray()) {
        const QJsonObject app = v.toObject();
        if (!requireKeys(app, {QStringLiteral("id"), QStringLiteral("label"), QStringLiteral("adapter"), QStringLiteral("installed"), QStringLiteral("version"), QStringLiteral("installation"),
                               QStringLiteral("running"), QStringLiteral("foreground"), QStringLiteral("launch_state"), QStringLiteral("last_error")},
                         QStringLiteral("state.applications[]"), error))
            return false;
        if (!requireEnum(app, QStringLiteral("launch_state"), {QStringLiteral("idle"), QStringLiteral("launching"), QStringLiteral("running"), QStringLiteral("failed"), QStringLiteral("exited"), QStringLiteral("crashed")},
                         QStringLiteral("state.applications[]"), error))
            return false;
        // Optional: an optional app that is not installed (no tile).
        if (app.contains(QStringLiteral("hidden")) && !app.value(QStringLiteral("hidden")).isBool()) {
            if (error)
                *error = QStringLiteral("state.applications[].hidden must be a boolean");
            return false;
        }
        // Optional: a web app the owner can turn on and off (Apps → Streaming sites).
        if (app.contains(QStringLiteral("enabled")) && !app.value(QStringLiteral("enabled")).isBool()) {
            if (error)
                *error = QStringLiteral("state.applications[].enabled must be a boolean");
            return false;
        }
        // Optional: what the owner should know about the app, as short sentences.
        if (app.contains(QStringLiteral("notes"))) {
            const QJsonValue notes = app.value(QStringLiteral("notes"));
            bool ok = notes.isArray();
            for (const QJsonValue &n : notes.toArray())
                ok = ok && n.isString();
            if (!ok) {
                if (error)
                    *error = QStringLiteral("state.applications[].notes must be an array of strings");
                return false;
            }
        }
        // Optional: the app's install from Flathub (state.schema.json#/$defs/install).
        if (app.contains(QStringLiteral("install"))) {
            const QString where = QStringLiteral("state.applications[].install");
            if (!requireType(app, QStringLiteral("install"), QJsonValue::Object, QStringLiteral("state.applications[]"), error))
                return false;
            const QJsonObject inst = app.value(QStringLiteral("install")).toObject();
            if (!requireKeys(inst, {QStringLiteral("state"), QStringLiteral("progress"), QStringLiteral("phase")}, where, error)
                || !requireEnum(inst, QStringLiteral("state"), {QStringLiteral("none"), QStringLiteral("available"), QStringLiteral("preparing"), QStringLiteral("downloading"),
                                                                QStringLiteral("installing"), QStringLiteral("failed"), QStringLiteral("done")}, where, error)
                || !requireEnum(inst, QStringLiteral("phase"), {QString(), QStringLiteral("checking"), QStringLiteral("runtime"), QStringLiteral("app"), QStringLiteral("finishing")}, where, error))
                return false;
            const QJsonValue progress = inst.value(QStringLiteral("progress"));
            if (!progress.isDouble() || progress.toDouble() < 0 || progress.toDouble() > 100 || progress.toDouble() != progress.toInt()) {
                if (error)
                    *error = QStringLiteral("%1.progress must be an integer 0..100").arg(where);
                return false;
            }
            if (inst.contains(QStringLiteral("drm"))
                && !requireEnum(inst, QStringLiteral("drm"), {QStringLiteral("ready"), QStringLiteral("preparing"), QStringLiteral("pending")}, where, error))
                return false;
        }
    }
    if (snapshot.contains(QStringLiteral("apps"))) {
        // state.apps (optional, shell and owner phones): install settings and
        // the web apps' browsers (browser, streaming_browser, browsers[]).
        if (!requireType(snapshot, QStringLiteral("apps"), QJsonValue::Object, QStringLiteral("state"), error)
            || !requireKeys(snapshot.value(QStringLiteral("apps")).toObject(), {QStringLiteral("auto_update")}, QStringLiteral("state.apps"), error)
            || !requireType(snapshot.value(QStringLiteral("apps")).toObject(), QStringLiteral("auto_update"), QJsonValue::Bool, QStringLiteral("state.apps"), error))
            return false;
        const QJsonObject apps = snapshot.value(QStringLiteral("apps")).toObject();
        for (const QString &key : {QStringLiteral("browser"), QStringLiteral("streaming_browser")}) {
            if (apps.contains(key) && !requireType(apps, key, QJsonValue::String, QStringLiteral("state.apps"), error))
                return false;
        }
        if (apps.contains(QStringLiteral("browsers"))) {
            if (!requireType(apps, QStringLiteral("browsers"), QJsonValue::Array, QStringLiteral("state.apps"), error))
                return false;
            for (const QJsonValue &b : apps.value(QStringLiteral("browsers")).toArray()) {
                const QJsonObject browser = b.toObject();
                const QString where = QStringLiteral("state.apps.browsers[]");
                if (!requireKeys(browser, {QStringLiteral("id"), QStringLiteral("label"), QStringLiteral("flatpak_id"), QStringLiteral("streaming_unverified")}, where, error)
                    || !requireType(browser, QStringLiteral("streaming_unverified"), QJsonValue::Bool, where, error))
                    return false;
                // notes (optional): plain words shown beside the choice.
                if (browser.contains(QStringLiteral("notes"))) {
                    if (!requireType(browser, QStringLiteral("notes"), QJsonValue::Array, where, error))
                        return false;
                    for (const QJsonValue &n : browser.value(QStringLiteral("notes")).toArray()) {
                        if (!n.isString()) {
                            if (error)
                                *error = where + QStringLiteral(".notes[] must be a string");
                            return false;
                        }
                    }
                }
            }
        }
    }
    const QJsonObject remote = snapshot.value(QStringLiteral("remote")).toObject();
    if (!requireKeys(remote, {QStringLiteral("enabled"), QStringLiteral("transport"), QStringLiteral("listening"), QStringLiteral("addresses"), QStringLiteral("https"),
                              QStringLiteral("http_layout_editing"), QStringLiteral("paired_device_count"), QStringLiteral("hold"), QStringLiteral("limits")},
                     QStringLiteral("state.remote"), error))
        return false;
    if (!requireEnum(remote, QStringLiteral("transport"), {QStringLiteral("local-only"), QStringLiteral("trusted-lan-http"), QStringLiteral("https")}, QStringLiteral("state.remote"), error))
        return false;
    if (snapshot.contains(QStringLiteral("pairing"))) {
        const QJsonObject pairing = snapshot.value(QStringLiteral("pairing")).toObject();
        if (!requireKeys(pairing, {QStringLiteral("active"), QStringLiteral("code"), QStringLiteral("url"), QStringLiteral("qr_modules"), QStringLiteral("expires_in_s"), QStringLiteral("attempts_left")},
                         QStringLiteral("state.pairing"), error))
            return false;
        if (pairing.value(QStringLiteral("active")).toBool()) {
            const QString code = pairing.value(QStringLiteral("code")).toString();
            if (code.size() != 6 || !std::all_of(code.cbegin(), code.cend(), [](QChar c) { return c.isDigit(); })) {
                if (error)
                    *error = QStringLiteral("state.pairing.code must be six digits while active");
                return false;
            }
        }
    }
    for (const QJsonValue &v : snapshot.value(QStringLiteral("devices")).toArray()) {
        if (!requireKeys(v.toObject(), {QStringLiteral("id"), QStringLiteral("name"), QStringLiteral("permissions"), QStringLiteral("connected"), QStringLiteral("last_seen_ms"), QStringLiteral("created_at")},
                         QStringLiteral("state.devices[]"), error))
            return false;
        // Permissions are a closed set; a guest pass never comes with another
        // permission (state.schema.json devices[].permissions).
        const QJsonArray perms = v.toObject().value(QStringLiteral("permissions")).toArray();
        bool guest = false;
        for (const QJsonValue &p : perms) {
            const QString name = p.toString();
            if (name != QLatin1String("controller") && name != QLatin1String("layout_editor") && name != QLatin1String("owner") && name != QLatin1String("guest")) {
                if (error)
                    *error = QStringLiteral("state.devices[].permissions: unknown permission '%1'").arg(name);
                return false;
            }
            guest = guest || name == QLatin1String("guest");
        }
        if (guest && perms.size() != 1) {
            if (error)
                *error = QStringLiteral("state.devices[].permissions: guest never comes with another permission");
            return false;
        }
        const QJsonValue expires = v.toObject().value(QStringLiteral("expires_at_ms"));
        if (!expires.isUndefined() && !expires.isNull() && !expires.isDouble()) {
            if (error)
                *error = QStringLiteral("state.devices[].expires_at_ms must be a number or null");
            return false;
        }
    }
    if (!requireType(snapshot, QStringLiteral("notifications"), QJsonValue::Array, QStringLiteral("state"), error))
        return false;
    for (const QJsonValue &v : snapshot.value(QStringLiteral("notifications")).toArray()) {
        const QJsonObject n = v.toObject();
        if (!requireKeys(n, {QStringLiteral("id"), QStringLiteral("kind"), QStringLiteral("text"), QStringLiteral("created_ms")}, QStringLiteral("state.notifications[]"), error))
            return false;
        if (!requireEnum(n, QStringLiteral("kind"), {QStringLiteral("info"), QStringLiteral("success"), QStringLiteral("warning"), QStringLiteral("error")}, QStringLiteral("state.notifications[]"), error))
            return false;
    }
    if (snapshot.contains(QStringLiteral("layout")) && !validateLayout(snapshot.value(QStringLiteral("layout")).toObject(), error))
        return false;
    if (snapshot.contains(QStringLiteral("content"))) {
        const QJsonObject content = snapshot.value(QStringLiteral("content")).toObject();
        if (!requireKeys(content, {QStringLiteral("provider"), QStringLiteral("status"), QStringLiteral("message"), QStringLiteral("sections")}, QStringLiteral("state.content"), error))
            return false;
        if (!requireEnum(content, QStringLiteral("status"), {QStringLiteral("disabled"), QStringLiteral("connecting"), QStringLiteral("ready"), QStringLiteral("stale"), QStringLiteral("error")},
                         QStringLiteral("state.content"), error))
            return false;
        for (const QJsonValue &sv : content.value(QStringLiteral("sections")).toArray()) {
            const QJsonObject s = sv.toObject();
            if (!requireKeys(s, {QStringLiteral("section_id"), QStringLiteral("items")}, QStringLiteral("state.content.sections[]"), error))
                return false;
            for (const QJsonValue &iv : s.value(QStringLiteral("items")).toArray()) {
                const QJsonObject item = iv.toObject();
                if (!requireKeys(item, {QStringLiteral("id"), QStringLiteral("title"), QStringLiteral("subtitle"), QStringLiteral("artwork"), QStringLiteral("progress"), QStringLiteral("open_action"), QStringLiteral("demo")},
                                 QStringLiteral("state.content.sections[].items[]"), error))
                    return false;
                if (!requireEnum(item, QStringLiteral("open_action"), {QStringLiteral("open_app"), QStringLiteral("play_exact")}, QStringLiteral("state.content.sections[].items[]"), error))
                    return false;
            }
        }
    }
    if (snapshot.contains(QStringLiteral("weather"))) {
        // state.weather (optional, shell view only): status/message/place/units/scene/current.
        const QString where = QStringLiteral("state.weather");
        if (!requireType(snapshot, QStringLiteral("weather"), QJsonValue::Object, QStringLiteral("state"), error))
            return false;
        const QJsonObject weather = snapshot.value(QStringLiteral("weather")).toObject();
        if (!requireKeys(weather, {QStringLiteral("status"), QStringLiteral("message"), QStringLiteral("place"), QStringLiteral("units"), QStringLiteral("scene"), QStringLiteral("current")},
                         where, error))
            return false;
        if (!requireEnum(weather, QStringLiteral("status"), {QStringLiteral("disabled"), QStringLiteral("connecting"), QStringLiteral("ready"), QStringLiteral("stale"), QStringLiteral("error")},
                         where, error)
            || !requireEnum(weather, QStringLiteral("units"), {QStringLiteral("celsius"), QStringLiteral("fahrenheit")}, where, error)
            || !requireType(weather, QStringLiteral("message"), QJsonValue::String, where, error)
            || !requireType(weather, QStringLiteral("place"), QJsonValue::String, where, error)
            || !requireType(weather, QStringLiteral("scene"), QJsonValue::Bool, where, error))
            return false;
        const QJsonValue currentValue = weather.value(QStringLiteral("current"));
        if (!currentValue.isNull()) {
            if (!currentValue.isObject()) {
                if (error)
                    *error = QStringLiteral("state.weather.current: must be an object or null");
                return false;
            }
            const QJsonObject current = currentValue.toObject();
            const QString cw = QStringLiteral("state.weather.current");
            if (!requireKeys(current, {QStringLiteral("temperature"), QStringLiteral("condition"), QStringLiteral("intensity"), QStringLiteral("is_day"), QStringLiteral("observed_at")}, cw, error)
                || !requireType(current, QStringLiteral("temperature"), QJsonValue::Double, cw, error)
                || !requireEnum(current, QStringLiteral("condition"),
                                {QStringLiteral("clear"), QStringLiteral("partly-cloudy"), QStringLiteral("cloudy"), QStringLiteral("fog"), QStringLiteral("drizzle"), QStringLiteral("rain"),
                                 QStringLiteral("snow"), QStringLiteral("thunder")},
                                cw, error)
                || !requireEnum(current, QStringLiteral("intensity"), {QStringLiteral("light"), QStringLiteral("moderate"), QStringLiteral("heavy")}, cw, error)
                || !requireType(current, QStringLiteral("is_day"), QJsonValue::Bool, cw, error)
                || !requireType(current, QStringLiteral("observed_at"), QJsonValue::String, cw, error))
                return false;
            const double t = current.value(QStringLiteral("temperature")).toDouble();
            if (t != std::floor(t)) {
                if (error)
                    *error = QStringLiteral("state.weather.current.temperature must be an integer");
                return false;
            }
        }
    }
    if (snapshot.contains(QStringLiteral("now_playing")) && !snapshot.value(QStringLiteral("now_playing")).isNull()) {
        // state.now_playing (optional, phones only; the shell never shows it
        // but accepts a snapshot that carries it): app_id/title/status/position_at/rate.
        const QString where = QStringLiteral("state.now_playing");
        if (!requireType(snapshot, QStringLiteral("now_playing"), QJsonValue::Object, QStringLiteral("state"), error))
            return false;
        const QJsonObject np = snapshot.value(QStringLiteral("now_playing")).toObject();
        if (!requireKeys(np, {QStringLiteral("app_id"), QStringLiteral("title"), QStringLiteral("status"), QStringLiteral("position_at"), QStringLiteral("rate")}, where, error)
            || !requireEnum(np, QStringLiteral("status"), {QStringLiteral("playing"), QStringLiteral("paused"), QStringLiteral("stopped")}, where, error)
            || !requireType(np, QStringLiteral("title"), QJsonValue::String, where, error)
            || !requireType(np, QStringLiteral("position_at"), QJsonValue::Double, where, error)
            || !requireType(np, QStringLiteral("rate"), QJsonValue::Double, where, error))
            return false;
        // foreground (optional): false while the app plays behind Home.
        if (np.contains(QStringLiteral("foreground")) && !requireType(np, QStringLiteral("foreground"), QJsonValue::Bool, where, error))
            return false;
        // source (optional): mpris or plex_server.
        if (np.contains(QStringLiteral("source")) && !requireEnum(np, QStringLiteral("source"), {QStringLiteral("mpris"), QStringLiteral("plex_server")}, where, error))
            return false;
    }
    if (snapshot.contains(QStringLiteral("power"))) {
        // state.power (optional): sleep_at_ms (integer or null), warning, display on|off.
        const QString where = QStringLiteral("state.power");
        if (!requireType(snapshot, QStringLiteral("power"), QJsonValue::Object, QStringLiteral("state"), error))
            return false;
        const QJsonObject power = snapshot.value(QStringLiteral("power")).toObject();
        if (!requireKeys(power, {QStringLiteral("sleep_at_ms"), QStringLiteral("warning"), QStringLiteral("display")}, where, error)
            || !requireEnum(power, QStringLiteral("display"), {QStringLiteral("on"), QStringLiteral("off")}, where, error)
            || !requireType(power, QStringLiteral("warning"), QJsonValue::Bool, where, error))
            return false;
        const QJsonValue at = power.value(QStringLiteral("sleep_at_ms"));
        if (!at.isNull() && !(at.isDouble() && at.toDouble() == std::floor(at.toDouble()))) {
            if (error)
                *error = QStringLiteral("state.power.sleep_at_ms must be an integer or null");
            return false;
        }
    }
    if (snapshot.contains(QStringLiteral("cec"))) {
        // state.cec (optional): available, enabled, volume_target pc|tv,
        // tv_power on|standby|unknown; reason (string) when unavailable.
        const QString where = QStringLiteral("state.cec");
        if (!requireType(snapshot, QStringLiteral("cec"), QJsonValue::Object, QStringLiteral("state"), error))
            return false;
        const QJsonObject cec = snapshot.value(QStringLiteral("cec")).toObject();
        if (!requireKeys(cec, {QStringLiteral("available"), QStringLiteral("enabled"), QStringLiteral("volume_target"), QStringLiteral("tv_power")}, where, error)
            || !requireType(cec, QStringLiteral("available"), QJsonValue::Bool, where, error)
            || !requireType(cec, QStringLiteral("enabled"), QJsonValue::Bool, where, error)
            || !requireEnum(cec, QStringLiteral("volume_target"), {QStringLiteral("pc"), QStringLiteral("tv")}, where, error)
            || !requireEnum(cec, QStringLiteral("tv_power"), {QStringLiteral("on"), QStringLiteral("standby"), QStringLiteral("unknown")}, where, error))
            return false;
        if (cec.contains(QStringLiteral("reason")) && !requireType(cec, QStringLiteral("reason"), QJsonValue::String, where, error))
            return false;
    }
    if (snapshot.contains(QStringLiteral("onboarding"))) {
        // state.onboarding (optional, shell view only): completed (boolean).
        const QString where = QStringLiteral("state.onboarding");
        if (!requireType(snapshot, QStringLiteral("onboarding"), QJsonValue::Object, QStringLiteral("state"), error))
            return false;
        const QJsonObject onboarding = snapshot.value(QStringLiteral("onboarding")).toObject();
        if (!requireKeys(onboarding, {QStringLiteral("completed")}, where, error)
            || !requireType(onboarding, QStringLiteral("completed"), QJsonValue::Bool, where, error))
            return false;
    }
    if (snapshot.contains(QStringLiteral("autostart"))) {
        // state.autostart (optional, shell view only): enabled, available
        // (booleans); reason (string) when unavailable.
        const QString where = QStringLiteral("state.autostart");
        if (!requireType(snapshot, QStringLiteral("autostart"), QJsonValue::Object, QStringLiteral("state"), error))
            return false;
        const QJsonObject autostart = snapshot.value(QStringLiteral("autostart")).toObject();
        if (!requireKeys(autostart, {QStringLiteral("enabled"), QStringLiteral("available")}, where, error)
            || !requireType(autostart, QStringLiteral("enabled"), QJsonValue::Bool, where, error)
            || !requireType(autostart, QStringLiteral("available"), QJsonValue::Bool, where, error))
            return false;
        if (autostart.contains(QStringLiteral("reason")) && !requireType(autostart, QStringLiteral("reason"), QJsonValue::String, where, error))
            return false;
    }
    if (snapshot.contains(QStringLiteral("plex"))) {
        // state.plex (optional, shell view only): the Plex sign-in flow.
        const QString where = QStringLiteral("state.plex");
        if (!requireType(snapshot, QStringLiteral("plex"), QJsonValue::Object, QStringLiteral("state"), error))
            return false;
        const QJsonObject plex = snapshot.value(QStringLiteral("plex")).toObject();
        if (!requireKeys(plex, {QStringLiteral("status"), QStringLiteral("message"), QStringLiteral("code"), QStringLiteral("link_url"), QStringLiteral("server"), QStringLiteral("servers"), QStringLiteral("libraries")}, where, error)
            || !requireEnum(plex, QStringLiteral("status"),
                            {QStringLiteral("signed_out"), QStringLiteral("linking"), QStringLiteral("choose_server"), QStringLiteral("choose_libraries"), QStringLiteral("connected"), QStringLiteral("error")},
                            where, error)
            || !requireType(plex, QStringLiteral("servers"), QJsonValue::Array, where, error)
            || !requireType(plex, QStringLiteral("libraries"), QJsonValue::Array, where, error))
            return false;
        for (const QJsonValue &sv : plex.value(QStringLiteral("servers")).toArray()) {
            if (!requireKeys(sv.toObject(), {QStringLiteral("id"), QStringLiteral("name"), QStringLiteral("owned"), QStringLiteral("local")}, QStringLiteral("state.plex.servers[]"), error))
                return false;
        }
        for (const QJsonValue &lv : plex.value(QStringLiteral("libraries")).toArray()) {
            const QJsonObject lib = lv.toObject();
            if (!requireKeys(lib, {QStringLiteral("id"), QStringLiteral("title"), QStringLiteral("kind"), QStringLiteral("selected")}, QStringLiteral("state.plex.libraries[]"), error)
                || !requireEnum(lib, QStringLiteral("kind"), {QStringLiteral("movie"), QStringLiteral("show"), QStringLiteral("artist"), QStringLiteral("photo"), QStringLiteral("other")},
                                QStringLiteral("state.plex.libraries[]"), error))
                return false;
        }
    }
    if (snapshot.contains(QStringLiteral("achievements"))) {
        // state.achievements (optional): Den badges. Only ids, counts and the
        // local day each was earned; a time in `day` is rejected.
        const QString where = QStringLiteral("state.achievements");
        if (!requireType(snapshot, QStringLiteral("achievements"), QJsonValue::Object, QStringLiteral("state"), error))
            return false;
        const QJsonObject ach = snapshot.value(QStringLiteral("achievements")).toObject();
        if (!requireKeys(ach, {QStringLiteral("enabled"), QStringLiteral("earned"), QStringLiteral("progress")}, where, error)
            || !requireType(ach, QStringLiteral("enabled"), QJsonValue::Bool, where, error)
            || !requireType(ach, QStringLiteral("earned"), QJsonValue::Array, where, error)
            || !requireType(ach, QStringLiteral("progress"), QJsonValue::Array, where, error))
            return false;
        static const QRegularExpression dayRe(QStringLiteral("^[0-9]{4}-[0-9]{2}-[0-9]{2}$"));
        for (const QJsonValue &ev : ach.value(QStringLiteral("earned")).toArray()) {
            const QJsonObject e = ev.toObject();
            const QString w = QStringLiteral("state.achievements.earned[]");
            if (!requireKeys(e, {QStringLiteral("id"), QStringLiteral("day")}, w, error)
                || !requireType(e, QStringLiteral("id"), QJsonValue::String, w, error)
                || !requireType(e, QStringLiteral("day"), QJsonValue::String, w, error))
                return false;
            if (!dayRe.match(e.value(QStringLiteral("day")).toString()).hasMatch()) {
                if (error)
                    *error = QStringLiteral("state.achievements.earned[].day must be a calendar day (YYYY-MM-DD)");
                return false;
            }
        }
        for (const QJsonValue &pv : ach.value(QStringLiteral("progress")).toArray()) {
            const QJsonObject p = pv.toObject();
            const QString w = QStringLiteral("state.achievements.progress[]");
            if (!requireKeys(p, {QStringLiteral("id"), QStringLiteral("count"), QStringLiteral("goal")}, w, error)
                || !requireType(p, QStringLiteral("count"), QJsonValue::Double, w, error)
                || !requireType(p, QStringLiteral("goal"), QJsonValue::Double, w, error))
                return false;
        }
        if (ach.contains(QStringLiteral("celebrate"))
            && !requireType(ach, QStringLiteral("celebrate"), QJsonValue::Array, where, error))
            return false;
    }
    return true;
}

bool SessionModel::applySnapshot(const QJsonObject &snapshot)
{
    QString error;
    if (!validateSnapshot(snapshot, &error)) {
        m_lastError = error;
        emit snapshotRejected(error);
        return false;
    }
    m_snapshot = snapshot;
    m_lastError.clear();
    m_loaded = true;
    if (snapshot.contains(QStringLiteral("layout")))
        m_layout = snapshot.value(QStringLiteral("layout")).toObject();
    if (Theme::instance() && !m_previewActive)
        Theme::instance()->applyUi(m_layout.value(QStringLiteral("ui")).toObject().toVariantMap());
    // A look tried on the TV and then applied: the coordinator's layout now
    // says the same, so the preview has done its job.
    if (m_localPreview && m_layout.value(QStringLiteral("ui")) == m_previewLayout.value(QStringLiteral("ui"))) {
        m_localPreview = false;
        m_previewActive = false;
        m_previewLayout = {};
    }
    rebuildSections();
    emit snapshotChanged();
    emit layoutChanged();
    return true;
}

bool SessionModel::loadFixture(const QString &path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly)) {
        m_lastError = QStringLiteral("cannot open fixture %1").arg(path);
        emit snapshotRejected(m_lastError);
        return false;
    }
    QJsonParseError parseError;
    const QJsonDocument doc = QJsonDocument::fromJson(file.readAll(), &parseError);
    if (!doc.isObject()) {
        m_lastError = QStringLiteral("fixture %1: %2").arg(path, parseError.errorString());
        emit snapshotRejected(m_lastError);
        return false;
    }
    return applySnapshot(doc.object());
}

void SessionModel::applyLayoutPreview(const QJsonObject &layout)
{
    QString error;
    if (!validateLayout(layout, &error)) {
        m_lastError = error;
        emit snapshotRejected(error);
        return;
    }
    m_previewLayout = layout;
    m_previewActive = true;
    m_localPreview = false; // a phone's preview replaces a look tried on the TV
    if (Theme::instance())
        Theme::instance()->applyUi(layout.value(QStringLiteral("ui")).toObject().toVariantMap());
    rebuildSections();
    emit layoutChanged();
}

void SessionModel::previewUi(const QVariantMap &ui)
{
    QJsonObject layout = m_layout;
    layout.insert(QStringLiteral("ui"), QJsonObject::fromVariantMap(ui));
    applyLayoutPreview(layout);
    m_localPreview = m_previewActive && m_previewLayout == layout;
    emit layoutChanged();
}

void SessionModel::endUiPreview()
{
    if (!m_localPreview)
        return;
    m_localPreview = false;
    endLayoutPreview();
}

void SessionModel::endLayoutPreview()
{
    if (!m_previewActive)
        return;
    m_previewActive = false;
    m_previewLayout = {};
    if (Theme::instance())
        Theme::instance()->applyUi(m_layout.value(QStringLiteral("ui")).toObject().toVariantMap());
    rebuildSections();
    emit layoutChanged();
}

QVariantMap SessionModel::shellFocus() const
{
    return m_snapshot.value(QStringLiteral("shell")).toObject().value(QStringLiteral("focus")).toObject().toVariantMap();
}

QVariantMap SessionModel::application(const QString &id) const
{
    for (const QJsonValue &v : m_snapshot.value(QStringLiteral("applications")).toArray()) {
        const QJsonObject app = v.toObject();
        if (app.value(QStringLiteral("id")).toString() == id)
            return app.toVariantMap();
    }
    return {};
}

void SessionModel::rebuildSections()
{
    QHash<QString, QJsonObject> apps;
    for (const QJsonValue &v : m_snapshot.value(QStringLiteral("applications")).toArray()) {
        const QJsonObject app = v.toObject();
        apps.insert(app.value(QStringLiteral("id")).toString(), app);
    }
    QHash<QString, QJsonArray> contentBySection;
    const bool contentReady = hasContent();
    for (const QJsonValue &v : m_snapshot.value(QStringLiteral("content")).toObject().value(QStringLiteral("sections")).toArray()) {
        const QJsonObject s = v.toObject();
        contentBySection.insert(s.value(QStringLiteral("section_id")).toString(), s.value(QStringLiteral("items")).toArray());
    }
    // The apps Bear Den could install now (state.applications[].install, not
    // "none"), in the coordinator's order, those without a tile of their own
    // first: the first apps rail ends with an "Add apps" tile naming the
    // first two while there are any.
    QStringList addable;
    QStringList addableWithTile;
    for (const QJsonValue &v : m_snapshot.value(QStringLiteral("applications")).toArray()) {
        const QJsonObject app = v.toObject();
        const QJsonObject install = app.value(QStringLiteral("install")).toObject();
        if (app.value(QStringLiteral("installed")).toBool() || install.isEmpty() || install.value(QStringLiteral("state")).toString() == QLatin1String("none"))
            continue;
        const QString label = app.value(QStringLiteral("label")).toString();
        QStringList &list = app.value(QStringLiteral("hidden")).toBool() ? addable : addableWithTile;
        if (!label.isEmpty() && !list.contains(label))
            list << label;
    }
    addable << addableWithTile;
    bool addTilePlaced = false;
    Theme *theme = Theme::instance();
    QList<SectionsModel::Section> visible;
    for (const QJsonValue &v : effectiveLayout().value(QStringLiteral("sections")).toArray()) {
        const QJsonObject def = v.toObject();
        if (!def.value(QStringLiteral("enabled")).toBool())
            continue;
        SectionsModel::Section section;
        section.id = def.value(QStringLiteral("id")).toString();
        section.title = def.value(QStringLiteral("title")).toString();
        section.kind = def.value(QStringLiteral("kind")).toString();
        if (section.kind == QLatin1String("applications")) {
            for (const QJsonValue &idv : def.value(QStringLiteral("application_ids")).toArray()) {
                const QString appId = idv.toString();
                if (!apps.contains(appId))
                    continue; // dangling reference: the coordinator's config layer rejects these; never fabricate a tile
                const QJsonObject app = apps.value(appId);
                if (app.value(QStringLiteral("hidden")).toBool())
                    continue; // an optional app that is not installed: no tile (contracts/state.schema.json)
                ItemsModel::Item item;
                item.id = appId;
                item.appId = appId;
                item.kind = QStringLiteral("app");
                item.title = app.value(QStringLiteral("label")).toString();
                item.installed = app.value(QStringLiteral("installed")).toBool();
                item.running = app.value(QStringLiteral("running")).toBool();
                item.launchState = app.value(QStringLiteral("launch_state")).toString();
                const QJsonObject install = app.value(QStringLiteral("install")).toObject();
                item.installState = install.value(QStringLiteral("state")).toString();
                item.installProgress = install.value(QStringLiteral("progress")).toInt();
                if (item.installState == QLatin1String("preparing") || item.installState == QLatin1String("downloading")
                    || item.installState == QLatin1String("installing"))
                    item.subtitle = QStringLiteral("Installing… %1%").arg(item.installProgress);
                else if (!item.installed && item.installState == QLatin1String("available"))
                    item.subtitle = QStringLiteral("Not installed — press OK to install");
                else if (!item.installed)
                    item.subtitle = QStringLiteral("Not installed — see Apps");
                else if (item.launchState == QLatin1String("launching"))
                    item.subtitle = QStringLiteral("Starting…");
                else if (item.launchState == QLatin1String("failed") || item.launchState == QLatin1String("crashed"))
                    item.subtitle = QStringLiteral("Last launch failed");
                else if (item.running)
                    item.subtitle = QStringLiteral("Running");
                else
                    item.subtitle = QStringLiteral("Ready");
                item.tint = theme ? theme->tintFor(appId) : QColor(Qt::gray);
                section.items.append(item);
            }
            if (!addTilePlaced && !addable.isEmpty()) {
                addTilePlaced = true;
                ItemsModel::Item add;
                add.id = QStringLiteral("add-apps");
                add.kind = QStringLiteral("add-apps");
                add.title = QStringLiteral("Add apps");
                add.subtitle = addable.size() == 1 ? addable.first()
                             : addable.size() == 2 ? QStringLiteral("%1 and %2").arg(addable.at(0), addable.at(1))
                                                   : QStringLiteral("%1, %2 and more").arg(addable.at(0), addable.at(1));
                add.tint = theme ? theme->accent() : QColor(Qt::gray);
                section.items.append(add);
            }
        } else {
            const QJsonArray items = contentBySection.value(section.id);
            for (const QJsonValue &iv : items) {
                const QJsonObject c = iv.toObject();
                ItemsModel::Item item;
                item.id = c.value(QStringLiteral("id")).toString();
                item.kind = QStringLiteral("content");
                item.title = c.value(QStringLiteral("title")).toString();
                item.subtitle = c.value(QStringLiteral("subtitle")).toString();
                item.progress = c.value(QStringLiteral("progress")).isNull() ? -1 : c.value(QStringLiteral("progress")).toDouble();
                item.demo = c.value(QStringLiteral("demo")).toBool();
                item.openAction = c.value(QStringLiteral("open_action")).toString();
                // Artwork is a coordinator-local path (same machine) or a proxied URL.
                const QString art = c.value(QStringLiteral("artwork")).toString();
                item.artwork = art.startsWith(QLatin1Char('/')) ? QUrl::fromLocalFile(art).toString() : art;
                item.tint = theme ? theme->tintFor(item.id) : QColor(Qt::gray);
                section.items.append(item);
            }
            if (section.items.isEmpty()) {
                // An empty row that is loading or failing says so, even when it
                // would otherwise hide: an honest "Can't reach your Plex server"
                // beats rows that silently vanish.
                const QJsonObject content = m_snapshot.value(QStringLiteral("content")).toObject();
                const QString status = content.value(QStringLiteral("status")).toString();
                const bool failing = contentReady && status == QLatin1String("error");
                const bool loading = contentReady && status == QLatin1String("connecting");
                if (def.value(QStringLiteral("hide_when_empty")).toBool() && !failing && !loading)
                    continue;
                section.isEmpty = true;
                QString title;
                if (failing) {
                    title = QStringLiteral("Can't load right now");
                    section.emptyMessage = content.value(QStringLiteral("message")).toString();
                    if (section.emptyMessage.isEmpty())
                        section.emptyMessage = QStringLiteral("%1 could not be loaded").arg(section.title);
                } else if (loading) {
                    title = QStringLiteral("Loading…");
                    section.emptyMessage = QStringLiteral("Asking your Plex server");
                } else if (contentReady && status != QLatin1String("disabled")) {
                    title = QStringLiteral("Nothing here yet");
                    section.emptyMessage = title;
                } else {
                    title = QStringLiteral("Connect Plex");
                    section.emptyMessage = QStringLiteral("Connect Plex to see %1").arg(section.title);
                }
                ItemsModel::Item setup;
                setup.id = QStringLiteral("%1--setup").arg(section.id);
                setup.kind = QStringLiteral("setup");
                setup.title = title;
                setup.subtitle = section.emptyMessage;
                setup.tint = theme ? theme->accent() : QColor(Qt::gray);
                section.items.append(setup);
            }
        }
        if (section.items.isEmpty() && def.value(QStringLiteral("hide_when_empty")).toBool())
            continue;
        visible.append(section);
    }
    m_sections->reconcile(visible);
}
