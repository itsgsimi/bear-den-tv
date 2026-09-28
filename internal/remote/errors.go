// Sentinel errors shared by the remote server and its backends.

package remote

import "errors"

// Pairing errors returned by Devices.Claim.
var (
	ErrInvalidInvitation = errors.New("invalid invitation")
	ErrInvitationExpired = errors.New("invitation expired")
	ErrTooManyAttempts   = errors.New("too many attempts")
	ErrRevisionConflict  = errors.New("revision conflict")
	ErrForbidden         = errors.New("forbidden")
	ErrTransport         = errors.New("not allowed on this transport")
)
