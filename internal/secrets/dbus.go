// DBus: the Secret Service (keyring) store over the session bus
// (docs/security.md).

package secrets

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// Secret Service constants (freedesktop.org Secret Service API, S32).
const (
	secretsDest     = "org.freedesktop.secrets"
	servicePath     = dbus.ObjectPath("/org/freedesktop/secrets")
	ifaceService    = "org.freedesktop.Secret.Service"
	ifaceCollection = "org.freedesktop.Secret.Collection"
	ifaceItem       = "org.freedesktop.Secret.Item"
	ifaceSession    = "org.freedesktop.Secret.Session"
	ifaceProperties = "org.freedesktop.DBus.Properties"
	noObject        = dbus.ObjectPath("/")
	attrApplication = "bear-den-tv"
	plainAlgorithm  = "plain"
)

// Bus is the slice of a D-Bus session connection the Secret Service store
// uses. Production wraps *dbus.Conn; tests inject a fake that models
// collections, items, and locks.
type Bus interface {
	// Call invokes method ("interface.Member") on the org.freedesktop.secrets
	// object at path and stores the reply values into out.
	Call(ctx context.Context, path dbus.ObjectPath, method string, args []any, out ...any) error
	// Property reads iface.name from the object at path into out.
	Property(ctx context.Context, path dbus.ObjectPath, iface, name string, out any) error
}

// Secret is the D-Bus Secret struct (oayays): session, parameters, value,
// content type. With the plain algorithm parameters are empty and value is
// the secret bytes.
type Secret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// DBus stores secrets in the Secret Service default collection, looked up by
// the attributes {application: bear-den-tv, ref: <ref>}. It opens a plain
// session per operation and never drives unlock prompts: a locked collection
// yields ErrLocked with a user-facing reason.
type DBus struct {
	bus Bus
}

// NewDBus returns a Secret Service store over the given bus.
func NewDBus(bus Bus) *DBus { return &DBus{bus: bus} }

// OpenSessionBus connects to the user's session bus and returns a Secret
// Service store over it. The error wraps ErrUnavailable when no session bus
// exists (no DBUS_SESSION_BUS_ADDRESS, headless service).
func OpenSessionBus() (*DBus, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: no D-Bus session bus (%v)", ErrUnavailable, err)
	}
	return NewDBus(connBus{conn: conn}), nil
}

// Detect returns the Store for this process: the Secret Service when the
// session bus is reachable, otherwise Unavailable with the reason. It never
// fails. A reachable but locked or collection-less keyring still yields the
// DBus store so that every operation reports the precise ErrLocked or
// ErrUnavailable reason at the time it runs.
func Detect() Store {
	store, err := OpenSessionBus()
	if err != nil {
		return Unavailable{Reason: reason(err)}
	}
	return store
}

// connBus adapts *dbus.Conn to Bus.
type connBus struct{ conn *dbus.Conn }

func (b connBus) Call(ctx context.Context, path dbus.ObjectPath, method string, args []any, out ...any) error {
	call := b.conn.Object(secretsDest, path).CallWithContext(ctx, method, 0, args...)
	if call.Err != nil {
		return call.Err
	}
	if len(out) == 0 {
		return nil
	}
	return call.Store(out...)
}

func (b connBus) Property(ctx context.Context, path dbus.ObjectPath, iface, name string, out any) error {
	var v dbus.Variant
	if err := b.Call(ctx, path, ifaceProperties+".Get", []any{iface, name}, &v); err != nil {
		return err
	}
	return dbus.Store([]any{v.Value()}, out)
}

func attributes(ref string) map[string]string {
	return map[string]string{"application": attrApplication, "ref": ref}
}

func unavailable(err error) error {
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}

// defaultCollection resolves the "default" alias and checks its lock state.
func (s *DBus) defaultCollection(ctx context.Context) (dbus.ObjectPath, error) {
	var coll dbus.ObjectPath
	if err := s.bus.Call(ctx, servicePath, ifaceService+".ReadAlias", []any{"default"}, &coll); err != nil {
		return noObject, unavailable(err)
	}
	if coll == noObject || coll == "" {
		return noObject, fmt.Errorf("%w: the keyring has no default collection", ErrUnavailable)
	}
	var locked bool
	if err := s.bus.Property(ctx, coll, ifaceCollection, "Locked", &locked); err != nil {
		return noObject, unavailable(err)
	}
	if locked {
		return coll, lockedError()
	}
	return coll, nil
}

