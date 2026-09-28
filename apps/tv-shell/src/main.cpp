// bear-den-tv-shell: the Qt Quick TV home screen. Started and supervised by the
// coordinator (`bear-den-tv session`), which it reaches over the private shell
// socket (contracts/ipc.md).
//
//   --dev                 development build behavior; honors BDTV_SHELL_SOCKET
//   --fixture PATH        offline: render a state snapshot file, no coordinator
//   --screen NAME         start on home|settings|pairing|devices|diagnostics|remote-setup
//   --windowed            do not go fullscreen
//   --screenshot PATH     save a PNG of the window, then keep running (or exit with --exit-after)
//   --screenshot-after MS delay before the (first) screenshot, default 1500
//   --screenshot-every MS rewrite the screenshot periodically (dev evidence)
//   --exit-after MS       quit after MS milliseconds (screenshots, tests)
#include "Navigator.h"
#include "SessionModel.h"
#include "ShellController.h"
#include "Theme.h"
#include "ThemeRegistry.h"

#include <QCommandLineParser>
#include <QDir>
#include <QFile>
#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQuickWindow>
#include <QTimer>

#include <cstdio>

int main(int argc, char *argv[])
{
    // WM_CLASS is derived from the application name; the coordinator matches it.
    QGuiApplication::setApplicationName(QStringLiteral("bear-den-tv-shell"));
    QGuiApplication::setApplicationDisplayName(QStringLiteral("Bear Den TV"));
    QGuiApplication::setApplicationVersion(QStringLiteral("0.1.0"));
    QGuiApplication::setDesktopFileName(QStringLiteral("bear-den-tv-shell"));
    QGuiApplication app(argc, argv);

    QCommandLineParser parser;
    parser.setApplicationDescription(QStringLiteral("Bear Den TV home screen"));
    parser.addHelpOption();
    parser.addVersionOption();
    const QCommandLineOption devOpt(QStringLiteral("dev"), QStringLiteral("Development mode."));
    const QCommandLineOption fixtureOpt(QStringLiteral("fixture"), QStringLiteral("Render a state snapshot offline."), QStringLiteral("path"));
    const QCommandLineOption screenOpt(QStringLiteral("screen"), QStringLiteral("Start screen."), QStringLiteral("name"), QStringLiteral("home"));
    const QCommandLineOption windowedOpt(QStringLiteral("windowed"), QStringLiteral("Do not go fullscreen."));
    const QCommandLineOption shotOpt(QStringLiteral("screenshot"), QStringLiteral("Save a PNG of the window."), QStringLiteral("path"));
    const QCommandLineOption shotAfterOpt(QStringLiteral("screenshot-after"), QStringLiteral("Delay before screenshot (ms)."), QStringLiteral("ms"), QStringLiteral("1500"));
    const QCommandLineOption shotEveryOpt(QStringLiteral("screenshot-every"), QStringLiteral("Rewrite the screenshot every N ms."), QStringLiteral("ms"));
    const QCommandLineOption exitAfterOpt(QStringLiteral("exit-after"), QStringLiteral("Quit after N ms."), QStringLiteral("ms"));
    const QCommandLineOption sizeOpt(QStringLiteral("size"), QStringLiteral("Window size WxH."), QStringLiteral("WxH"), QStringLiteral("1920x1080"));
    parser.addOptions({devOpt, fixtureOpt, screenOpt, windowedOpt, shotOpt, shotAfterOpt, shotEveryOpt, exitAfterOpt, sizeOpt});
    parser.process(app);

    ShellController::Options opts;
    opts.dev = parser.isSet(devOpt);
    opts.fixturePath = parser.value(fixtureOpt);
    opts.offline = !opts.fixturePath.isEmpty();
    opts.startScreen = parser.value(screenOpt);
    // The socket override is a test/development affordance only (contracts/ipc.md).
    const QString envSocket = qEnvironmentVariable("BDTV_SHELL_SOCKET");
    if (!envSocket.isEmpty()) {
        if (opts.dev)
            opts.socketPath = envSocket;
        else
            qWarning("bear-den-tv-shell: ignoring BDTV_SHELL_SOCKET without --dev");
    }

    // Singletons exist before QML so C++ wiring and QML share one instance.
    auto *nav = Navigator::create(nullptr, nullptr);
    auto *themes = ThemeRegistry::create(nullptr, nullptr); // before Theme: backgrounds come from themes
    auto *theme = Theme::create(nullptr, nullptr);
    QObject::connect(themes, &ThemeRegistry::changed, theme, &Theme::tokensChanged);
    SessionModel::create(nullptr, nullptr);
    auto *controller = ShellController::create(nullptr, nullptr);
    controller->configure(opts);

    const QStringList size = parser.value(sizeOpt).split(QLatin1Char('x'));
    const int width = size.value(0).toInt() > 0 ? size.value(0).toInt() : 1920;
    const int height = size.value(1).toInt() > 0 ? size.value(1).toInt() : 1080;

    QQmlApplicationEngine engine;
    engine.setInitialProperties({{QStringLiteral("initialWidth"), width},
                                 {QStringLiteral("initialHeight"), height},
                                 {QStringLiteral("fullscreen"), !parser.isSet(windowedOpt) && !opts.offline}});
    QObject::connect(&engine, &QQmlApplicationEngine::objectCreationFailed, &app, [] { QCoreApplication::exit(1); }, Qt::QueuedConnection);
    engine.loadFromModule("BearDen", "Main");
    if (engine.rootObjects().isEmpty())
        return 1;
    auto *window = qobject_cast<QQuickWindow *>(engine.rootObjects().constFirst());
    nav->setWindow(window);
    // A TV is driven by the remote; never show the mouse pointer over the shell.
    if (window && !parser.isSet(windowedOpt))
        window->setCursor(Qt::BlankCursor);
    theme->setWindowWidth(width);
    theme->setWindowHeight(height);
    controller->start();

    if (parser.isSet(shotOpt) && window) {
        const QString path = parser.value(shotOpt);
        auto shoot = [window, path] {
            // Write then rename so readers never see a half-written PNG.
            const QImage img = window->grabWindow();
            const QString tmp = path + QStringLiteral(".tmp.png");
            if (img.isNull() || !img.save(tmp) || (QFile::exists(path) && !QFile::remove(path)) || !QFile::rename(tmp, path))
                qWarning("bear-den-tv-shell: screenshot to %s failed", qPrintable(path));
        };
        QTimer::singleShot(parser.value(shotAfterOpt).toInt(), window, shoot);
        if (parser.isSet(shotEveryOpt) && opts.dev) {
            auto *timer = new QTimer(window);
            QObject::connect(timer, &QTimer::timeout, window, shoot);
            timer->start(qMax(250, parser.value(shotEveryOpt).toInt()));
        }
    }
    // Performance diagnostics: BDTV_FPS_LOG=1 prints frames per second every
    // 5 s (an idle, resting shell should print 0).
    if (qEnvironmentVariableIntValue("BDTV_FPS_LOG") == 1 && window) {
        auto *frames = new int(0);
        QObject::connect(window, &QQuickWindow::frameSwapped, window, [frames] { ++*frames; });
        auto *fpsTimer = new QTimer(window);
        QObject::connect(fpsTimer, &QTimer::timeout, window, [frames] {
            fprintf(stderr, "bdtv fps: %.1f\n", *frames / 5.0);
            *frames = 0;
        });
        fpsTimer->start(5000);
    }
    if (parser.isSet(exitAfterOpt))
        QTimer::singleShot(parser.value(exitAfterOpt).toInt(), &app, [] { QCoreApplication::exit(0); });

    return app.exec();
}
