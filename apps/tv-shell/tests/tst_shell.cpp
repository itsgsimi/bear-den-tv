// Focus-graph and screen tests for the TV shell (UI-01..03). The shell is
// loaded offline from a DEMO fixture and driven through Nav.apply, the same
// entry point the coordinator's `input` messages use.
#include "FocusMemory.h"
#include "IpcClient.h"
#include "Navigator.h"
#include "ItemsModel.h"
#include "SectionsModel.h"
#include "SessionModel.h"
#include "ShellController.h"
#include "Theme.h"
#include "ThemeRegistry.h"

#include <QDateTime>
#include <QDir>
#include <QFile>
#include <QImageReader>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QQmlApplicationEngine>
#include <QQmlComponent>
#include <QQuickItem>
#include <QQuickWindow>
#include <QTemporaryDir>
#include <QtTest>

#include <cmath>
#include <functional>
#include <memory>

class ShellTest : public QObject {
    Q_OBJECT

private:
    QQmlApplicationEngine *m_engine = nullptr;
    QQuickWindow *m_window = nullptr;
    Navigator *m_nav = nullptr;
    QString m_shotDir;

    QJsonObject fixture() const
    {
        QFile f(QStringLiteral(BDTV_FIXTURE_DIR "/state.demo.json"));
        if (!f.open(QIODevice::ReadOnly))
            return {};
        return QJsonDocument::fromJson(f.readAll()).object();
    }
    QVariantMap act(const QString &action)
    {
        const QVariantMap r = m_nav->apply(action);
        QCoreApplication::processEvents();
        return r;
    }
    void shot(const QString &name)
    {
        if (m_shotDir.isEmpty())
            return;
        QTest::qWait(250);
        m_window->grabWindow().save(QDir(m_shotDir).filePath(name + QStringLiteral(".png")));
    }
    void goHome()
    {
        act(QStringLiteral("home"));
        QCOMPARE(m_nav->screen(), QStringLiteral("home"));
    }
    // Home restores the last focused section; tests that need a fixed start walk there.
    void toHeader()
    {
        for (int i = 0; i < 5 && m_nav->sectionId() != QLatin1String("header"); ++i)
            act(QStringLiteral("nav.up"));
        QCOMPARE(m_nav->sectionId(), QStringLiteral("header"));
    }
    void toFavorites()
    {
        toHeader();
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->sectionId(), QStringLiteral("favorites"));
    }

    // Every visible settings row, and the focused row's ring, fits inside the
    // clipping list that holds it: nothing of a rounded corner is cut off.
    void settingsRowsFitTheirList()
    {
        QTest::qWait(Theme::instance()->durationFast() + 50); // the focused row's scale-up
        int rows = 0;
        QList<QQuickItem *> items; // delegates are visual children only, so walk the item tree
        std::function<void(QQuickItem *)> collect = [&](QQuickItem *item) {
            if (!item->isVisible())
                return;
            if (item->objectName() == QLatin1String("settingsRow"))
                items.append(item);
            for (QQuickItem *child : item->childItems())
                collect(child);
        };
        collect(m_window->contentItem());
        for (QQuickItem *row : items) {
            QQuickItem *clip = row->parentItem();
            while (clip && !clip->clip())
                clip = clip->parentItem();
            QVERIFY(clip);
            const QRectF box = clip->mapRectToScene(clip->boundingRect());
            QQuickItem *outer = row;
            for (QQuickItem *child : row->childItems())
                if (child->objectName() == QLatin1String("focusFrame") && child->isVisible())
                    outer = child;
            const QRectF r = outer->mapRectToScene(outer->boundingRect());
            if (r.bottom() < box.top() || r.top() > box.bottom())
                continue; // scrolled out of the list
            QVERIFY2(r.left() >= box.left() && r.right() <= box.right(),
                     qPrintable(QStringLiteral("%1 spans %2..%3, its list %4..%5")
                                    .arg(row->property("label").toString())
                                    .arg(r.left()).arg(r.right()).arg(box.left()).arg(box.right())));
            ++rows;
        }
        QVERIFY(rows > 3);
    }

    // Local weather in the corner scene (SceneWeather.qml, World.weatherLook;
    // docs/THEMES.md → Weather in the corner scene). Applies the demo snapshot
    // with `condition` (empty: no weather at all), day or night, reduced
    // motion and art style.
    void applyWeather(const QString &condition, bool day = true, bool still = false, const QString &art = QStringLiteral("pixel"))
    {
        QJsonObject snap = fixture();
        QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
        QJsonObject ui = layout.value(QStringLiteral("ui")).toObject();
        ui.insert(QStringLiteral("reduced_motion"), still);
        ui.insert(QStringLiteral("art_style"), art);
        layout.insert(QStringLiteral("ui"), ui);
        snap.insert(QStringLiteral("layout"), layout);
        if (condition.isEmpty()) {
            snap.remove(QStringLiteral("weather"));
        } else {
            QJsonObject w = snap.value(QStringLiteral("weather")).toObject();
            QJsonObject cur = w.value(QStringLiteral("current")).toObject();
            cur.insert(QStringLiteral("condition"), condition);
            cur.insert(QStringLiteral("is_day"), day);
            w.insert(QStringLiteral("current"), cur);
            w.insert(QStringLiteral("scene"), true);
            snap.insert(QStringLiteral("weather"), w);
        }
        QVERIFY2(SessionModel::instance()->applySnapshot(snap), qPrintable(SessionModel::instance()->lastError()));
        QCoreApplication::processEvents();
    }
    // The scene's SceneWeather for `side` ("back" or "front").
    static QObject *weatherSide(QObject *scene, const QString &side)
    {
        for (QObject *o : scene->findChildren<QObject *>(QStringLiteral("sceneWeather")))
            if (o->property("side").toString() == side)
                return o;
        return nullptr;
    }
    static QString drawn(QObject *wx)
    {
        return wx ? wx->property("drawn").toStringList().join(QLatin1Char(',')) : QStringLiteral("<none>");
    }