func (s *DBus) openSession(ctx context.Context) (dbus.ObjectPath, error) {
	var output dbus.Variant
	var session dbus.ObjectPath
	err := s.bus.Call(ctx, servicePath, ifaceService+".OpenSession", []any{plainAlgorithm, dbus.MakeVariant("")}, &output, &session)
	if err != nil {
		return noObject, unavailable(err)
	}
	if session == noObject || session == "" {
		return noObject, fmt.Errorf("%w: the keyring refused a plain session", ErrUnavailable)
	}
	return session, nil
}

func (s *DBus) closeSession(ctx context.Context, session dbus.ObjectPath) {
	_ = s.bus.Call(ctx, session, ifaceSession+".Close", nil)
}

func (s *DBus) search(ctx context.Context, ref string) (unlocked, locked []dbus.ObjectPath, err error) {
	if err := s.bus.Call(ctx, servicePath, ifaceService+".SearchItems", []any{attributes(ref)}, &unlocked, &locked); err != nil {
		return nil, nil, unavailable(err)
	}
	return unlocked, locked, nil
}

// Get implements Store.
func (s *DBus) Get(ctx context.Context, ref string) (string, error) {
	unlocked, locked, err := s.search(ctx, ref)
	if err != nil {
		return "", err
	}
	if len(unlocked) == 0 {
		if len(locked) > 0 {
			return "", lockedError()
		}
		return "", ErrNotFound
	}
	session, err := s.openSession(ctx)
	if err != nil {
		return "", err
	}
	defer s.closeSession(ctx, session)
	var secret Secret
	if err := s.bus.Call(ctx, unlocked[0], ifaceItem+".GetSecret", []any{session}, &secret); err != nil {
		return "", unavailable(err)
	}
	return string(secret.Value), nil
}

// Set implements Store.
func (s *DBus) Set(ctx context.Context, ref, value, label string) error {
	coll, err := s.defaultCollection(ctx)
	if err != nil {
		return err
	}
	session, err := s.openSession(ctx)
	if err != nil {
		return err
	}
	defer s.closeSession(ctx, session)
	props := map[string]dbus.Variant{
		ifaceItem + ".Label":      dbus.MakeVariant(label),
		ifaceItem + ".Attributes": dbus.MakeVariant(attributes(ref)),
	}
	secret := Secret{Session: session, Parameters: []byte{}, Value: []byte(value), ContentType: "text/plain"}
	var item, prompt dbus.ObjectPath
	if err := s.bus.Call(ctx, coll, ifaceCollection+".CreateItem", []any{props, secret, true}, &item, &prompt); err != nil {
		return unavailable(err)
	}
	if prompt != noObject && prompt != "" {
		return fmt.Errorf("%w: the keyring requires a prompt to store the item; %s", ErrLocked, lockedReason)
	}
	if item == noObject || item == "" {
		return fmt.Errorf("%w: the keyring did not create the item", ErrUnavailable)
	}
	return nil
}

// Delete implements Store.
func (s *DBus) Delete(ctx context.Context, ref string) error {
	unlocked, locked, err := s.search(ctx, ref)
	if err != nil {
		return err
	}
	if len(unlocked) == 0 {
		if len(locked) > 0 {
			return lockedError()
		}
		return ErrNotFound
	}
	for _, item := range unlocked {
		var prompt dbus.ObjectPath
		if err := s.bus.Call(ctx, item, ifaceItem+".Delete", nil, &prompt); err != nil {
			return unavailable(err)
		}
		if prompt != noObject && prompt != "" {
			return fmt.Errorf("%w: the keyring requires a prompt to delete the item; %s", ErrLocked, lockedReason)
		}
	}
	return nil
}

// Available implements Store.
func (s *DBus) Available(ctx context.Context) (bool, string) {
	if _, err := s.defaultCollection(ctx); err != nil {
		return false, reason(err)
	}
	return true, ""
}

// reason strips the sentinel prefix ("secret store locked: ") so the text
// reads as a user-facing sentence.
func reason(err error) string {
	msg := err.Error()
	for _, sentinel := range []error{ErrLocked, ErrUnavailable} {
		prefix := sentinel.Error() + ": "
		if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
			return msg[len(prefix):]
		}
	}
	return msg
}
