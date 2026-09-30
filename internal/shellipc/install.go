// App install messages (contracts/ipc.md, app.install*, apps.configure,
// apps.browser): the TV's install card and Apps → Add apps start, size
// and cancel a per-user Flathub install of one registered app, Settings →
// Keep apps up to date stores config apps.auto_update, and Settings →
// Streaming sites stores the web apps' browsers (config apps.browser and
// apps.streaming_browser). Every one is answered with Result. The
// shell and the cli (`bear-den-tv apps install`) are the trusted senders;
// phones use the owner-only actions app.install and app.install_cancel.
// app.uninstall removes an app's Flatpak for this user (the TV's Remove,
// confirmed there); phones use the owner-only action app.uninstall.

package shellipc

// App install message type names.
const (
	TypeAppInstall       = "app.install"
	TypeAppInstallInfo   = "app.install_info"
	TypeAppInstallCancel = "app.install_cancel"
	TypeAppUninstall     = "app.uninstall"
	TypeAppsConfigure    = "apps.configure"
	TypeAppsBrowser      = "apps.browser"
)

// AppInstall starts installing the Flatpak of a registered application
// (by its config id) for this user from Flathub. The reply comes once the
// install has started; progress is in state.applications[].install.
// Enable says the owner pressed Install on that app's own card (its tile or
// its Streaming sites row, never a shared row of Settings → Add apps): the
// app is turned on (config applications[].enabled) once the install is
// done, before its tile lights up, so the shell can open it.
type AppInstall struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	AppID     string `json:"app_id"`
	Enable    bool   `json:"enable,omitempty"`
}

// Kind implements Message.
func (AppInstall) Kind() string { return TypeAppInstall }

// AppInstallInfo asks Flathub how big an app's install is (the install card
// opening on the TV). The reply's Data carries size_bytes and disk_bytes;
// state.applications[].install carries them too.
type AppInstallInfo struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	AppID     string `json:"app_id"`
}

// Kind implements Message.
func (AppInstallInfo) Kind() string { return TypeAppInstallInfo }

// AppInstallCancel stops a running install of an application's Flatpak.
type AppInstallCancel struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	AppID     string `json:"app_id"`
}

// Kind implements Message.
func (AppInstallCancel) Kind() string { return TypeAppInstallCancel }

// AppUninstall removes the Flatpak of a registered application (by its
// config id) for this user: `flatpak uninstall --user`, never system-wide.
// DeleteData also deletes the app's own data (~/.var/app/<id>). The reply
// comes once the removal has started; state.applications[].install is
// "removing" while it runs, and the app is no longer installed afterwards.
type AppUninstall struct {
	Type       string `json:"type"`
	RequestID  string `json:"request_id"`
	AppID      string `json:"app_id"`
	DeleteData bool   `json:"delete_data,omitempty"`
}

// Kind implements Message.
func (AppUninstall) Kind() string { return TypeAppUninstall }

// AppsConfigure stores config apps.auto_update (TV Settings → Keep apps up
// to date).
type AppsConfigure struct {
	Type       string `json:"type"`
	RequestID  string `json:"request_id"`
	AutoUpdate bool   `json:"auto_update"`
}

// Kind implements Message.
func (AppsConfigure) Kind() string { return TypeAppsConfigure }

// AppsBrowser stores the web apps' browsers: Browser for the Browser tile
// and StreamingBrowser for the streaming sites, each a browser name from
// the adapter table (state.apps.browsers[].id). Both are sent every time.
type AppsBrowser struct {
	Type             string `json:"type"`
	RequestID        string `json:"request_id"`
	Browser          string `json:"browser"`
	StreamingBrowser string `json:"streaming_browser"`
}

// Kind implements Message.
func (AppsBrowser) Kind() string { return TypeAppsBrowser }

// decodeInstall returns an empty message for an app install type, or nil.
func decodeInstall(t string) Message {
	switch t {
	case TypeAppInstall:
		return &AppInstall{}
	case TypeAppInstallInfo:
		return &AppInstallInfo{}
	case TypeAppInstallCancel:
		return &AppInstallCancel{}
	case TypeAppUninstall:
		return &AppUninstall{}
	case TypeAppsConfigure:
		return &AppsConfigure{}
	case TypeAppsBrowser:
		return &AppsBrowser{}
	}
	return nil
}

// derefInstall returns the value form of an app install message, or nil.
func derefInstall(m Message) Message {
	switch t := m.(type) {
	case *AppInstall:
		return *t
	case *AppInstallInfo:
		return *t
	case *AppInstallCancel:
		return *t
	case *AppUninstall:
		return *t
	case *AppsConfigure:
		return *t
	case *AppsBrowser:
		return *t
	}
	return nil
}