private slots:
    void initTestCase()
    {
        m_shotDir = qEnvironmentVariable("BDTV_SCREENSHOT_DIR");
        Theme::create(nullptr, nullptr)->setForceNoAnimations(true);
        m_nav = Navigator::create(nullptr, nullptr);
        SessionModel::create(nullptr, nullptr);
        FocusMemory::create(nullptr, nullptr);
        ShellController::Options opts;
        opts.offline = true;
        opts.fixturePath = QStringLiteral(BDTV_FIXTURE_DIR "/state.demo.json");
        auto *controller = ShellController::create(nullptr, nullptr);
        controller->configure(opts);

        m_engine = new QQmlApplicationEngine(this);
        m_engine->setInitialProperties({{QStringLiteral("fullscreen"), false}});
        m_engine->loadFromModule("BearDen", "Main");
        QVERIFY(!m_engine->rootObjects().isEmpty());
        m_window = qobject_cast<QQuickWindow *>(m_engine->rootObjects().constFirst());
        QVERIFY(m_window);
        m_nav->setWindow(m_window);
        m_window->requestActivate();
        controller->start();
        QVERIFY2(SessionModel::instance()->loaded(), qPrintable(SessionModel::instance()->lastError()));
        QTRY_VERIFY(m_window->isExposed());
        QTRY_COMPARE(m_nav->sectionId(), QStringLiteral("favorites"));
    }

    void homeStartsOnFirstApp()
    {
        goHome(); // first visit: no focus memory yet
        QCOMPARE(m_nav->sectionId(), QStringLiteral("favorites"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("plex-htpc"));
        shot(QStringLiteral("home"));
    }

    void leftRightMovesWithinRailAndStopsAtEdges()
    {
        goHome();
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("youtube"));
        act(QStringLiteral("nav.right")); // last app: stays put
        QCOMPARE(m_nav->itemId(), QStringLiteral("youtube"));
        act(QStringLiteral("nav.left"));
        act(QStringLiteral("nav.left"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("plex-htpc"));
    }

    void upDownChangesSectionsAndRestoresItemById()
    {
        goHome();
        act(QStringLiteral("nav.right")); // youtube
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->sectionId(), QStringLiteral("plex-continue"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("demo-1"));
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("demo-3"));
        shot(QStringLiteral("home-continue"));
        act(QStringLiteral("nav.up"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("youtube"));
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("demo-3"));
    }

    void backAtRootIsReportedNoOp()
    {
        goHome();
        const QVariantMap r = act(QStringLiteral("back"));
        QCOMPARE(r.value(QStringLiteral("outcome")).toString(), QStringLiteral("observed"));
        QVERIFY(r.value(QStringLiteral("detail")).toMap().value(QStringLiteral("at_root")).toBool());
        QCOMPARE(m_nav->screen(), QStringLiteral("home"));
    }

    void headerPillsOpenScreensAndBackReturns()
    {
        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("settings"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        shot(QStringLiteral("settings"));
        settingsRowsFitTheirList();
        act(QStringLiteral("nav.down"));
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("devices"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("devices"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("dev_a1"));
        shot(QStringLiteral("devices"));
        act(QStringLiteral("select")); // revoke confirmation, focus on the safe choice
        QCOMPARE(m_nav->screen(), QStringLiteral("dialog"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("cancel"));
        shot(QStringLiteral("devices-confirm"));
        act(QStringLiteral("back"));
        QCOMPARE(m_nav->screen(), QStringLiteral("devices"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("dev_a1")); // modal focus returns to its control
        act(QStringLiteral("back"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        act(QStringLiteral("back"));
        QCOMPARE(m_nav->screen(), QStringLiteral("home"));
    }

    void secondaryScreensRender()
    {
        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("pairing"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("pairing"));
        shot(QStringLiteral("pairing"));
        act(QStringLiteral("back"));
        act(QStringLiteral("back"));
        QCOMPARE(m_nav->screen(), QStringLiteral("home"));
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select"));
        for (int i = 0; i < 20; ++i) // Settings remembers its row; walk to the top first
            act(QStringLiteral("nav.up"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("remote"));
        for (int i = 0; i < 17; ++i) // remote, pairing, devices, now playing … theme, style, art style, margin, motion, contrast, hero, clock, weather, playback, advanced playback
            act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("diagnostics"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("diagnostics"));
        shot(QStringLiteral("diagnostics"));
        goHome();
    }

    // Local weather (state.weather): the header chip shows the reading, a bad
    // enum rejects the whole snapshot, and Settings → Weather searches from its
    // text field (Return → weather.search) and configures from a result.
    void weatherChipShowsTemperature()
    {
        goHome();
        QObject *chip = m_window->findChild<QObject *>(QStringLiteral("weatherChip"));
        QVERIFY(chip);
        QVERIFY(chip->property("visible").toBool());
        QObject *temp = m_window->findChild<QObject *>(QStringLiteral("weatherTemperature"));
        QVERIFY(temp);
        QCOMPARE(temp->property("text").toString(), QStringLiteral("12°"));
        shot(QStringLiteral("home-weather"));

        // No reading: the chip hides.
        SessionModel *session = SessionModel::instance();
        QJsonObject snap = fixture();
        QJsonObject w = snap.value(QStringLiteral("weather")).toObject();
        w.insert(QStringLiteral("status"), QStringLiteral("error"));
        w.insert(QStringLiteral("current"), QJsonValue::Null);
        snap.insert(QStringLiteral("weather"), w);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QCoreApplication::processEvents();
        QVERIFY(!chip->property("visible").toBool());
        QVERIFY(session->applySnapshot(fixture()));
        QCoreApplication::processEvents();
        QVERIFY(chip->property("visible").toBool());
    }

    void badWeatherRejectsSnapshot()
    {
        SessionModel *session = SessionModel::instance();
        const int epoch = session->contextEpoch();
        QJsonObject snap = fixture();
        snap.insert(QStringLiteral("context_epoch"), epoch + 100);
        QJsonObject w = snap.value(QStringLiteral("weather")).toObject();
        QJsonObject current = w.value(QStringLiteral("current")).toObject();
        current.insert(QStringLiteral("condition"), QStringLiteral("hail"));
        w.insert(QStringLiteral("current"), current);
        snap.insert(QStringLiteral("weather"), w);
        QVERIFY(!session->applySnapshot(snap));
        QVERIFY2(session->lastError().contains(QStringLiteral("state.weather.current")), qPrintable(session->lastError()));
        QCOMPARE(session->contextEpoch(), epoch); // the previous state stays

        w = fixture().value(QStringLiteral("weather")).toObject();
        w.insert(QStringLiteral("status"), QStringLiteral("sunny"));
        snap.insert(QStringLiteral("weather"), w);
        QVERIFY(!session->applySnapshot(snap));
        w = fixture().value(QStringLiteral("weather")).toObject();
        w.remove(QStringLiteral("units"));
        snap.insert(QStringLiteral("weather"), w);
        QVERIFY(!session->applySnapshot(snap));
        // Absent is fine (weather off, or an older coordinator).
        snap.remove(QStringLiteral("weather"));
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QVERIFY(session->weather().isEmpty());
        QVERIFY(session->applySnapshot(fixture()));
    }

    // state.now_playing is phone-only, but a snapshot that carries it must
    // still be accepted (and a malformed one rejected, keeping the old state).
    void nowPlayingAcceptedAndChecked()
    {
        SessionModel *session = SessionModel::instance();
        const int epoch = session->contextEpoch();
        QJsonObject snap = fixture();
        snap.insert(QStringLiteral("context_epoch"), epoch + 200);
        QJsonObject np{{QStringLiteral("app_id"), QStringLiteral("plex-htpc")}, {QStringLiteral("title"), QStringLiteral("DEMO Episode")},
                       {QStringLiteral("subtitle"), QStringLiteral("DEMO Show")}, {QStringLiteral("status"), QStringLiteral("playing")},
                       {QStringLiteral("length_ms"), 2640000}, {QStringLiteral("position_ms"), 754000},
                       {QStringLiteral("position_at"), 203500}, {QStringLiteral("rate"), 1}};
        snap.insert(QStringLiteral("now_playing"), np);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QCOMPARE(session->contextEpoch(), epoch + 200);
        snap.insert(QStringLiteral("now_playing"), QJsonValue::Null);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));

        snap.insert(QStringLiteral("context_epoch"), epoch + 300);
        np.insert(QStringLiteral("status"), QStringLiteral("buffering"));
        snap.insert(QStringLiteral("now_playing"), np);
        QVERIFY(!session->applySnapshot(snap));
        QVERIFY2(session->lastError().contains(QStringLiteral("state.now_playing")), qPrintable(session->lastError()));
        np.insert(QStringLiteral("status"), QStringLiteral("paused"));
        np.remove(QStringLiteral("position_at"));
        snap.insert(QStringLiteral("now_playing"), np);
        QVERIFY(!session->applySnapshot(snap));
        QCOMPARE(session->contextEpoch(), epoch + 200); // the previous state stays
        QVERIFY(session->applySnapshot(fixture()));
    }

    // state.power (sleep timer and display) is accepted with a timer or with
    // null, exposed as Session.power, and a malformed one is rejected while
    // the previous state stays.
    void powerAcceptedAndChecked()
    {
        SessionModel *session = SessionModel::instance();
        const int epoch = session->contextEpoch();
        QJsonObject snap = fixture();
        snap.insert(QStringLiteral("context_epoch"), epoch + 400);
        QJsonObject power{{QStringLiteral("sleep_at_ms"), 2685000}, {QStringLiteral("sleep_minutes"), 45},
                          {QStringLiteral("warning"), true}, {QStringLiteral("display"), QStringLiteral("on")},
                          {QStringLiteral("suspend"), QJsonObject{{QStringLiteral("available"), false}, {QStringLiteral("reason"), QStringLiteral("The system asks for a password to suspend.")}}}};
        snap.insert(QStringLiteral("power"), power);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QCOMPARE(session->power().value(QStringLiteral("warning")).toBool(), true);
        QCOMPARE(session->power().value(QStringLiteral("sleep_minutes")).toInt(), 45);
        power.insert(QStringLiteral("sleep_at_ms"), QJsonValue::Null);
        power.insert(QStringLiteral("display"), QStringLiteral("off"));
        snap.insert(QStringLiteral("power"), power);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QCOMPARE(session->power().value(QStringLiteral("display")).toString(), QStringLiteral("off"));

        snap.insert(QStringLiteral("context_epoch"), epoch + 500);
        power.insert(QStringLiteral("display"), QStringLiteral("dim"));
        snap.insert(QStringLiteral("power"), power);
        QVERIFY(!session->applySnapshot(snap));
        QVERIFY2(session->lastError().contains(QStringLiteral("state.power")), qPrintable(session->lastError()));
        power.insert(QStringLiteral("display"), QStringLiteral("on"));
        power.insert(QStringLiteral("sleep_at_ms"), 12.5);
        snap.insert(QStringLiteral("power"), power);
        QVERIFY(!session->applySnapshot(snap));
        power.insert(QStringLiteral("sleep_at_ms"), QJsonValue::Null);
        power.remove(QStringLiteral("warning"));
        snap.insert(QStringLiteral("power"), power);
        QVERIFY(!session->applySnapshot(snap));
        QCOMPARE(session->contextEpoch(), epoch + 400); // the previous state stays
        QVERIFY(session->applySnapshot(fixture()));
        QVERIFY(session->power().isEmpty());
    }

    // Guest passes: pairing.guest/pass_expires_at_ms and devices[].guest/
    // expires_at_ms are accepted; guest with another permission is rejected.
    void guestPassFieldsAcceptedAndChecked()
    {
        SessionModel *session = SessionModel::instance();
        const int epoch = session->contextEpoch();
        QJsonObject snap = fixture();
        snap.insert(QStringLiteral("context_epoch"), epoch + 400);
        QJsonObject pairing = snap.value(QStringLiteral("pairing")).toObject();
        pairing.insert(QStringLiteral("guest"), true);
        pairing.insert(QStringLiteral("pass_expires_at_ms"), 1790647200000.0);
        snap.insert(QStringLiteral("pairing"), pairing);
        QJsonObject guest{{QStringLiteral("id"), QStringLiteral("dev_g")}, {QStringLiteral("name"), QStringLiteral("Guest phone")},
                          {QStringLiteral("permissions"), QJsonArray{QStringLiteral("guest")}}, {QStringLiteral("connected"), true},
                          {QStringLiteral("last_seen_ms"), 1000}, {QStringLiteral("created_at"), QStringLiteral("2026-09-28T19:30:00Z")},
                          {QStringLiteral("guest"), true}, {QStringLiteral("expires_at_ms"), 1790647200000.0}};
        QJsonArray devices = snap.value(QStringLiteral("devices")).toArray();
        devices.append(guest);
        snap.insert(QStringLiteral("devices"), devices);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QCOMPARE(session->contextEpoch(), epoch + 400);
        QVERIFY(session->pairing().value(QStringLiteral("guest")).toBool());
        QCOMPARE(session->devices().last().toMap().value(QStringLiteral("guest")).toBool(), true);

        snap.insert(QStringLiteral("context_epoch"), epoch + 500);
        guest.insert(QStringLiteral("permissions"), QJsonArray{QStringLiteral("guest"), QStringLiteral("controller")});
        devices.removeLast();
        devices.append(guest);
        snap.insert(QStringLiteral("devices"), devices);
        QVERIFY(!session->applySnapshot(snap));
        QVERIFY2(session->lastError().contains(QStringLiteral("guest")), qPrintable(session->lastError()));
        QCOMPARE(session->contextEpoch(), epoch + 400); // the previous state stays
        QVERIFY(session->applySnapshot(fixture()));
    }

    // Pair a phone: "Who is it for?" starts on Family phone (pair.issue without
    // pass); ◀ ▶ re-issues as a guest pass (pass tonight/24h/7d). Paired phones
    // shows a guest with its badge and the time it ends.
    void pairScreenOffersGuestPasses()
    {
        SessionModel *session = SessionModel::instance();
        IpcClient *ipc = ShellController::instance()->ipc();
        auto lastIssue = [ipc]() {
            const QList<QJsonObject> sent = ipc->sentMessages();
            for (auto it = sent.crbegin(); it != sent.crend(); ++it)
                if (it->value(QStringLiteral("type")).toString() == QLatin1String("pair.issue"))
                    return *it;
            return QJsonObject{};
        };
        auto kindRow = [this]() { return m_window->findChild<QObject *>(QStringLiteral("pairKindRow")); };

        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("nav.right"));
        ipc->clearSent();
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("pairing"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("pair-kind"));
        QVERIFY(!lastIssue().isEmpty());
        QVERIFY(!lastIssue().contains(QStringLiteral("pass"))); // a family phone by default
        QVERIFY(kindRow());
        QCOMPARE(kindRow()->property("value").toString(), QStringLiteral("Family phone"));

        act(QStringLiteral("nav.right"));
        QCOMPARE(lastIssue().value(QStringLiteral("pass")).toString(), QStringLiteral("tonight"));
        act(QStringLiteral("nav.right"));
        QCOMPARE(lastIssue().value(QStringLiteral("pass")).toString(), QStringLiteral("24h"));
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("nav.right")); // stops at the last choice
        QCOMPARE(lastIssue().value(QStringLiteral("pass")).toString(), QStringLiteral("7d"));
        act(QStringLiteral("nav.left"));
        act(QStringLiteral("nav.left"));
        QCOMPARE(lastIssue().value(QStringLiteral("pass")).toString(), QStringLiteral("tonight"));
        QVERIFY(kindRow()->property("value").toString().contains(QStringLiteral("Tonight")));

        // The coordinator answers with a live guest invitation.
        QJsonObject snap = fixture();
        // "Tonight": 04:00 the next morning, local time (the coordinator's PassEnd).
        const QDateTime nowLocal = QDateTime::currentDateTime();
        const double ends = double(QDateTime(nowLocal.date().addDays(nowLocal.time().hour() >= 4 ? 1 : 0), QTime(4, 0)).toMSecsSinceEpoch());
        QFile pf(QStringLiteral(BDTV_FIXTURE_DIR "/pairing.guest-demo.json")); // DEMO invitation with a real QR
        QVERIFY(pf.open(QIODevice::ReadOnly));
        QJsonObject pairing = QJsonDocument::fromJson(pf.readAll()).object();
        pairing.insert(QStringLiteral("pass_expires_at_ms"), ends);
        snap.insert(QStringLiteral("pairing"), pairing);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QCoreApplication::processEvents();
        QVERIFY2(kindRow()->property("description").toString().contains(QStringLiteral("ends ")), qPrintable(kindRow()->property("description").toString()));
        shot(QStringLiteral("pairing-guest"));
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("new-code"));
        act(QStringLiteral("select")); // a new code keeps the chosen kind
        QCOMPARE(lastIssue().value(QStringLiteral("pass")).toString(), QStringLiteral("tonight"));
        act(QStringLiteral("back"));
        act(QStringLiteral("back"));

        // Paired phones: a guest row with badge and end.
        QJsonArray devices = snap.value(QStringLiteral("devices")).toArray();
        devices.append(QJsonObject{{QStringLiteral("id"), QStringLiteral("dev_g3")}, {QStringLiteral("name"), QStringLiteral("DEMO visitor's phone")},
                                   {QStringLiteral("permissions"), QJsonArray{QStringLiteral("guest")}}, {QStringLiteral("connected"), true},
                                   {QStringLiteral("last_seen_ms"), 119000}, {QStringLiteral("created_at"), QStringLiteral("2026-09-28T19:30:00Z")},
                                   {QStringLiteral("guest"), true}, {QStringLiteral("expires_at_ms"), ends}});
        snap.insert(QStringLiteral("devices"), devices);
        snap.insert(QStringLiteral("pairing"), fixture().value(QStringLiteral("pairing")));
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select"));
        for (int i = 0; i < 20; ++i)
            act(QStringLiteral("nav.up"));
        act(QStringLiteral("nav.down"));
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("devices"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("devices"));
        act(QStringLiteral("nav.down"));
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("dev_g3"));
        QString guestRow;
        std::function<void(QQuickItem *)> find = [&](QQuickItem *item) {
            if (item->property("badge").toString() == QLatin1String("Guest") && item->isVisible())
                guestRow = item->property("description").toString();
            for (QQuickItem *child : item->childItems())
                find(child);
        };
        find(m_window->contentItem());
        QVERIFY2(guestRow.contains(QStringLiteral("Guest pass · ends 04:00")) && guestRow.contains(QStringLiteral(" left)")), qPrintable(guestRow));
        shot(QStringLiteral("devices-guest"));
        QVERIFY(session->applySnapshot(fixture()));
        goHome();
    }

    // state.plex (shell only): a well-formed sign-in state is accepted and
    // exposed as Session.plex; a bad status or library kind is rejected and
    // the previous state stays.
    void plexStateAcceptedAndChecked()
    {
        SessionModel *session = SessionModel::instance();
        const int epoch = session->contextEpoch();
        QJsonObject snap = fixture();
        snap.insert(QStringLiteral("context_epoch"), epoch + 210);
        QJsonObject lib{{QStringLiteral("id"), QStringLiteral("1")}, {QStringLiteral("title"), QStringLiteral("DEMO Movies")},
                        {QStringLiteral("kind"), QStringLiteral("movie")}, {QStringLiteral("selected"), true}};
        QJsonObject plex{{QStringLiteral("status"), QStringLiteral("linking")}, {QStringLiteral("message"), QString()},
                         {QStringLiteral("code"), QStringLiteral("D4K9")}, {QStringLiteral("link_url"), QStringLiteral("https://plex.tv/link")},
                         {QStringLiteral("server"), QJsonValue::Null}, {QStringLiteral("servers"), QJsonArray{}},
                         {QStringLiteral("libraries"), QJsonArray{lib}}};
        snap.insert(QStringLiteral("plex"), plex);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QCOMPARE(session->plex().value(QStringLiteral("code")).toString(), QStringLiteral("D4K9"));

        snap.insert(QStringLiteral("context_epoch"), epoch + 310);
        plex.insert(QStringLiteral("status"), QStringLiteral("linked"));
        snap.insert(QStringLiteral("plex"), plex);
        QVERIFY(!session->applySnapshot(snap));
        QVERIFY2(session->lastError().contains(QStringLiteral("state.plex")), qPrintable(session->lastError()));
        plex.insert(QStringLiteral("status"), QStringLiteral("choose_libraries"));
        lib.insert(QStringLiteral("kind"), QStringLiteral("podcast"));
        plex.insert(QStringLiteral("libraries"), QJsonArray{lib});
        snap.insert(QStringLiteral("plex"), plex);
        QVERIFY(!session->applySnapshot(snap));
        QCOMPARE(session->contextEpoch(), epoch + 210); // the previous state stays
        QVERIFY(session->applySnapshot(fixture()));
        QVERIFY(session->plex().isEmpty());
    }

    // Settings → Plex: the row opens the screen, and each state.plex status
    // offers the right step and sends the right plex.* message (ipc.md).
    void plexScreenDrivesSignIn()
    {
        SessionModel *session = SessionModel::instance();
        IpcClient *ipc = ShellController::instance()->ipc();
        auto last = [ipc](const QString &type) {
            const QList<QJsonObject> sent = ipc->sentMessages();
            for (auto it = sent.crbegin(); it != sent.crend(); ++it)
                if (it->value(QStringLiteral("type")).toString() == type)
                    return *it;
            return QJsonObject{};
        };
        auto count = [ipc](const QString &type) {
            int n = 0;
            for (const QJsonObject &m : ipc->sentMessages())
                n += m.value(QStringLiteral("type")).toString() == type;
            return n;
        };
        int epoch = session->contextEpoch() + 400;
        auto withPlex = [&](const QJsonObject &plex) {
            QJsonObject snap = fixture();
            snap.insert(QStringLiteral("context_epoch"), ++epoch);
            snap.insert(QStringLiteral("plex"), plex);
            QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
            QCoreApplication::processEvents();
        };
        auto plexState = [](const QString &status) {
            return QJsonObject{{QStringLiteral("status"), status}, {QStringLiteral("message"), QString()},
                               {QStringLiteral("code"), QJsonValue::Null}, {QStringLiteral("link_url"), QJsonValue::Null},
                               {QStringLiteral("server"), QJsonValue::Null}, {QStringLiteral("servers"), QJsonArray{}},
                               {QStringLiteral("libraries"), QJsonArray{}}};
        };
        const auto restore = qScopeGuard([&] { QVERIFY(session->applySnapshot(fixture())); goHome(); });
        withPlex(plexState(QStringLiteral("signed_out")));

        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        for (int i = 0; i < 25; ++i)
            act(QStringLiteral("nav.down"));
        act(QStringLiteral("nav.up")); // the last row is Exit; Plex sits just above it
        QCOMPARE(m_nav->itemId(), QStringLiteral("plex"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings")); // the contract's name for it
        QCOMPARE(m_nav->sectionId(), QStringLiteral("plex"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("sign-in"));
        ipc->clearSent();

        act(QStringLiteral("select"));
        QCOMPARE(count(QStringLiteral("plex.sign_in")), 1);

        QJsonObject linking = plexState(QStringLiteral("linking"));
        linking.insert(QStringLiteral("code"), QStringLiteral("D4K9"));
        linking.insert(QStringLiteral("link_url"), QStringLiteral("https://plex.tv/link"));
        linking.insert(QStringLiteral("qr_modules"), QJsonArray{QStringLiteral("111"), QStringLiteral("101"), QStringLiteral("111")});
        withPlex(linking);
        QCOMPARE(m_nav->itemId(), QStringLiteral("cancel"));
        QObject *linkingView = m_window->findChild<QObject *>(QStringLiteral("plexLinking"));
        QVERIFY(linkingView && linkingView->property("visible").toBool());
        shot(QStringLiteral("plex-linking"));
        act(QStringLiteral("select"));
        QCOMPARE(count(QStringLiteral("plex.cancel")), 1);

        QJsonObject servers = plexState(QStringLiteral("choose_server"));
        servers.insert(QStringLiteral("servers"), QJsonArray{
            QJsonObject{{QStringLiteral("id"), QStringLiteral("srv-a")}, {QStringLiteral("name"), QStringLiteral("DEMO A")}, {QStringLiteral("owned"), true}, {QStringLiteral("local"), true}},
            QJsonObject{{QStringLiteral("id"), QStringLiteral("srv-b")}, {QStringLiteral("name"), QStringLiteral("DEMO B")}, {QStringLiteral("owned"), false}, {QStringLiteral("local"), false}}});
        withPlex(servers);
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("server-srv-b"));
        act(QStringLiteral("select"));
        QCOMPARE(last(QStringLiteral("plex.choose_server")).value(QStringLiteral("server_id")).toString(), QStringLiteral("srv-b"));

        auto lib = [](const QString &id, const QString &kind, bool selected) {
            return QJsonObject{{QStringLiteral("id"), id}, {QStringLiteral("title"), QStringLiteral("DEMO ") + id},
                               {QStringLiteral("kind"), kind}, {QStringLiteral("selected"), selected}};
        };
        QJsonObject libs = plexState(QStringLiteral("choose_libraries"));
        libs.insert(QStringLiteral("server"), QStringLiteral("DEMO B"));
        libs.insert(QStringLiteral("libraries"), QJsonArray{lib(QStringLiteral("1"), QStringLiteral("movie"), true),
                                                            lib(QStringLiteral("3"), QStringLiteral("artist"), false),
                                                            lib(QStringLiteral("2"), QStringLiteral("show"), true)});
        withPlex(libs);
        QCOMPARE(m_nav->itemId(), QStringLiteral("library-1"));
        act(QStringLiteral("select")); // untick movies
        act(QStringLiteral("nav.down"));
        act(QStringLiteral("select")); // tick music
        act(QStringLiteral("nav.down"));
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("done"));
        act(QStringLiteral("select"));
        QCOMPARE(last(QStringLiteral("plex.choose_libraries")).value(QStringLiteral("library_ids")).toArray(),
                 (QJsonArray{QStringLiteral("3"), QStringLiteral("2")}));

        // Back in the middle of the flow cancels it.
        const int cancels = count(QStringLiteral("plex.cancel"));
        act(QStringLiteral("back"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("plex")); // back on the Settings row
        QCOMPARE(count(QStringLiteral("plex.cancel")), cancels + 1);

        QJsonObject connected = plexState(QStringLiteral("connected"));
        connected.insert(QStringLiteral("server"), QStringLiteral("DEMO B"));
        withPlex(connected);
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("sign-out"));
        act(QStringLiteral("select")); // confirmation, focus on the safe choice
        QCOMPARE(m_nav->screen(), QStringLiteral("dialog"));
        act(QStringLiteral("nav.left"));
        act(QStringLiteral("select"));
        QCOMPARE(count(QStringLiteral("plex.sign_out")), 1);
        QCOMPARE(count(QStringLiteral("plex.cancel")), cancels + 1); // leaving a finished sign-in cancels nothing
    }

    // Home: Plex posters come from the local artwork cache, pixelated in the
    // Pixel art style (decoded once at 1/World.px size, drawn without
    // smoothing) and smooth in Classic; empty rows that fail say why.
    void plexRowsPixelatePostersAndExplainFailures()
    {
        SessionModel *session = SessionModel::instance();
        QTemporaryDir dir;
        QVERIFY(dir.isValid());
        const QString poster = dir.filePath(QStringLiteral("poster.png"));
        QImage img(200, 300, QImage::Format_RGB32);
        img.fill(QColor(80, 60, 160));
        QVERIFY(img.save(poster));
        QJsonObject snap = fixture();
        snap.insert(QStringLiteral("context_epoch"), session->contextEpoch() + 500);
        QJsonObject content = snap.value(QStringLiteral("content")).toObject();
        QJsonArray sections = content.value(QStringLiteral("sections")).toArray();
        QJsonObject first = sections.at(0).toObject();
        QJsonArray items = first.value(QStringLiteral("items")).toArray();
        QJsonObject item = items.at(0).toObject();
        item.insert(QStringLiteral("artwork"), poster);
        item.insert(QStringLiteral("demo"), false);
        items.replace(0, item);
        first.insert(QStringLiteral("items"), items);
        sections.replace(0, first);
        content.insert(QStringLiteral("sections"), sections);
        snap.insert(QStringLiteral("content"), content);
        const auto restore = qScopeGuard([&] { QVERIFY(session->applySnapshot(fixture())); });

        auto artFor = [this]() {
            QList<QQuickItem *> found;
            std::function<void(QQuickItem *)> collect = [&](QQuickItem *it) {
                if (it->objectName() == QLatin1String("contentArt") && it->property("status").toInt() == 1 /* Image.Ready */)
                    found.append(it);
                for (QQuickItem *child : it->childItems())
                    collect(child);
            };
            collect(m_window->contentItem());
            return found;
        };
        for (const bool classic : {false, true}) {
            QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
            QJsonObject ui = layout.value(QStringLiteral("ui")).toObject();
            ui.insert(QStringLiteral("art_style"), classic ? QStringLiteral("classic") : QStringLiteral("pixel"));
            layout.insert(QStringLiteral("ui"), ui);
            snap.insert(QStringLiteral("layout"), layout);
            snap.insert(QStringLiteral("context_epoch"), snap.value(QStringLiteral("context_epoch")).toInt() + 1);
            QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
            QList<QQuickItem *> arts;
            QTRY_VERIFY_WITH_TIMEOUT(!(arts = artFor()).isEmpty(), 3000);
            QQuickItem *art = arts.first();
            const QSize source = art->property("sourceSize").toSize();
            const int px = std::max(1, int(std::lround(4 * Theme::instance()->scale())));
            QQuickItem *hero = m_window->findChild<QQuickItem *>(QStringLiteral("heroBackdrop"));
            QVERIFY(hero);
            QCOMPARE(hero->property("pixelSize").toInt(), classic ? 1 : px);
            if (classic) {
                QVERIFY(art->property("smooth").toBool());
                QVERIFY2(source.width() >= int(art->width()) - 1, qPrintable(QStringLiteral("classic decodes at %1 for %2").arg(source.width()).arg(art->width())));
            } else {
                QVERIFY(!art->property("smooth").toBool());
                QCOMPARE(source.width(), int(std::ceil(art->width() / px)));
            }
        }

        // The server is unreachable: the empty rows stay, with the reason.
        QJsonObject failing = fixture();
        failing.insert(QStringLiteral("context_epoch"), session->contextEpoch() + 1);
        QJsonObject bad = failing.value(QStringLiteral("content")).toObject();
        bad.insert(QStringLiteral("status"), QStringLiteral("error"));
        bad.insert(QStringLiteral("message"), QStringLiteral("Can't reach your Plex server"));
        QJsonArray empty;
        for (const QJsonValue &v : bad.value(QStringLiteral("sections")).toArray())
            empty.append(QJsonObject{{QStringLiteral("section_id"), v.toObject().value(QStringLiteral("section_id"))}, {QStringLiteral("items"), QJsonArray{}}});
        bad.insert(QStringLiteral("sections"), empty);
        failing.insert(QStringLiteral("content"), bad);
        QVERIFY2(session->applySnapshot(failing), qPrintable(session->lastError()));
        SectionsModel *model = session->sections();
        const int row = model->indexOfSection(QStringLiteral("plex-continue"));
        QVERIFY2(row >= 0, "a failing Plex row with hide_when_empty must still show");
        ItemsModel *cw = model->itemsFor(QStringLiteral("plex-continue"));
        QVERIFY(cw && cw->rowCount() == 1);
        QCOMPARE(cw->get(0).value(QStringLiteral("subtitle")).toString(), QStringLiteral("Can't reach your Plex server"));
    }

    // Settings → Now playing on phones: a toggle showing state.remote.now_playing
    // (missing means on), OK sends remote.now_playing with the opposite value.
    void nowPlayingRowTogglesTheSetting()
    {
        SessionModel *session = SessionModel::instance();
        auto focusedValue = [this]() {
            QString value;
            std::function<void(QQuickItem *)> find = [&](QQuickItem *item) {
                if (!item->isVisible() || item->opacity() == 0)
                    return;
                if (item->objectName() == QLatin1String("settingsRow") && item->property("focused").toBool())
                    value = item->property("value").toString();
                for (QQuickItem *child : item->childItems())
                    find(child);
            };
            find(m_window->contentItem());
            return value;
        };
        IpcClient *ipc = ShellController::instance()->ipc();
        auto lastToggle = [ipc]() {
            const QList<QJsonObject> sent = ipc->sentMessages();
            for (auto it = sent.crbegin(); it != sent.crend(); ++it)
                if (it->value(QStringLiteral("type")).toString() == QLatin1String("remote.now_playing"))
                    return *it;
            return QJsonObject{};
        };

        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        for (int i = 0; i < 20; ++i)
            act(QStringLiteral("nav.up"));
        for (int i = 0; i < 3; ++i) // remote, pairing, devices
            act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("now-playing"));
        QCOMPARE(focusedValue(), QStringLiteral("on")); // the demo fixture has no remote.now_playing: on
        shot(QStringLiteral("settings-now-playing"));

        ipc->clearSent();
        act(QStringLiteral("select"));
        QCOMPARE(lastToggle().value(QStringLiteral("enabled")), QJsonValue(false));
        QVERIFY(!lastToggle().value(QStringLiteral("request_id")).toString().isEmpty());

        QJsonObject snap = fixture();
        QJsonObject remote = snap.value(QStringLiteral("remote")).toObject();
        remote.insert(QStringLiteral("now_playing"), false);
        snap.insert(QStringLiteral("remote"), remote);
        QVERIFY2(session->applySnapshot(snap), qPrintable(session->lastError()));
        QCoreApplication::processEvents();
        QCOMPARE(focusedValue(), QStringLiteral("off"));
        act(QStringLiteral("select"));
        QCOMPARE(lastToggle().value(QStringLiteral("enabled")), QJsonValue(true));

        QVERIFY(session->applySnapshot(fixture()));
        goHome();
    }

    // Settings → Sleep timer (◀ ▶ over Off, 15 … 120 min, sending
    // power.sleep_timer) and Turn the screen off (display.off after a short
    // pause, only while the capability is available).
    void sleepRowSetsTheTimerAndScreenOff()
    {
        SessionModel *session = SessionModel::instance();
        IpcClient *ipc = ShellController::instance()->ipc();
        auto focusedValue = [this]() {
            QString value;
            std::function<void(QQuickItem *)> find = [&](QQuickItem *item) {
                if (!item->isVisible() || item->opacity() == 0)
                    return;
                if (item->objectName() == QLatin1String("settingsRow") && item->property("focused").toBool())
                    value = item->property("value").toString();
                for (QQuickItem *child : item->childItems())
                    find(child);
            };
            find(m_window->contentItem());
            return value;
        };
        auto lastRequest = [ipc](const QString &action) {
            const QList<QJsonObject> sent = ipc->sentMessages();
            for (auto it = sent.crbegin(); it != sent.crend(); ++it)
                if (it->value(QStringLiteral("type")).toString() == QLatin1String("request")
                    && it->value(QStringLiteral("action")).toString() == action)
                    return *it;
            return QJsonObject{};
        };
        auto withPower = [this](const QJsonObject &power, bool screenOff) {
            QJsonObject snap = fixture();
            snap.insert(QStringLiteral("power"), power);
            QJsonObject caps = snap.value(QStringLiteral("capabilities")).toObject();
            caps.insert(QStringLiteral("display.off"), screenOff ? QJsonObject{{QStringLiteral("available"), true}, {QStringLiteral("backend"), QStringLiteral("x11-dpms")}}
                                                                 : QJsonObject{{QStringLiteral("available"), false}, {QStringLiteral("reason"), QStringLiteral("The screen cannot be turned off here: no DPMS")}});
            snap.insert(QStringLiteral("capabilities"), caps);
            return snap;
        };
        const QJsonObject noTimer{{QStringLiteral("sleep_at_ms"), QJsonValue::Null}, {QStringLiteral("warning"), false}, {QStringLiteral("display"), QStringLiteral("on")}};

        QVERIFY2(session->applySnapshot(withPower(noTimer, true)), qPrintable(session->lastError()));
        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        for (int i = 0; i < 25; ++i)
            act(QStringLiteral("nav.up"));
        for (int i = 0; i < 18; ++i) // remote … advanced playback, diagnostics
            act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("sleep"));
        QCOMPARE(focusedValue(), QStringLiteral("Off"));

        ipc->clearSent();
        act(QStringLiteral("nav.right"));
        QCOMPARE(lastRequest(QStringLiteral("power.sleep_timer")).value(QStringLiteral("args")).toObject().value(QStringLiteral("minutes")).toInt(), 15);
        QJsonObject timer{{QStringLiteral("sleep_at_ms"), 2700000}, {QStringLiteral("sleep_minutes"), 45}, {QStringLiteral("warning"), false}, {QStringLiteral("display"), QStringLiteral("on")}};
        QVERIFY2(session->applySnapshot(withPower(timer, true)), qPrintable(session->lastError()));
        QTRY_COMPARE(focusedValue(), QStringLiteral("45 min"));
        shot(QStringLiteral("settings-sleep"));
        act(QStringLiteral("nav.right"));
        QCOMPARE(lastRequest(QStringLiteral("power.sleep_timer")).value(QStringLiteral("args")).toObject().value(QStringLiteral("minutes")).toInt(), 60);
        timer.insert(QStringLiteral("sleep_minutes"), 15);
        QVERIFY(session->applySnapshot(withPower(timer, true)));
        QCoreApplication::processEvents();
        act(QStringLiteral("nav.left"));
        const QJsonObject cancel = lastRequest(QStringLiteral("power.sleep_timer"));
        QVERIFY(cancel.value(QStringLiteral("args")).toObject().contains(QStringLiteral("minutes")));
        QCOMPARE(cancel.value(QStringLiteral("args")).toObject().value(QStringLiteral("minutes")).toInt(), 0);

        // Screen off: sent only after the OK key is let go, and only when available.
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("screen-off"));
        ipc->clearSent();
        act(QStringLiteral("select"));
        QVERIFY(lastRequest(QStringLiteral("display.off")).isEmpty()); // not at once
        QTRY_VERIFY_WITH_TIMEOUT(!lastRequest(QStringLiteral("display.off")).isEmpty(), 3000);
        QVERIFY(session->applySnapshot(withPower(noTimer, false)));
        QCoreApplication::processEvents();
        ipc->clearSent();
        act(QStringLiteral("select"));
        QTest::qWait(1200);
        QVERIFY(lastRequest(QStringLiteral("display.off")).isEmpty());

        QVERIFY(session->applySnapshot(fixture()));
        goHome();
    }

    // The sleep timer's last minute shows the dozing-cub card, and while it
    // shows (or while the display is off) a key does nothing but send
    // power.activity; afterwards keys work again.
    void sleepWarningSwallowsKeysAndShowsTheCard()
    {
        SessionModel *session = SessionModel::instance();
        IpcClient *ipc = ShellController::instance()->ipc();
        auto activities = [ipc]() {
            int n = 0;
            for (const QJsonObject &m : ipc->sentMessages())
                if (m.value(QStringLiteral("type")).toString() == QLatin1String("power.activity"))
                    ++n;
            return n;
        };
        auto withPower = [this](bool warning, const QString &display) {
            QJsonObject snap = fixture();
            snap.insert(QStringLiteral("power"), QJsonObject{{QStringLiteral("sleep_at_ms"), warning ? QJsonValue(2700000) : QJsonValue()},
                                                            {QStringLiteral("warning"), warning}, {QStringLiteral("display"), display}});
            return snap;
        };
        QQuickItem *card = nullptr;
        std::function<void(QQuickItem *)> find = [&](QQuickItem *item) {
            if (item->objectName() == QLatin1String("sleepWarning"))
                card = item;
            for (QQuickItem *child : item->childItems())
                find(child);
        };
        find(m_window->contentItem());
        QVERIFY(card);

        goHome();
        toFavorites();
        for (int i = 0; i < 8; ++i) // the first favourite, so "right" can move
            act(QStringLiteral("nav.left"));
        const QString before = m_nav->itemId();
        QVERIFY(!card->isVisible());
        QVERIFY2(session->applySnapshot(withPower(true, QStringLiteral("on"))), qPrintable(session->lastError()));
        QTRY_VERIFY(card->isVisible() && card->opacity() > 0.99);
        shot(QStringLiteral("sleep-warning"));
        ipc->clearSent();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->itemId(), before); // swallowed: no move, no launch
        QCOMPARE(m_nav->screen(), QStringLiteral("home"));
        QVERIFY(ShellController::instance()->launchingAppId().isEmpty());
        QCOMPARE(activities(), 2);

        // Display off: the card is gone (nothing to see) and keys still only wake.
        QVERIFY(session->applySnapshot(withPower(false, QStringLiteral("off"))));
        QTRY_VERIFY(!card->isVisible());
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), before);
        QCOMPARE(activities(), 3);

        // Awake again: keys move focus and send nothing.
        QVERIFY(session->applySnapshot(withPower(false, QStringLiteral("on"))));
        QCoreApplication::processEvents();
        act(QStringLiteral("nav.right"));
        QVERIFY(m_nav->itemId() != before);
        QCOMPARE(activities(), 3);
        QVERIFY(session->applySnapshot(fixture()));
        goHome();
    }

    void weatherScreenSearchesAndConfigures()
    {
        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        for (int i = 0; i < 20; ++i)
            act(QStringLiteral("nav.up"));
        for (int i = 0; i < 14; ++i) // remote … now playing, … art style, … hero, clock, weather
            act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("weather"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings")); // the contract's name for it
        QCOMPARE(m_nav->sectionId(), QStringLiteral("weather"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("enabled"));
        QVERIFY(!m_nav->textFieldFocused());

        IpcClient *ipc = ShellController::instance()->ipc();
        auto last = [ipc](const QString &type) {
            const QList<QJsonObject> sent = ipc->sentMessages();
            for (auto it = sent.crbegin(); it != sent.crend(); ++it)
                if (it->value(QStringLiteral("type")).toString() == type)
                    return *it;
            return QJsonObject{};
        };
        auto countOf = [ipc](const QString &type) {
            int n = 0;
            for (const QJsonObject &m : ipc->sentMessages())
                n += m.value(QStringLiteral("type")).toString() == type;
            return n;
        };
        ipc->clearSent();

        // Focus reports carry text_field, and a new one follows when it changes.
        QSignalSpy reports(m_nav, &Navigator::focusReported);
        for (int i = 0; i < 3; ++i)
            act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("search"));
        QVERIFY(m_nav->textFieldFocused());
        QVERIFY(!reports.isEmpty());
        QCOMPARE(reports.constLast().at(4).toBool(), true);
        ipc->sendFocus(m_nav->screen(), m_nav->sectionId(), m_nav->itemId(), 0, m_nav->textFieldFocused());
        QCOMPARE(last(QStringLiteral("focus")).value(QStringLiteral("text_field")), QJsonValue(true));

        // Only the field's focus changes (same row): a new report still follows.
        QObject *field = m_window->findChild<QObject *>(QStringLiteral("weatherSearchField"));
        QVERIFY(field);
        const qsizetype before = reports.size();
        field->setProperty("focus", false);
        QCoreApplication::processEvents();
        QCOMPARE(reports.size(), before + 1);
        QCOMPARE(reports.constLast().at(4).toBool(), false);
        QMetaObject::invokeMethod(field, "forceActiveFocus");
        QCoreApplication::processEvents();
        QCOMPARE(reports.constLast().at(4).toBool(), true);

        // The phone keyboard: text.submit fills the field and presses Return.
        const QVariantMap r = m_nav->apply(QStringLiteral("text.submit"), {{QStringLiteral("text"), QStringLiteral("Zagreb")}});
        QCoreApplication::processEvents();
        QCOMPARE(r.value(QStringLiteral("outcome")).toString(), QStringLiteral("observed"));
        QCOMPARE(countOf(QStringLiteral("weather.search")), 1);
        const QJsonObject search = last(QStringLiteral("weather.search"));
        QCOMPARE(search.value(QStringLiteral("query")).toString(), QStringLiteral("Zagreb"));
        const QString requestId = search.value(QStringLiteral("request_id")).toString();
        QVERIFY(!requestId.isEmpty());
        QVERIFY(ShellController::instance()->weatherSearching());

        // A physical Return in the field searches again.
        act(QStringLiteral("select"));
        QCOMPARE(countOf(QStringLiteral("weather.search")), 2);
        const QString requestId2 = last(QStringLiteral("weather.search")).value(QStringLiteral("request_id")).toString();

        // The coordinator answers with places; the first place is one press away.
        const QJsonObject places{
            {QStringLiteral("type"), QStringLiteral("weather_places")},
            {QStringLiteral("request_id"), requestId2},
            {QStringLiteral("ok"), true},
            {QStringLiteral("error"), QString()},
            {QStringLiteral("places"), QJsonArray{
                QJsonObject{{QStringLiteral("name"), QStringLiteral("Zagreb")}, {QStringLiteral("region"), QStringLiteral("City of Zagreb")},
                            {QStringLiteral("country"), QStringLiteral("Croatia")}, {QStringLiteral("latitude"), 45.81}, {QStringLiteral("longitude"), 15.98}},
                QJsonObject{{QStringLiteral("name"), QStringLiteral("Zagreb Hill")}, {QStringLiteral("region"), QString()},
                            {QStringLiteral("country"), QStringLiteral("Demo")}, {QStringLiteral("latitude"), 1.0}, {QStringLiteral("longitude"), 2.0}},
            }},
        };
        emit ipc->replyReceived(requestId2, QStringLiteral("weather.search"), places);
        QCoreApplication::processEvents();
        QCOMPARE(ShellController::instance()->weatherPlaces().size(), 2);
        QVERIFY(!ShellController::instance()->weatherSearching());
        shot(QStringLiteral("weather"));
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("place-0"));
        QVERIFY(!m_nav->textFieldFocused());
        QCOMPARE(reports.constLast().at(4).toBool(), false);
        act(QStringLiteral("select"));
        const QJsonObject configure = last(QStringLiteral("weather.configure"));
        QCOMPARE(configure.value(QStringLiteral("enabled")), QJsonValue(true));
        QCOMPARE(configure.value(QStringLiteral("units")).toString(), QStringLiteral("celsius"));
        QCOMPARE(configure.value(QStringLiteral("scene")), QJsonValue(true));
        const QJsonObject place = configure.value(QStringLiteral("place")).toObject();
        QCOMPARE(place.value(QStringLiteral("name")).toString(), QStringLiteral("Zagreb"));
        QCOMPARE(place.value(QStringLiteral("latitude")).toDouble(), 45.81);

        // Units: ◀ ▶ on its row; place null keeps the stored place.
        for (int i = 0; i < 10; ++i)
            act(QStringLiteral("nav.up"));
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("units"));
        act(QStringLiteral("nav.right"));
        const QJsonObject units = last(QStringLiteral("weather.configure"));
        QCOMPARE(units.value(QStringLiteral("units")).toString(), QStringLiteral("fahrenheit"));
        QVERIFY(units.contains(QStringLiteral("place")));
        QVERIFY(units.value(QStringLiteral("place")).isNull());
        QCOMPARE(units.value(QStringLiteral("enabled")), QJsonValue(true));
        act(QStringLiteral("nav.up"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("enabled"));
        act(QStringLiteral("select")); // Weather off, keeping the place
        const QJsonObject off = last(QStringLiteral("weather.configure"));
        QCOMPARE(off.value(QStringLiteral("enabled")), QJsonValue(false));
        QVERIFY(off.value(QStringLiteral("place")).isNull());

        act(QStringLiteral("back"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("weather"));
        QVERIFY(!m_nav->textFieldFocused());
        goHome();
    }

    // Settings → Advanced playback lists each app's settings from state.playback;
    // Left/Right sends playback.set with the next offered option, OK returns the
    // row to Auto (value "").
    void advancedPlaybackSendsPlaybackSet()
    {
        goHome();
        toHeader();
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        for (int i = 0; i < 20; ++i)
            act(QStringLiteral("nav.up"));
        for (int i = 0; i < 16; ++i) // remote … now playing, … art style, … clock, weather, playback, advanced playback
            act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("advanced-playback"));
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("diagnostics")); // the contract's name for these screens
        QCOMPARE(m_nav->sectionId(), QStringLiteral("playback"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("moonlight.codec"));
        shot(QStringLiteral("advanced-playback"));
        settingsRowsFitTheirList();

        IpcClient *ipc = ShellController::instance()->ipc();
        auto lastPlaybackSet = [ipc]() {
            const QList<QJsonObject> sent = ipc->sentMessages();
            for (auto it = sent.crbegin(); it != sent.crend(); ++it)
                if (it->value(QStringLiteral("type")).toString() == QLatin1String("playback.set"))
                    return *it;
            return QJsonObject{};
        };
        ipc->clearSent();
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("moonlight.fps"));
        act(QStringLiteral("nav.right")); // 60 fps (chosen by hand) → 120 fps
        QJsonObject m = lastPlaybackSet();
        QCOMPARE(m.value(QStringLiteral("adapter")).toString(), QStringLiteral("moonlight"));
        QCOMPARE(m.value(QStringLiteral("setting")).toString(), QStringLiteral("fps"));
        QCOMPARE(m.value(QStringLiteral("value")).toString(), QStringLiteral("120"));
        QVERIFY(!m.value(QStringLiteral("request_id")).toString().isEmpty());
        act(QStringLiteral("select")); // back to Auto
        QCOMPARE(lastPlaybackSet().value(QStringLiteral("value")).toString(), QString());
        QCOMPARE(lastPlaybackSet().value(QStringLiteral("setting")).toString(), QStringLiteral("fps"));

        ipc->clearSent();
        act(QStringLiteral("nav.up")); // codec: Auto (H.264) is the last offered option here
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("select")); // not chosen by hand: nothing to reset
        QVERIFY(lastPlaybackSet().isEmpty());
        for (int i = 0; i < 4; ++i) // fps, resolution, frame pacing → YouTube's first setting
            act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("vacuumtube.codecs"));
        act(QStringLiteral("back"));
        QCOMPARE(m_nav->screen(), QStringLiteral("settings"));
        goHome();
    }

    void uninstalledAppExplainsInsteadOfLaunching()
    {
        goHome();
        toFavorites();
        act(QStringLiteral("nav.right")); // YouTube is not installed in the fixture
        act(QStringLiteral("select"));
        QCOMPARE(m_nav->screen(), QStringLiteral("dialog"));
        QCOMPARE(ShellController::instance()->launchingAppId(), QString());
        shot(QStringLiteral("app-unavailable"));
        act(QStringLiteral("back"));
        QCOMPARE(m_nav->screen(), QStringLiteral("home"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("youtube"));
    }

    void homeClosesDialogsAndRestoresFocus()
    {
        goHome();
        toFavorites();
        act(QStringLiteral("nav.down"));
        act(QStringLiteral("nav.right"));
        const QString before = m_nav->itemId();
        act(QStringLiteral("select")); // DEMO item → message dialog
        QCOMPARE(m_nav->screen(), QStringLiteral("dialog"));
        act(QStringLiteral("home"));
        QCOMPARE(m_nav->screen(), QStringLiteral("home"));
        QCOMPARE(m_nav->itemId(), before);
    }

    void refreshReorderKeepsFocusedId()
    {
        goHome();
        toFavorites();
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("youtube"));
        QJsonObject snap = fixture();
        QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
        QJsonArray sections = layout.value(QStringLiteral("sections")).toArray();
        QJsonObject fav = sections.at(0).toObject();
        fav.insert(QStringLiteral("application_ids"), QJsonArray{QStringLiteral("youtube"), QStringLiteral("plex-htpc")});
        sections.replace(0, fav);
        layout.insert(QStringLiteral("sections"), sections);
        snap.insert(QStringLiteral("layout"), layout);
        snap.insert(QStringLiteral("context_epoch"), snap.value(QStringLiteral("context_epoch")).toInt() + 1);
        QVERIFY(SessionModel::instance()->applySnapshot(snap));
        QCoreApplication::processEvents();
        act(QStringLiteral("nav.left")); // youtube is now first: left must stay on it
        QCOMPARE(m_nav->itemId(), QStringLiteral("youtube"));
        QVERIFY(SessionModel::instance()->applySnapshot(fixture()));
    }

    // Optional apps (config hide_when_missing): a `hidden` app has no tile, a
    // present one does; a non-boolean `hidden` rejects the snapshot.
    void optionalAppHiddenWhenMissing()
    {
        const auto restore = qScopeGuard([&] { QVERIFY(SessionModel::instance()->applySnapshot(fixture())); });
        auto withOptional = [&](const QJsonValue &hidden) {
            QJsonObject snap = fixture();
            QJsonArray apps = snap.value(QStringLiteral("applications")).toArray();
            QJsonObject app = apps.at(0).toObject();
            for (const char *id : {"spotify", "retroarch"}) {
                app.insert(QStringLiteral("id"), QString::fromLatin1(id));
                app.insert(QStringLiteral("adapter"), QString::fromLatin1(id));
                app.insert(QStringLiteral("label"), QString::fromLatin1(id));
                app.remove(QStringLiteral("hidden"));
                if (qstrcmp(id, "spotify") == 0) {
                    app.insert(QStringLiteral("installed"), false);
                    app.insert(QStringLiteral("hidden"), hidden);
                } else {
                    app.insert(QStringLiteral("installed"), true);
                }
                apps.append(app);
            }
            snap.insert(QStringLiteral("applications"), apps);
            QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
            QJsonArray sections = layout.value(QStringLiteral("sections")).toArray();
            QJsonObject fav = sections.at(0).toObject();
            fav.insert(QStringLiteral("application_ids"), QJsonArray{QStringLiteral("plex-htpc"), QStringLiteral("youtube"), QStringLiteral("spotify"), QStringLiteral("retroarch")});
            sections.replace(0, fav);
            layout.insert(QStringLiteral("sections"), sections);
            snap.insert(QStringLiteral("layout"), layout);
            snap.insert(QStringLiteral("context_epoch"), snap.value(QStringLiteral("context_epoch")).toInt() + 1);
            return SessionModel::instance()->applySnapshot(snap);
        };
        QVERIFY(!withOptional(QStringLiteral("yes")));
        QVERIFY(withOptional(true));
        QCoreApplication::processEvents();
        goHome();
        toFavorites();
        for (int i = 0; i < 4; ++i)
            act(QStringLiteral("nav.left"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("plex-htpc"));
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("youtube"));
        act(QStringLiteral("nav.right")); // spotify is hidden: straight to retroarch
        QCOMPARE(m_nav->itemId(), QStringLiteral("retroarch"));
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("retroarch"));
        shot(QStringLiteral("optional-hidden"));
        QVERIFY(withOptional(false)); // installed now: its tile appears
        QCoreApplication::processEvents();
        goHome();
        toFavorites();
        for (int i = 0; i < 4; ++i)
            act(QStringLiteral("nav.left"));
        act(QStringLiteral("nav.right"));
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), QStringLiteral("spotify"));
        // Spotify's featured panel says how to play from a phone.
        QObject *hint = m_window->findChild<QObject *>(QStringLiteral("heroHint"));
        QVERIFY(hint);
        QTRY_VERIFY(hint->property("text").toString().contains(QStringLiteral("pick this TV in the device list")));
        QVERIFY(hint->property("visible").toBool());
    }

    // App icons (docs/THEMES.md → App icons): every adapter has Bear Den's own
    // icon in both art styles, and Shell.appArt picks the owner's brand folder
    // first, then that bundled icon, and only then the Flatpak's exported icon.
    void appArtResolutionOrder()
    {
        const QStringList adapters{QStringLiteral("plex-htpc"), QStringLiteral("vacuumtube"), QStringLiteral("moonlight"),
                                   QStringLiteral("spotify"), QStringLiteral("jellyfin"), QStringLiteral("retroarch")};
        ShellController *shell = ShellController::instance();
        for (const QString &a : adapters) {
            QVERIFY2(!shell->flatpakIdFor(a).isEmpty(), qPrintable(a));
            const QImage px(QStringLiteral(":/qt/qml/BearDen/assets/pixel/app-%1.png").arg(a));
            QVERIFY2(px.size() == QSize(32, 32), qPrintable(a + QStringLiteral(": pixel icon must be 32x32")));
            QVERIFY2(!QImage(QStringLiteral(":/qt/qml/BearDen/assets/classic/app-%1.svg").arg(a)).isNull(), qPrintable(a));
        }

        QTemporaryDir data, home;
        QVERIFY(data.isValid() && home.isValid());
        const QByteArray oldData = qgetenv("XDG_DATA_HOME"), oldHome = qgetenv("HOME");
        const auto restore = qScopeGuard([&] {
            qputenv("XDG_DATA_HOME", oldData);
            qputenv("HOME", oldHome);
            ShellController::instance()->forgetArt();
        });
        qputenv("XDG_DATA_HOME", data.path().toUtf8());
        qputenv("HOME", home.path().toUtf8());
        // The installed Flatpak exports an icon: ours still wins.
        const QString exported = home.filePath(QStringLiteral(".local/share/flatpak/exports/share/icons/hicolor/128x128/apps"));
        QVERIFY(QDir().mkpath(exported));
        QImage(8, 8, QImage::Format_ARGB32).save(exported + QStringLiteral("/tv.plex.PlexHTPC.png"));
        shell->forgetArt();
        QVariantMap art = shell->appArt(QStringLiteral("plex-htpc"));
        QCOMPARE(art.value(QStringLiteral("iconSource")).toString(), QStringLiteral("bundled"));
        QCOMPARE(art.value(QStringLiteral("icon")).toString(), QStringLiteral("qrc:/qt/qml/BearDen/assets/pixel/app-plex-htpc.png"));
        art = shell->appArt(QStringLiteral("plex-htpc"), true);
        QCOMPARE(art.value(QStringLiteral("icon")).toString(), QStringLiteral("qrc:/qt/qml/BearDen/assets/classic/app-plex-htpc.svg"));
        // An adapter without a bundled icon falls back to nothing here (no Flatpak id).
        QCOMPARE(shell->appArt(QStringLiteral("something-new")).value(QStringLiteral("iconSource")).toString(), QString());
        // The owner's brand folder beats everything.
        const QString brand = data.filePath(QStringLiteral("bear-den-tv/brand/plex-htpc"));
        QVERIFY(QDir().mkpath(brand));
        QImage(8, 8, QImage::Format_ARGB32).save(brand + QStringLiteral("/icon.png"));
        shell->forgetArt();
        for (bool classic : {false, true}) {
            art = shell->appArt(QStringLiteral("plex-htpc"), classic);
            QCOMPARE(art.value(QStringLiteral("iconSource")).toString(), QStringLiteral("brand"));
            QCOMPARE(art.value(QStringLiteral("icon")).toString(), QUrl::fromLocalFile(brand + QStringLiteral("/icon.png")).toString());
        }
        // AppIcon draws our pixel icon unsmoothed at a whole-number scale.
        shell->forgetArt();
        QQmlComponent c(m_engine);
        c.setData("import QtQuick\nimport BearDen\nAppIcon { adapter: \"spotify\"; label: \"Spotify\"; size: 70 }",
                  QUrl(QStringLiteral("qrc:/test/Icon.qml")));
        std::unique_ptr<QObject> icon(c.create());
        QVERIFY2(icon, qPrintable(c.errorString()));
        QVERIFY(icon->property("pixelArt").toBool());
        QCOMPARE(icon->property("drawn").toReal(), 64.0);
        QObject *image = icon->findChild<QObject *>(QStringLiteral("appIconImage"));
        QVERIFY(image && !image->property("smooth").toBool());
        QVERIFY(image->property("source").toUrl().toString().endsWith(QStringLiteral("/pixel/app-spotify.png")));
    }

    void lockedHidesEverything()
    {
        goHome();
        QJsonObject snap = fixture();
        QJsonObject session = snap.value(QStringLiteral("session")).toObject();
        session.insert(QStringLiteral("locked"), true);
        snap.insert(QStringLiteral("session"), session);
        snap.remove(QStringLiteral("devices"));
        snap.remove(QStringLiteral("content"));
        QVERIFY(SessionModel::instance()->applySnapshot(snap));
        QCoreApplication::processEvents();
        const QString item = m_nav->itemId();
        act(QStringLiteral("nav.right"));
        QCOMPARE(m_nav->itemId(), item); // no navigation while locked
        shot(QStringLiteral("locked"));
        QVERIFY(SessionModel::instance()->applySnapshot(fixture()));
    }

    void screensaverAfterIdleAndFirstPressOnlyWakes()
    {
        goHome();
        QObject *saver = m_engine->rootObjects().constFirst()->findChild<QObject *>(QStringLiteral("screensaver"));
        QVERIFY(saver);
        ShellController::instance()->setScreensaverSeconds(1);
        act(QStringLiteral("nav.left")); // input restarts the idle timer with the new interval
        const QString focused = m_nav->itemId();
        QTRY_VERIFY_WITH_TIMEOUT(saver->property("active").toBool(), 3000);
        shot(QStringLiteral("screensaver"));
        act(QStringLiteral("nav.right")); // wakes only
        QVERIFY(!saver->property("active").toBool());
        QCOMPARE(m_nav->itemId(), focused);
        ShellController::instance()->setScreensaverSeconds(300);
    }

    // Pixel-art wallpapers: sprites resolve to URLs; a bad sprite skips the theme.
    void themeWallpaperSprites()
    {
        QTemporaryDir root;
        QVERIFY(root.isValid());
        auto write = [&](const QString &rel, const QByteArray &data) {
            QDir(root.path()).mkpath(QFileInfo(rel).path());
            QFile f(root.filePath(rel));
            QVERIFY(f.open(QIODevice::WriteOnly));
            f.write(data);
        };
        const QByteArray manifest = R"({"schema":1,"id":"%1","name":"P","accent":"#123456","wallpaper":{"image":"bg.png","pixel":true,"top":"#000000","bottom":"#111111","sprites":[{"sheet":"fire.png","frames":4,"fps":%2,"x":10,"y":200}]}})";
        for (const auto &[id, fps] : {std::pair{QStringLiteral("pix"), 8}, std::pair{QStringLiteral("badfps"), 21}}) {
            write(id + QStringLiteral("/theme.json"), QString::fromUtf8(manifest).arg(id).arg(fps).toUtf8());
            write(id + QStringLiteral("/bg.png"), "png");
            write(id + QStringLiteral("/fire.png"), "png");
        }
        ThemeRegistry &reg = *ThemeRegistry::create(nullptr, nullptr); // the singleton; reloaded below
        reg.loadFrom({root.path()});
        const auto restore = qScopeGuard([&] { reg.reload(); });
        const QVariantMap wp = reg.get(QStringLiteral("pix")).value(QStringLiteral("wallpaper")).toMap();
        QCOMPARE(reg.get(QStringLiteral("pix")).value(QStringLiteral("id")).toString(), QStringLiteral("pix"));
        QVERIFY(wp.value(QStringLiteral("pixel")).toBool());
        const QVariantList sprites = wp.value(QStringLiteral("sprites")).toList();
        QCOMPARE(sprites.size(), 1);
        const QVariantMap sp = sprites.first().toMap();
        QVERIFY(sp.value(QStringLiteral("sheet")).toString().endsWith(QStringLiteral("/pix/fire.png")));
        QCOMPARE(sp.value(QStringLiteral("frames")).toInt(), 4);
        QCOMPARE(sp.value(QStringLiteral("fps")).toInt(), 8);
        QCOMPARE(sp.value(QStringLiteral("x")).toInt(), 10);
        QCOMPARE(sp.value(QStringLiteral("y")).toInt(), 200);
        QCOMPARE(reg.problems().size(), 1);
        QVERIFY2(reg.problems().first().contains(QStringLiteral("badfps")) && reg.problems().first().contains(QStringLiteral("fps")),
                 qPrintable(reg.problems().first()));
    }

    // Art style (layout.ui.art_style): a theme resolves in Pixel and Classic;
    // Classic takes its classic wallpaper and backdrop (drawn smooth) and SVG
    // ornaments first, Pixel the PNGs; a theme without classic art keeps its
    // own. The setting reaches Theme.artStyle, and a missing value means pixel.
    void artStyleResolvesThemes()
    {
        QTemporaryDir root;
        QVERIFY(root.isValid());
        auto write = [&](const QString &rel, const QByteArray &data) {
            QDir(root.path()).mkpath(QFileInfo(rel).path());
            QFile f(root.filePath(rel));
            QVERIFY(f.open(QIODevice::WriteOnly));
            f.write(data);
        };
        const QByteArray bothManifest = R"({"schema":1,"id":"both","name":"B","accent":"#123456","heading":"leaf","wallpaper":{"image":"bg.png","pixel":true,"top":"#000000","bottom":"#111111"},"phone":{"backdrop":"phone.png"},"classic":{"wallpaper":{"image":"bg.jpg","sprites":[{"sheet":"mist.png","frames":2,"fps":4,"x":0,"y":0}]},"phone":{"backdrop":"phone.jpg"}}})";
        const QByteArray oneManifest = R"({"schema":1,"id":"one","name":"O","accent":"#123456","wallpaper":{"image":"bg.png","pixel":true,"top":"#000000","bottom":"#111111"}})";
        const QByteArray badManifest = R"({"schema":1,"id":"bad","name":"X","accent":"#123456","wallpaper":{"top":"#000000","bottom":"#111111"},"classic":{"wallpaper":{"image":"gone.jpg"}}})";
        write(QStringLiteral("both/theme.json"), bothManifest);
        write(QStringLiteral("one/theme.json"), oneManifest);
        write(QStringLiteral("bad/theme.json"), badManifest);
        for (const char *f : {"both/bg.png", "both/bg.jpg", "both/mist.png", "both/phone.png", "both/phone.jpg", "both/leaf.png", "both/leaf.svg", "one/bg.png"})
            write(QString::fromLatin1(f), "img");
        ThemeRegistry &reg = *ThemeRegistry::create(nullptr, nullptr);
        reg.loadFrom({root.path()});
        const auto restore = qScopeGuard([&] { reg.reload(); });
        auto wall = [&](const char *id, const char *style) { return reg.get(QString::fromLatin1(id), QString::fromLatin1(style)).value(QStringLiteral("wallpaper")).toMap(); };
        auto backdrop = [&](const char *id, const char *style) {
            return reg.get(QString::fromLatin1(id), QString::fromLatin1(style)).value(QStringLiteral("phone")).toMap().value(QStringLiteral("backdrop")).toString();
        };
        QVERIFY(wall("both", "pixel").value(QStringLiteral("image")).toString().endsWith(QStringLiteral("/both/bg.png")));
        QVERIFY(wall("both", "pixel").value(QStringLiteral("pixel")).toBool());
        QVERIFY(wall("both", "pixel").value(QStringLiteral("sprites")).toList().isEmpty());
        QVERIFY(wall("both", "classic").value(QStringLiteral("image")).toString().endsWith(QStringLiteral("/both/bg.jpg")));
        QVERIFY(!wall("both", "classic").value(QStringLiteral("pixel")).toBool());
        QCOMPARE(wall("both", "classic").value(QStringLiteral("sprites")).toList().size(), 1);
        QVERIFY(backdrop("both", "pixel").endsWith(QStringLiteral("/both/phone.png")));
        QVERIFY(backdrop("both", "classic").endsWith(QStringLiteral("/both/phone.jpg")));
        QVERIFY(reg.get(QStringLiteral("both")).value(QStringLiteral("heading")).toString().endsWith(QStringLiteral("leaf.png")));
        QVERIFY(reg.get(QStringLiteral("both"), QStringLiteral("classic")).value(QStringLiteral("heading")).toString().endsWith(QStringLiteral("leaf.svg")));
        QVERIFY(reg.ornament(QStringLiteral("both"), QStringLiteral("leaf"), QStringLiteral("classic")).toString().endsWith(QStringLiteral("leaf.svg")));
        QVERIFY(wall("one", "classic").value(QStringLiteral("image")).toString().endsWith(QStringLiteral("/one/bg.png")));
        QVERIFY(wall("one", "classic").value(QStringLiteral("pixel")).toBool());
        QCOMPARE(reg.problems().size(), 1);
        QVERIFY2(reg.problems().first().contains(QStringLiteral("gone.jpg")), qPrintable(reg.problems().first()));

        QJsonObject snap = fixture();
        QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
        QJsonObject ui = layout.value(QStringLiteral("ui")).toObject();
        ui.insert(QStringLiteral("art_style"), QStringLiteral("classic"));
        layout.insert(QStringLiteral("ui"), ui);
        snap.insert(QStringLiteral("layout"), layout);
        QVERIFY(SessionModel::instance()->applySnapshot(snap));
        QCOMPARE(Theme::instance()->artStyle(), QStringLiteral("classic"));
        ui.insert(QStringLiteral("art_style"), QStringLiteral("watercolour"));
        layout.insert(QStringLiteral("ui"), ui);
        snap.insert(QStringLiteral("layout"), layout);
        QVERIFY(!SessionModel::instance()->applySnapshot(snap));
        QVERIFY(SessionModel::instance()->applySnapshot(fixture())); // no art_style: pixel
        QCOMPARE(Theme::instance()->artStyle(), QStringLiteral("pixel"));
    }

    // Classic art style, the basics: boxes are antialiased rounded
    // Rectangles, the built-in worlds show their classic pictures, and
    // ornaments resolve to their SVGs; switching back restores pixel art.
    void classicDrawsSmooth()
    {
        auto withArt = [&](const char *art) {
            QJsonObject snap = fixture();
            QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
            QJsonObject ui = layout.value(QStringLiteral("ui")).toObject();
            ui.insert(QStringLiteral("background"), QStringLiteral("den"));
            ui.insert(QStringLiteral("art_style"), QString::fromLatin1(art));
            layout.insert(QStringLiteral("ui"), ui);
            snap.insert(QStringLiteral("layout"), layout);
            QVERIFY(SessionModel::instance()->applySnapshot(snap));
        };
        const auto restore = qScopeGuard([&] { QVERIFY(SessionModel::instance()->applySnapshot(fixture())); });
        QQmlComponent c(m_engine);
        c.setData("import QtQuick\nimport BearDen\nPixelBox { width: 80; height: 40; radius: 8; property url sprig: World.ornament(\"sprig\") }",
                  QUrl(QStringLiteral("qrc:/test/Classic.qml")));
        std::unique_ptr<QObject> box(c.create());
        QVERIFY2(box, qPrintable(c.errorString()));
        auto drawn = [&](const char *type) {
            for (QQuickItem *child : qobject_cast<QQuickItem *>(box.get())->childItems())
                if (QByteArray(child->metaObject()->className()).startsWith(type))
                    return child->isVisible();
            return false;
        };
        withArt("classic");
        QVERIFY(drawn("QQuickRectangle") && !drawn("QQuickCanvasItem"));
        QVERIFY(box->property("sprig").toUrl().toString().endsWith(QStringLiteral("/sprig.svg")));
        QVERIFY(Theme::instance()->wallpaperSource().toString().endsWith(QStringLiteral("/den/wallpaper.jpg")));
        // Its own size, for animated layers placed in its pixels.
        const QVariantMap wp = ThemeRegistry::instance()->get(QStringLiteral("den"), QStringLiteral("classic")).value(QStringLiteral("wallpaper")).toMap();
        QCOMPARE(wp.value(QStringLiteral("width")).toInt(), 2560);
        QCOMPARE(wp.value(QStringLiteral("height")).toInt(), 1440);
        withArt("pixel");
        QVERIFY(!drawn("QQuickRectangle") && drawn("QQuickCanvasItem"));
        QVERIFY(box->property("sprig").toUrl().toString().endsWith(QStringLiteral("/sprig.png")));
        QVERIFY(Theme::instance()->wallpaperSource().toString().endsWith(QStringLiteral("/den/wallpaper.png")));
    }

    // Pixel art building blocks (docs/THEMES.md → Pixel art): PixelBox cuts
    // its corners in stairs of whole art pixels, bears pick the frame for
    // their pose from the generated rig, PNG ornaments draw as pixel art.
    void pixelArtBuildingBlocks()
    {
        auto make = [&](const QByteArray &qml) {
            QQmlComponent c(m_engine);
            c.setData("import QtQuick\nimport BearDen\n" + qml, QUrl(QStringLiteral("qrc:/test/Pixel.qml")));
            QObject *o = c.create();
            if (!o)
                qWarning() << c.errors();
            return o;
        };
        QObject *box = make("PixelBox { width: 80; height: 40; radius: 8 }");
        QVERIFY(box);
        QVariant ret;
        QMetaObject::invokeMethod(box, "insets", Q_RETURN_ARG(QVariant, ret), Q_ARG(QVariant, 10), Q_ARG(QVariant, 2));
        QCOMPARE(ret.toList(), (QVariantList{2, 1, 0, 0, 0, 0, 0, 0, 1, 2}));
        QMetaObject::invokeMethod(box, "insets", Q_RETURN_ARG(QVariant, ret), Q_ARG(QVariant, 12), Q_ARG(QVariant, 5));
        const QVariantList round = ret.toList();
        for (int y = 1; y < 6; ++y)
            QVERIFY2(round.at(y).toInt() <= round.at(y - 1).toInt(), "a round corner never widens going in");
        QVERIFY(round.at(0).toInt() > 1 && round.at(5).toInt() == 0);
        delete box;

        QObject *bear = make("BearPuppet { kind: \"dad\"; size: 184 }");
        QVERIFY(bear);
        QVERIFY(bear->property("unit").toReal() > 0);
        QCOMPARE(bear->property("width").toReal(), bear->property("frameW").toReal() * bear->property("unit").toReal());
        QCOMPARE(bear->property("pose").toString(), QStringLiteral("stand"));
        bear->setProperty("walking", true);
        bear->setProperty("walk", 0.1);
        QCOMPARE(bear->property("pose").toString(), QStringLiteral("walk0"));
        bear->setProperty("walk", 3.3);
        QCOMPARE(bear->property("pose").toString(), QStringLiteral("walk2"));
        bear->setProperty("walk", -0.2);   // negative phases wrap round
        QCOMPARE(bear->property("pose").toString(), QStringLiteral("walk3"));
        bear->setProperty("walking", false);
        bear->setProperty("wave", 1.0);
        bear->setProperty("wavePhase", 1.5);
        QCOMPARE(bear->property("pose").toString(), QStringLiteral("wave0"));
        bear->setProperty("wave", 0.0);
        bear->setProperty("sitting", true);
        bear->setProperty("reach", 1.0);
        QCOMPARE(bear->property("pose").toString(), QStringLiteral("reach"));
        bear->setProperty("asleep", true);
        QCOMPARE(bear->property("pose").toString(), QStringLiteral("sleep"));
        // Every pose the puppet can pick has anchors in the generated rig.
        const QVariantMap at = bear->property("at").toMap();
        QVERIFY(at.contains(QStringLiteral("paw")) && at.contains(QStringLiteral("head")));
        delete bear;

        QObject *orn = make("Ornament { name: \"daisy\"; width: 40; height: 40 }");
        QVERIFY(orn);
        QVERIFY2(orn->property("pixel").toBool(), "built-in ornaments are PNG pixel art");
        delete orn;
    }

    // Classic art style, the drawn and animated parts: bears are smooth SVG
    // rigs posed by the same numbers as the pixel ones, the corner engine and
    // brand backdrops paint at full size with antialiasing, the focus ring's
    // spark is the smooth canvas, and Home picks the classic corner scene.
    // Pixel keeps the grid-snapped versions.
    void classicComponents()
    {
        auto withArt = [&](const char *art) {
            QJsonObject snap = fixture();
            QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
            QJsonObject ui = layout.value(QStringLiteral("ui")).toObject();
            ui.insert(QStringLiteral("background"), QStringLiteral("den"));
            ui.insert(QStringLiteral("art_style"), QString::fromLatin1(art));
            layout.insert(QStringLiteral("ui"), ui);
            snap.insert(QStringLiteral("layout"), layout);
            QVERIFY(SessionModel::instance()->applySnapshot(snap));
        };
        const auto restore = qScopeGuard([&] { QVERIFY(SessionModel::instance()->applySnapshot(fixture())); });
        const QByteArray probeQml = R"(import QtQuick
import BearDen
Item {
    width: 400; height: 300
    BearPuppet { objectName: "puppet"; kind: "dad"; size: 200 }
    BearHead { objectName: "head"; kind: "dad"; width: 80 }
    CornerDecor { objectName: "corner"; progress: 1 }
    BrandBackdrop { objectName: "backdrop"; width: 120; height: 80 }
    Item { width: 200; height: 100; FocusFrame { objectName: "ring"; shown: true; glint: true } }
})";
        QQmlComponent c(m_engine);
        c.setData(probeQml, QUrl(QStringLiteral("qrc:/test/ClassicParts.qml")));
        std::unique_ptr<QObject> probe(c.create());
        QVERIFY2(probe, qPrintable(c.errorString()));
        auto item = [&](const char *name) { return probe->findChild<QQuickItem *>(QString::fromLatin1(name)); };
        // Image sources drawn (visible) under an item.
        auto sources = [&](QQuickItem *root) {
            QStringList out;
            std::function<void(QQuickItem *)> walk = [&](QQuickItem *i) {
                if (!i->isVisible())
                    return;
                const QString s = i->property("source").toUrl().toString();
                if (!s.isEmpty() && QByteArray(i->metaObject()->className()).contains("Image"))
                    out << s;
                for (QQuickItem *child : i->childItems())
                    walk(child);
            };
            walk(root);
            return out;
        };
        QQuickItem *puppet = item("puppet");
        QVERIFY(puppet);

        withArt("classic");
        QCoreApplication::processEvents();
        QQuickItem *rigLoader = item("bearPuppetClassic");
        QVERIFY(rigLoader);
        QObject *rig = rigLoader->property("item").value<QObject *>();
        QVERIFY2(rig, "Classic loads the smooth rig");
        const QStringList parts = sources(puppet);
        QVERIFY2(parts.contains(QStringLiteral("qrc:/qt/qml/BearDen/assets/bear-mark.svg"))
                     && parts.contains(QStringLiteral("qrc:/qt/qml/BearDen/assets/bear-arm.svg")),
                 qPrintable(parts.join(QLatin1Char(' '))));
        for (const QString &s : parts)
            QVERIFY2(s.endsWith(QStringLiteral(".svg")), qPrintable(s));
        QCOMPARE(puppet->width(), 140.0);
        puppet->setProperty("walking", true);
        puppet->setProperty("walk", 1.0);
        QCOMPARE(rig->property("step").toReal(), std::sin(1.0));
        puppet->setProperty("sitting", true);
        QVERIFY(rig->property("seat").toReal() > 0);
        puppet->setProperty("asleep", true);
        QVERIFY(sources(puppet).contains(QStringLiteral("qrc:/qt/qml/BearDen/assets/bear-sleep.svg")));
        QVERIFY(sources(item("head")).contains(QStringLiteral("qrc:/qt/qml/BearDen/assets/bear-mark.svg")));
        QVERIFY(item("cornerSmooth")->isVisible() && item("cornerSmooth")->antialiasing() && !item("cornerPixel")->isVisible());
        QCOMPARE(item("cornerSmooth")->width(), item("corner")->width());
        QVERIFY(item("brandBackdropSmooth")->isVisible() && item("brandBackdropSmooth")->antialiasing());
        QCOMPARE(item("ring")->property("thickness").toReal(), Theme::instance()->property("focusWidth").toReal());
        QVERIFY(item("focusSpark")->antialiasing());

        goHome();
        QObject *scene = m_window->findChild<QObject *>(QStringLiteral("cornerScene"));
        QVERIFY(scene);
        auto sceneMade = [&]() {
            auto *comp = scene->property("sourceComponent").value<QQmlComponent *>();
            std::unique_ptr<QObject> o(comp ? comp->create(comp->creationContext()) : nullptr);
            return o ? QString::fromLatin1(o->metaObject()->className()) : QString();
        };
        QVERIFY2(sceneMade().startsWith(QStringLiteral("DenFamilyClassic")), qPrintable(sceneMade()));

        withArt("pixel");
        QCoreApplication::processEvents();
        QVERIFY(!rigLoader->property("item").value<QObject *>());
        for (const QString &s : sources(puppet))
            QVERIFY2(s.endsWith(QStringLiteral(".png")), qPrintable(s));
        QCOMPARE(puppet->width(), puppet->property("frameW").toReal() * puppet->property("unit").toReal());
        QVERIFY(!item("cornerSmooth")->isVisible() && item("cornerPixel")->isVisible());
        QVERIFY(!item("cornerPixel")->antialiasing() && !item("cornerPixel")->smooth());
        QVERIFY(!item("brandBackdropSmooth")->isVisible());
        const int px = qRound(4 * Theme::instance()->property("scale").toReal());
        QCOMPARE(std::fmod(item("ring")->property("thickness").toReal(), qMax(1, px)), 0.0);
        const QString pixelScene = sceneMade();
        QVERIFY2(pixelScene.startsWith(QStringLiteral("DenFamily")) && !pixelScene.contains(QStringLiteral("Classic")), qPrintable(pixelScene));
    }

    // The featured panel's life: every app's stage names a room that exists in
    // the generated rig, the cabin TV plays its static once per item, and the
    // remote's secret code starts the bear parade.
    void heroPanelLife()
    {
        auto make = [&](const QByteArray &qml) {
            QQmlComponent c(m_engine);
            c.setData("import QtQuick\nimport BearDen\n" + qml, QUrl(QStringLiteral("qrc:/test/Hero.qml")));
            QObject *o = c.create();
            if (!o)
                qWarning() << c.errors();
            return o;
        };
        QObject *probe = make(R"(import "qrc:/qt/qml/BearDen/HeroRig.js" as Rig
            Item {
                function missing() {
                    const out = []
                    for (const a of ["plex-htpc", "vacuumtube", "moonlight", "spotify", "jellyfin", "retroarch", "something-new"])
                        if (!Rig.scenes[Apps.stage(a).scene]) out.push(a)
                    return out.join(",")
                }
                // Every app has its own room and brand colours (only unknown
                // adapters fall back to the cabin).
                function rooms() {
                    const out = []
                    for (const a of ["plex-htpc", "vacuumtube", "moonlight", "spotify", "jellyfin", "retroarch"])
                        out.push(Apps.stage(a).scene + (Apps.brand(a) ? "" : "!"))
                    return out.join(",")
                }
            })");
        QVERIFY(probe);
        QVariant missing;
        QMetaObject::invokeMethod(probe, "missing", Q_RETURN_ARG(QVariant, missing));
        QCOMPARE(missing.toString(), QString());
        QVariant rooms;
        QMetaObject::invokeMethod(probe, "rooms", Q_RETURN_ARG(QVariant, rooms));
        QCOMPARE(rooms.toString(), QStringLiteral("cinema,cabin,arcade,nook,theatre,retro"));
        delete probe;

        Theme::instance()->setForceNoAnimations(false);
        const auto restore = qScopeGuard([] { Theme::instance()->setForceNoAnimations(true); });
        QObject *room = make("HeroScene { scene: \"cabin\" }");
        QVERIFY(room);
        room->setProperty("itemId", QStringLiteral("youtube"));
        QVERIFY2(room->property("introPlaying").toBool(), "a new item starts on the TV's static");
        QTRY_VERIFY_WITH_TIMEOUT(!room->property("introPlaying").toBool(), 3000);
        delete room;

        goHome();
        QObject *visitors = m_window->findChild<QObject *>(QStringLiteral("bearVisitors"));
        QVERIFY(visitors);
        QVERIFY(!visitors->property("busy").toBool());
        for (const char *a : {"nav.up", "nav.up", "nav.down", "nav.down", "nav.left", "nav.right", "nav.left", "nav.right"})
            act(QString::fromLatin1(a));
        QVERIFY(!visitors->property("busy").toBool());
        act(QStringLiteral("select"));
        QVERIFY2(visitors->property("busy").toBool(), "the secret code starts a parade");
        QMetaObject::invokeMethod(visitors, "stop");
        goHome();
    }

    // Classic art for what was only ever pixel art (tools/classicart): every
    // featured-panel room, weather icon and seasonal piece has an SVG; Winter
    // has a classic world; HeroScene draws the SVG room in Classic with the
    // icon's screen rectangle exactly where the pixel room puts it.
    void classicArtAssets()
    {
        const QString dir = QStringLiteral(":/qt/qml/BearDen/assets/");
        QStringList names{QStringLiteral("classic/hero-cinema"), QStringLiteral("classic/hero-arcade"),
                          QStringLiteral("classic/hero-static-0"), QStringLiteral("classic/hero-static-1"),
                          QStringLiteral("classic/snowcap"), QStringLiteral("classic/sleep-z"), QStringLiteral("ornaments/pumpkin")};
        for (const char *scene : {"cinema", "cabin", "arcade", "nook", "theatre", "retro"})
            names << QStringLiteral("classic/hero-%1-glow").arg(QLatin1String(scene));
        for (const char *scene : {"nook", "theatre", "retro"}) {
            names << QStringLiteral("classic/hero-%1").arg(QLatin1String(scene));
            QVERIFY2(!QImage(dir + QStringLiteral("pixel/hero-%1.png").arg(QLatin1String(scene))).isNull(), scene);
        }
        for (const char *time : {"night", "dawn", "day", "dusk"})
            names << QStringLiteral("classic/hero-cabin-%1").arg(QLatin1String(time));
        for (const char *icon : {"sun", "moon", "sun-cloud", "moon-cloud", "cloud", "fog", "drizzle", "rain", "snow", "thunder"}) {
            QVERIFY2(QFile::exists(dir + QStringLiteral("pixel/weather-%1.png").arg(QLatin1String(icon))), icon);
            names << QStringLiteral("classic/weather-%1").arg(QLatin1String(icon));
        }
        for (const QString &name : names)
            QVERIFY2(QFile::exists(dir + name + QStringLiteral(".svg")), qPrintable(name));
        // The weather props (tools/classicart/extras.py) must also decode: Qt's SVG
        // reader rejects a file with a repeated attribute, and the prop vanishes.
        for (const char *prop : {"classic/scene-tarp", "classic/startle", "classic/umbrella-leaf"}) {
            const QString path = dir + QLatin1String(prop) + QStringLiteral(".svg");
            QVERIFY2(!QImage(path).isNull(), prop);
        }
        for (const char *prop : {"tarp", "startle", "umbrella"})
            QVERIFY2(!QImage(dir + QStringLiteral("pixel/scene-wx-%1.png").arg(QLatin1String(prop))).isNull(), prop);

        auto withArt = [&](const char *art) {
            QJsonObject snap = fixture();
            QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
            QJsonObject ui = layout.value(QStringLiteral("ui")).toObject();
            ui.insert(QStringLiteral("background"), QStringLiteral("winter"));
            ui.insert(QStringLiteral("art_style"), QString::fromLatin1(art));
            layout.insert(QStringLiteral("ui"), ui);
            snap.insert(QStringLiteral("layout"), layout);
            QVERIFY(SessionModel::instance()->applySnapshot(snap));
        };
        const auto restore = qScopeGuard([&] { QVERIFY(SessionModel::instance()->applySnapshot(fixture())); });
        const QByteArray qml = "import QtQuick\nimport BearDen\n"
                               "HeroScene { scene: \"cinema\"; itemId: \"plex\"; shift: 1\n"
                               "  property var classicWinter: Themes.get(\"winter\", \"classic\")\n"
                               "  property var pixelWinter: Themes.get(\"winter\", \"pixel\") }";
        QQmlComponent c(m_engine);
        c.setData(qml, QUrl(QStringLiteral("qrc:/test/ClassicArt.qml")));
        std::unique_ptr<QObject> room(c.create());
        QVERIFY2(room, qPrintable(c.errorString()));

        const QVariantMap cw = room->property("classicWinter").toMap();
        const QVariantMap cwWall = cw.value(QStringLiteral("wallpaper")).toMap();
        QVERIFY(cwWall.value(QStringLiteral("image")).toString().endsWith(QStringLiteral("/winter/classic-wallpaper.svg")));
        QVERIFY(!cwWall.value(QStringLiteral("pixel")).toBool());
        QVERIFY(cw.value(QStringLiteral("phone")).toMap().value(QStringLiteral("backdrop")).toString().endsWith(QStringLiteral("/winter/classic-backdrop.svg")));
        // Every built-in world animates in Classic too (tools/classicart/layers.py):
        // at least one classic.wallpaper.sprites layer, each sheet loads, splits
        // into whole frames and sits inside the picture it is placed on.
        int builtIns = 0;
        for (const QVariant &entry : ThemeRegistry::instance()->list()) {
            const QVariantMap info = entry.toMap();
            if (!info.value(QStringLiteral("builtIn")).toBool())
                continue;
            ++builtIns;
            const QString id = info.value(QStringLiteral("id")).toString();
            const QVariantMap wall = ThemeRegistry::instance()->get(id, QStringLiteral("classic")).value(QStringLiteral("wallpaper")).toMap();
            const QVariantList layers = wall.value(QStringLiteral("sprites")).toList();
            QVERIFY2(!wall.value(QStringLiteral("pixel")).toBool(), qPrintable(id));
            QVERIFY2(!layers.isEmpty(), qPrintable(id + QStringLiteral(": no classic layers")));
            for (const QVariant &layer : layers) {
                const QVariantMap sp = layer.toMap();
                const QUrl url(sp.value(QStringLiteral("sheet")).toString());
                const QString path = url.scheme() == QLatin1String("qrc") ? QLatin1Char(':') + url.path() : url.toLocalFile();
                QImageReader reader(path);
                const QImage sheet = reader.read();
                QVERIFY2(!sheet.isNull(), qPrintable(id + QStringLiteral(": ") + path + QStringLiteral(" ") + reader.errorString()));
                const int frames = sp.value(QStringLiteral("frames")).toInt();
                QVERIFY2(frames > 0 && sheet.width() % frames == 0, qPrintable(path));
                const int x = sp.value(QStringLiteral("x")).toInt(), y = sp.value(QStringLiteral("y")).toInt();
                QVERIFY2(x >= 0 && y >= 0 && x < wall.value(QStringLiteral("width")).toInt() && y < wall.value(QStringLiteral("height")).toInt(),
                         qPrintable(id + QStringLiteral(": layer outside the picture")));
            }
        }
        QVERIFY(builtIns >= 5);

        const QVariantMap pw = room->property("pixelWinter").toMap();
        QVERIFY(pw.value(QStringLiteral("wallpaper")).toMap().value(QStringLiteral("image")).toString().endsWith(QStringLiteral("/winter/wallpaper.png")));

        withArt("pixel");
        QCoreApplication::processEvents();
        const QRectF pixelScreen = room->property("screenRect").toRectF();
        QObject *classicRoom = room->findChild<QObject *>(QStringLiteral("heroClassicRoom"));
        QObject *classicImage = room->findChild<QObject *>(QStringLiteral("heroClassicImage"));
        QVERIFY(classicRoom && classicImage);
        QVERIFY(!classicRoom->property("visible").toBool());
        withArt("classic");
        QCoreApplication::processEvents();
        QVERIFY(classicRoom->property("visible").toBool());
        QVERIFY(classicImage->property("source").toUrl().toString().endsWith(QStringLiteral("/classic/hero-cinema.svg")));
        QCOMPARE(room->property("screenRect").toRectF(), pixelScreen);
        QVERIFY(!pixelScreen.isEmpty());
    }

    void largeTextStillFits()
    {
        QJsonObject snap = fixture();
        QJsonObject layout = snap.value(QStringLiteral("layout")).toObject();
        QJsonObject ui = layout.value(QStringLiteral("ui")).toObject();
        ui.insert(QStringLiteral("text_scale"), 1.5);
        ui.insert(QStringLiteral("tile_density"), QStringLiteral("large"));
        ui.insert(QStringLiteral("safe_margin_percent"), 6);
        ui.insert(QStringLiteral("high_contrast_focus"), true);
        layout.insert(QStringLiteral("ui"), ui);
        snap.insert(QStringLiteral("layout"), layout);
        QVERIFY(SessionModel::instance()->applySnapshot(snap));
        goHome();
        toFavorites();
        act(QStringLiteral("nav.down"));
        QCOMPARE(m_nav->sectionId(), QStringLiteral("plex-continue"));
        shot(QStringLiteral("home-large-text"));
        QVERIFY(SessionModel::instance()->applySnapshot(fixture()));
    }

    // The campfire scene picks the pieces its data declares for each
    // condition; no weather, or a clear or cloudy day, leaves it unchanged.
    void sceneWeatherPicksVariant()
    {
        const auto restore = qScopeGuard([&] { QVERIFY(SessionModel::instance()->applySnapshot(fixture())); });
        QQmlComponent c(m_engine);
        c.setData("import QtQuick\nimport BearDen\nCampfireScene {}", QUrl(QStringLiteral("qrc:/test/Campfire.qml")));
        std::unique_ptr<QObject> scene(c.create());
        QVERIFY2(scene, qPrintable(c.errorString()));
        QObject *back = weatherSide(scene.get(), QStringLiteral("back"));
        QObject *front = weatherSide(scene.get(), QStringLiteral("front"));
        QVERIFY(back && front);
        QObject *flame = scene->findChild<QObject *>(QStringLiteral("campfireFlame"));
        QVERIFY(flame);

        struct Case { const char *condition; bool day; const char *look, *back, *front; };
        const Case cases[] = {
            {"", true, "", "", ""},
            {"clear", true, "", "", ""},
            {"cloudy", false, "", "", ""},
            {"drizzle", true, "wet", "puddles,shelter", "drips"},
            {"rain", false, "wet", "puddles,shelter", "drips"},
            {"thunder", true, "storm", "puddles,shelter", "drips,startle"},
            {"snow", true, "snow", "", "caps"},
            {"fog", true, "fog", "", "fade,mist"},
            {"clear", false, "night", "stars", ""},
            {"partly-cloudy", false, "night", "stars", ""},
        };
        for (const Case &k : cases) {
            applyWeather(QString::fromLatin1(k.condition), k.day);
            const QString what = QStringLiteral("%1 %2").arg(QLatin1String(k.condition), k.day ? QStringLiteral("day") : QStringLiteral("night"));
            QCOMPARE(front->property("look").toString(), QString::fromLatin1(k.look));
            QVERIFY2(drawn(back) == QLatin1String(k.back), qPrintable(what + QStringLiteral(": back ") + drawn(back)));
            QVERIFY2(drawn(front) == QLatin1String(k.front), qPrintable(what + QStringLiteral(": front ") + drawn(front)));
            // The fire smoulders only in the rain; fog fades the props.
            const bool wet = QByteArray(k.look) == "wet" || QByteArray(k.look) == "storm";
            QCOMPARE(flame->property("opacity").toReal() < 1.0, wet);
            QCOMPARE(front->property("fade").toReal() > 0, QByteArray(k.look) == "fog");
        }
    }

    // Every corner scene, in both art styles, reacts to every look (ADR 0006:
    // Classic matches pixel art); visiting bears dress for rain and snow.
    void sceneWeatherEveryScene()
    {
        const auto restore = qScopeGuard([&] { QVERIFY(SessionModel::instance()->applySnapshot(fixture())); });
        const struct { const char *condition; bool day; const char *look; } looks[] = {
            {"rain", true, "wet"}, {"thunder", true, "storm"}, {"snow", true, "snow"}, {"fog", true, "fog"}, {"clear", false, "night"}};
        for (const char *art : {"pixel", "classic"}) {
            for (const char *name : {"DenFamily", "CampScene", "MoonScene", "CampfireScene"}) {
                const QByteArray type = QByteArray(name) + (QByteArray(art) == "classic" ? "Classic" : "");
                applyWeather(QString(), true, false, QString::fromLatin1(art));
                QQmlComponent c(m_engine);
                c.setData("import QtQuick\nimport BearDen\n" + type + " {}", QUrl(QStringLiteral("qrc:/test/Scene.qml")));
                std::unique_ptr<QObject> scene(c.create());
                QVERIFY2(scene, qPrintable(c.errorString()));
                QObject *back = weatherSide(scene.get(), QStringLiteral("back"));
                QObject *front = weatherSide(scene.get(), QStringLiteral("front"));
                QVERIFY2(back && front, type.constData());
                QVERIFY2(drawn(back).isEmpty() && drawn(front).isEmpty(), type.constData());
                for (const auto &l : looks) {
                    applyWeather(QString::fromLatin1(l.condition), l.day, false, QString::fromLatin1(art));
                    QCOMPARE(front->property("look").toString(), QString::fromLatin1(l.look));
                    QVERIFY2(!(drawn(back) + drawn(front)).isEmpty(), qPrintable(QString::fromLatin1(type) + QLatin1Char(' ') + QLatin1String(l.look)));
                    if (QByteArray(l.look) == "storm")
                        QVERIFY2(drawn(front).contains(QStringLiteral("startle")), type.constData());
                }
            }
        }

        QQmlComponent c(m_engine);
        c.setData("import QtQuick\nimport BearDen\nItem {\n"
                  "  property var visitor: BearPuppet { kind: \"cub\"; weatherDress: true }\n"
                  "  property var scenery: BearPuppet { kind: \"cub\" } }",
                  QUrl(QStringLiteral("qrc:/test/Dress.qml")));
        std::unique_ptr<QObject> probe(c.create());
        QVERIFY2(probe, qPrintable(c.errorString()));
        auto *visitor = probe->property("visitor").value<QObject *>();
        auto *scenery = probe->property("scenery").value<QObject *>();
        applyWeather(QStringLiteral("rain"));
        QCOMPARE(visitor->property("dress").toString(), QStringLiteral("umbrella"));
        QVERIFY(visitor->findChild<QQuickItem *>(QStringLiteral("bearUmbrella"))->isVisible());
        QCOMPARE(scenery->property("dress").toString(), QString());   // scene bears keep their own dress
        applyWeather(QStringLiteral("snow"));
        QVERIFY2(visitor->property("wornHat").toUrl().toString().endsWith(QStringLiteral("hat-beanie.png")),
                 qPrintable(visitor->property("wornHat").toUrl().toString()));
        applyWeather(QString());
        QCOMPARE(visitor->property("dress").toString(), QString());
        QVERIFY(!visitor->findChild<QQuickItem *>(QStringLiteral("bearUmbrella"))->isVisible());
    }

    // Motion: the pieces move on World's heartbeat and the bears startle on a
    // lightning flash; with reduced motion the scene shows its still version
    // (same pieces, nothing moves, no startle).
    void sceneWeatherMotionAndStill()
    {
        Theme::instance()->setForceNoAnimations(false);
        const auto restore = qScopeGuard([&] {
            Theme::instance()->setForceNoAnimations(true);
            QVERIFY(SessionModel::instance()->applySnapshot(fixture()));
        });
        QQmlComponent c(m_engine);
        c.setData("import QtQuick\nimport BearDen\nItem {\n"
                  "  property var scene: CampfireScene {}\n"
                  "  function flash(on) { World.flashing = on } }",
                  QUrl(QStringLiteral("qrc:/test/Motion.qml")));
        std::unique_ptr<QObject> probe(c.create());
        QVERIFY2(probe, qPrintable(c.errorString()));
        auto *scene = probe->property("scene").value<QObject *>();
        QObject *front = weatherSide(scene, QStringLiteral("front"));
        QVERIFY(front);
        auto flash = [&](bool on) { QMetaObject::invokeMethod(probe.get(), "flash", Q_ARG(QVariant, on)); };

        applyWeather(QStringLiteral("thunder"));
        QVERIFY(front->property("alive").toBool());
        QTRY_VERIFY_WITH_TIMEOUT(front->property("t").toReal() > 0, 2000);   // drips fall on the heartbeat
        flash(true);
        QVERIFY2(front->property("startled").toBool(), "a flash startles the bears");
        flash(false);
        QTRY_VERIFY_WITH_TIMEOUT(!front->property("startled").toBool(), 4000);   // and it passes

        applyWeather(QStringLiteral("thunder"), true, true);   // reduced motion
        QCOMPARE(drawn(front), QStringLiteral("drips,startle"));   // the still version keeps its pieces
        QVERIFY(!front->property("alive").toBool());
        const qreal t = front->property("t").toReal();
        flash(true);
        QVERIFY2(!front->property("startled").toBool(), "no startle with reduced motion");
        flash(false);
        QTest::qWait(300);
        QCOMPARE(front->property("t").toReal(), t);
    }
};

QTEST_MAIN(ShellTest)
#include "tst_shell.moc"
