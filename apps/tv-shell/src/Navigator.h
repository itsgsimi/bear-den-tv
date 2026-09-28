#pragma once
// Navigator: the single entry point for navigation input, whether it arrives as a physical
// key on the shell window or as an IPC `input` message from the coordinator.
//
// Contract: physical keys are intercepted by an application-wide event filter, normalized to
// the named actions of contracts/actions.md (nav.*, select, back, home, text.submit) and
// re-dispatched to the window as canonical synthetic keys (arrows, Return, Escape), so QML
// only ever handles one key vocabulary. `apply` performs an action synchronously and returns
// the `input_result` shape: outcome/code plus detail {screen, section_id, item_id} read from
// the focus QML reported through `reportFocus`. `text.submit` is delivered only into a
// focused text field; everything else is `failed/unsupported`. Back at the root never exits.
#include <QObject>
#include <QPointer>
#include <QVariantMap>
#include <QtQml/qqmlregistration.h>

class QQmlEngine;
class QJSEngine;
class QWindow;

class Navigator : public QObject {
    Q_OBJECT
    QML_NAMED_ELEMENT(Nav)
    QML_SINGLETON
    Q_PROPERTY(QString screen READ screen WRITE setScreen NOTIFY focusChanged)
    Q_PROPERTY(QString sectionId READ sectionId NOTIFY focusChanged)
    Q_PROPERTY(QString itemId READ itemId NOTIFY focusChanged)
    Q_PROPERTY(qreal scrollX READ scrollX NOTIFY focusChanged)
    Q_PROPERTY(bool textFieldFocused READ textFieldFocused NOTIFY focusChanged)

public:
    // Private: QML must obtain the shared instance through create(); a public
    // default constructor would make the engine build its own copy.
private:
    explicit Navigator(QObject *parent = nullptr);
public:
    ~Navigator() override;
    static Navigator *create(QQmlEngine *, QJSEngine *);
    static Navigator *instance();

    /// The window that receives synthetic keys and activation requests. Defaults to the
    /// application's focus window when unset.
    void setWindow(QWindow *window);

    QString screen() const { return m_screen; }
    void setScreen(const QString &screen);
    QString sectionId() const { return m_sectionId; }
    QString itemId() const { return m_itemId; }
    qreal scrollX() const { return m_scrollX; }
    bool textFieldFocused() const;

    /// Applies a named action and returns {outcome, code, message, detail{screen, section_id,
    /// item_id[, at_root]}}. Unknown actions fail with `unsupported`.
    Q_INVOKABLE QVariantMap apply(const QString &action, const QVariantMap &args = {});
    /// QML reports the focused stable ids here on every focus change; forwarded as `focus`.
    Q_INVOKABLE void reportFocus(const QString &sectionId, const QString &itemId, qreal scrollX = 0);
    /// QML marks that a Back press reached the root and was a no-op.
    Q_INVOKABLE void noteAtRoot();
    /// Maps a Qt key to an action name, or empty when the key is not a navigation key.
    Q_INVOKABLE QString actionForKey(int key) const;

    bool eventFilter(QObject *watched, QEvent *event) override;

signals:
    /// Emitted after every reportFocus and after setScreen.
    void focusChanged();
    /// Same as focusChanged but with the payload the `focus` IPC message needs
    /// (text_field: a QQuickTextInput/TextEdit has focus; re-emitted when that alone changes).
    void focusReported(const QString &screen, const QString &sectionId, const QString &itemId, qreal scrollX, bool textField);
    /// The `home` action: QML closes dialogs, returns to the home screen, restores focus.
    void homeRequested();
    /// Emitted for every applied action (diagnostics/tests).
    void actionApplied(const QString &action, const QVariantMap &result);

private:
    QWindow *targetWindow() const;
    bool dispatchKey(int key, const QString &text = QString());
    QVariantMap result(const QString &outcome, const QString &code, const QString &message) const;
    /// Re-reads whether a text field has focus; emits a new focus report when it changed.
    void refreshTextField();

    QPointer<QWindow> m_window;
    QString m_screen{QStringLiteral("home")};
    QString m_sectionId;
    QString m_itemId;
    qreal m_scrollX = 0;
    bool m_atRoot = false;
    bool m_textField = false; // last reported `text_field`
    QMetaObject::Connection m_focusItemConnection;
};
