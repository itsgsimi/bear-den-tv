// Navigator implementation: keys and IPC input become focus moves and
// activations (contract in Navigator.h; guide apps/tv-shell/AGENTS.md).

#include "Navigator.h"

#include <QCoreApplication>
#include <QGuiApplication>
#include <QJSEngine>
#include <QKeyEvent>
#include <QQmlEngine>
#include <QQuickWindow>
#include <QWindow>

namespace {
Navigator *g_instance = nullptr;

bool isTextInput(const QObject *focus)
{
    return focus && (focus->inherits("QQuickTextInput") || focus->inherits("QQuickTextEdit"));
}
} // namespace

Navigator::Navigator(QObject *parent) : QObject(parent)
{
    if (!g_instance)
        g_instance = this;
    if (qApp)
        qApp->installEventFilter(this);
}

Navigator::~Navigator()
{
    if (g_instance == this)
        g_instance = nullptr;
}

Navigator *Navigator::create(QQmlEngine *, QJSEngine *)
{
    if (!g_instance)
        g_instance = new Navigator();
    QJSEngine::setObjectOwnership(g_instance, QJSEngine::CppOwnership);
    return g_instance;
}

Navigator *Navigator::instance()
{
    return g_instance;
}

void Navigator::setWindow(QWindow *window)
{
    if (m_focusItemConnection)
        disconnect(m_focusItemConnection);
    m_window = window;
    if (auto *quick = qobject_cast<QQuickWindow *>(window))
        m_focusItemConnection = connect(quick, &QQuickWindow::activeFocusItemChanged, this, &Navigator::refreshTextField);
}

void Navigator::refreshTextField()
{
    const bool now = textFieldFocused();
    if (now == m_textField)
        return;
    m_textField = now;
    emit focusChanged();
    emit focusReported(m_screen, m_sectionId, m_itemId, m_scrollX, m_textField);
}

void Navigator::setScreen(const QString &screen)
{
    if (m_screen == screen)
        return;
    m_screen = screen;
    emit focusChanged();
    m_textField = textFieldFocused();
    emit focusReported(m_screen, m_sectionId, m_itemId, m_scrollX, m_textField);
}

bool Navigator::textFieldFocused() const
{
    QWindow *window = targetWindow();
    return window && isTextInput(window->focusObject());
}

QWindow *Navigator::targetWindow() const
{
    if (m_window)
        return m_window;
    if (QWindow *focus = QGuiApplication::focusWindow())
        return focus;
    const auto windows = QGuiApplication::topLevelWindows();
    for (QWindow *w : windows)
        if (qobject_cast<QQuickWindow *>(w))
            return w;
    return nullptr;
}

QString Navigator::actionForKey(int key) const
{
    switch (key) {
    case Qt::Key_Up: return QStringLiteral("nav.up");
    case Qt::Key_Down: return QStringLiteral("nav.down");
    case Qt::Key_Left: return QStringLiteral("nav.left");
    case Qt::Key_Right: return QStringLiteral("nav.right");
    case Qt::Key_Return:
    case Qt::Key_Enter:
    case Qt::Key_Select:
    case Qt::Key_Space: return QStringLiteral("select");
    case Qt::Key_Escape:
    case Qt::Key_Back:
    case Qt::Key_Backspace: return QStringLiteral("back");
    case Qt::Key_HomePage:
    case Qt::Key_Home: return QStringLiteral("home");
    default: return QString();
    }
}

bool Navigator::eventFilter(QObject *watched, QEvent *event)
{
    if (!event->spontaneous() || !watched->isWindowType())
        return false;
    if (event->type() != QEvent::KeyPress && event->type() != QEvent::KeyRelease)
        return false;
    auto *keyEvent = static_cast<QKeyEvent *>(event);
    const QString action = actionForKey(keyEvent->key());
    if (action.isEmpty())
        return false;
    // Inside a text field the editing keys keep their meaning; only D-pad-style keys are ours.
    if (isTextInput(static_cast<QWindow *>(watched)->focusObject())) {
        switch (keyEvent->key()) {
        case Qt::Key_Left:
        case Qt::Key_Right:
        case Qt::Key_Backspace:
        case Qt::Key_Home:
        case Qt::Key_Space:
            return false;
        default:
            break;
        }
    }
    if (event->type() == QEvent::KeyRelease)
        return true; // the synthetic pair already delivered a release
    apply(action);
    return true;
}

