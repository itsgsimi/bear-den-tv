// Plex sign-in messages (contracts/ipc.md, plex.*): Settings → Plex on the
// TV drives the coordinator's sign-in flow; every message is answered with
// Result and a new state (state.plex) follows. Trusted local senders only.

package shellipc

// Plex message type names.
const (
	TypePlexSignIn          = "plex.sign_in"
	TypePlexCancel          = "plex.cancel"
	TypePlexChooseServer    = "plex.choose_server"
	TypePlexChooseLibraries = "plex.choose_libraries"
	TypePlexSignOut         = "plex.sign_out"
)

// PlexSignIn starts (or restarts) linking: the coordinator asks plex.tv for a
// link code and state.plex shows it.
type PlexSignIn struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// Kind implements Message.
func (PlexSignIn) Kind() string { return TypePlexSignIn }

// PlexCancel abandons linking or choosing; nothing is stored.
type PlexCancel struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// Kind implements Message.
func (PlexCancel) Kind() string { return TypePlexCancel }

// PlexChooseServer picks one of state.plex.servers by id.
type PlexChooseServer struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	ServerID  string `json:"server_id"`
}

// Kind implements Message.
func (PlexChooseServer) Kind() string { return TypePlexChooseServer }

// PlexChooseLibraries finishes sign-in with the libraries rows should use
// (ids from state.plex.libraries, at least one).
type PlexChooseLibraries struct {
	Type       string   `json:"type"`
	RequestID  string   `json:"request_id"`
	LibraryIDs []string `json:"library_ids"`
}

// Kind implements Message.
func (PlexChooseLibraries) Kind() string { return TypePlexChooseLibraries }

// PlexSignOut forgets the account: the token leaves the keyring and the
// private file (internal/secrets), rows and
// cached artwork go.
type PlexSignOut struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// Kind implements Message.
func (PlexSignOut) Kind() string { return TypePlexSignOut }

// decodePlex returns an empty message for a plex.* type, or nil.
func decodePlex(t string) Message {
	switch t {
	case TypePlexSignIn:
		return &PlexSignIn{}
	case TypePlexCancel:
		return &PlexCancel{}
	case TypePlexChooseServer:
		return &PlexChooseServer{}
	case TypePlexChooseLibraries:
		return &PlexChooseLibraries{}
	case TypePlexSignOut:
		return &PlexSignOut{}
	}
	return nil
}

// derefPlex returns the value form of a plex.* message, or nil.
func derefPlex(m Message) Message {
	switch t := m.(type) {
	case *PlexSignIn:
		return *t
	case *PlexCancel:
		return *t
	case *PlexChooseServer:
		return *t
	case *PlexChooseLibraries:
		return *t
	case *PlexSignOut:
		return *t
	}
	return nil
}
