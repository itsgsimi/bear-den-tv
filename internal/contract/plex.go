// Plex sign-in state for the TV shell: state.schema.json#/properties/plex
// (shell view only; contracts/ipc.md plex.* messages drive it).

package contract

// Plex sign-in statuses (state.schema.json#/properties/plex/status).
const (
	PlexSignedOut       = "signed_out"
	PlexLinking         = "linking"
	PlexChooseServer    = "choose_server"
	PlexChooseLibraries = "choose_libraries"
	PlexConnected       = "connected"
	PlexError           = "error"
)

// Plex is state.schema.json#/properties/plex: where Settings → Plex is in
// the sign-in flow. The link code is for the TV screen only; the account
// token never appears in this struct.
type Plex struct {
	Status    string        `json:"status"`
	Message   string        `json:"message"`
	Code      *string       `json:"code"`
	LinkURL   *string       `json:"link_url"`
	Server    *string       `json:"server"`
	Servers   []PlexServer  `json:"servers"`
	Libraries []PlexLibrary `json:"libraries"`
}

// PlexServer is one server the signed-in account can use.
type PlexServer struct {
	ID    string `json:"id"` // machine identifier
	Name  string `json:"name"`
	Owned bool   `json:"owned"`
	Local bool   `json:"local"`
}

// PlexLibrary is one library on the chosen server. Kind is movie, show,
// artist, photo or other.
type PlexLibrary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Selected bool   `json:"selected"`
}