bool Navigator::dispatchKey(int key, const QString &text)
{
    QWindow *window = targetWindow();
    if (!window)
        return false;
    QKeyEvent press(QEvent::KeyPress, key, Qt::NoModifier, text);
    QCoreApplication::sendEvent(window, &press);
    QKeyEvent release(QEvent::KeyRelease, key, Qt::NoModifier, text);
    QCoreApplication::sendEvent(window, &release);
    return press.isAccepted();
}

QVariantMap Navigator::result(const QString &outcome, const QString &code, const QString &message) const
{
    QVariantMap detail{
        {QStringLiteral("screen"), m_screen},
        {QStringLiteral("section_id"), m_sectionId.isEmpty() ? QVariant() : QVariant(m_sectionId)},
        {QStringLiteral("item_id"), m_itemId.isEmpty() ? QVariant() : QVariant(m_itemId)},
    };
    if (m_atRoot)
        detail.insert(QStringLiteral("at_root"), true);
    return {
        {QStringLiteral("outcome"), outcome},
        {QStringLiteral("code"), code},
        {QStringLiteral("message"), message},
        {QStringLiteral("detail"), detail},
    };
}

QVariantMap Navigator::apply(const QString &action, const QVariantMap &args)
{
    m_atRoot = false;
    QVariantMap out;
    if (action == QLatin1String("nav.up")) {
        dispatchKey(Qt::Key_Up);
        out = result(QStringLiteral("observed"), QStringLiteral("ok"), QString());
    } else if (action == QLatin1String("nav.down")) {
        dispatchKey(Qt::Key_Down);
        out = result(QStringLiteral("observed"), QStringLiteral("ok"), QString());
    } else if (action == QLatin1String("nav.left")) {
        dispatchKey(Qt::Key_Left);
        out = result(QStringLiteral("observed"), QStringLiteral("ok"), QString());
    } else if (action == QLatin1String("nav.right")) {
        dispatchKey(Qt::Key_Right);
        out = result(QStringLiteral("observed"), QStringLiteral("ok"), QString());
    } else if (action == QLatin1String("select")) {
        dispatchKey(Qt::Key_Return);
        out = result(QStringLiteral("observed"), QStringLiteral("ok"), QString());
    } else if (action == QLatin1String("back")) {
        dispatchKey(Qt::Key_Escape);
        out = result(QStringLiteral("observed"), QStringLiteral("ok"), m_atRoot ? QStringLiteral("at_root") : QString());
    } else if (action == QLatin1String("home")) {
        emit homeRequested();
        if (QWindow *window = targetWindow())
            window->requestActivate();
        out = result(QStringLiteral("observed"), QStringLiteral("ok"), QString());
    } else if (action == QLatin1String("text.submit")) {
        const QString text = args.value(QStringLiteral("text")).toString();
        const bool clean = !text.isEmpty() && text.size() <= 256
            && std::none_of(text.cbegin(), text.cend(), [](QChar c) { return c.category() == QChar::Other_Control; });
        QWindow *window = targetWindow();
        QObject *focus = window ? window->focusObject() : nullptr;
        if (!clean) {
            out = result(QStringLiteral("failed"), QStringLiteral("invalid"), QStringLiteral("text must be 1..256 characters without control characters"));
        } else if (!isTextInput(focus)) {
            out = result(QStringLiteral("failed"), QStringLiteral("unsupported"), QStringLiteral("No text field is focused."));
        } else {
            focus->setProperty("text", text);
            dispatchKey(Qt::Key_Return);
            out = result(QStringLiteral("observed"), QStringLiteral("ok"), QString());
        }
    } else {
        out = result(QStringLiteral("failed"), QStringLiteral("unsupported"), QStringLiteral("Unknown shell action '%1'").arg(action));
    }
    emit actionApplied(action, out);
    return out;
}

void Navigator::reportFocus(const QString &sectionId, const QString &itemId, qreal scrollX)
{
    if (m_sectionId == sectionId && m_itemId == itemId && qFuzzyCompare(m_scrollX + 1, scrollX + 1))
        return;
    m_sectionId = sectionId;
    m_itemId = itemId;
    m_scrollX = scrollX;
    emit focusChanged();
    m_textField = textFieldFocused();
    emit focusReported(m_screen, m_sectionId, m_itemId, m_scrollX, m_textField);
}

void Navigator::noteAtRoot()
{
    m_atRoot = true;
}
