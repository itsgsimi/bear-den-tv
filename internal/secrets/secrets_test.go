// Tests for the Secret Service store with a fake bus (dbus.go, memory.go).

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakeBus models one Secret Service with a default collection, a lock flag,
// and items addressed by attributes. It answers exactly the calls DBus makes.
type fakeBus struct {
	noDefault  bool
	locked     bool
	down       error
	items      map[dbus.ObjectPath]*fakeItem
	nextItem   int
	sessions   int
	closed     int
	promptOnCr bool
}

type fakeItem struct {
	attrs map[string]string
	value []byte
	label string
}

const (
	fakeCollection = dbus.ObjectPath("/org/freedesktop/secrets/collection/login")
	fakeSession    = dbus.ObjectPath("/org/freedesktop/secrets/session/s1")
)

func newFakeBus() *fakeBus { return &fakeBus{items: map[dbus.ObjectPath]*fakeItem{}} }

func (b *fakeBus) Call(ctx context.Context, path dbus.ObjectPath, method string, args []any, out ...any) error {
	if b.down != nil {
		return b.down
	}
	switch method {
	case ifaceService + ".ReadAlias":
		if b.noDefault {
			return dbus.Store([]any{noObject}, out...)
		}
		return dbus.Store([]any{fakeCollection}, out...)
	case ifaceService + ".OpenSession":
		if args[0].(string) != plainAlgorithm {
			return fmt.Errorf("unsupported algorithm %v", args[0])
		}
		b.sessions++
		return dbus.Store([]any{dbus.MakeVariant(""), fakeSession}, out...)
	case ifaceSession + ".Close":
		b.closed++
		return nil
	case ifaceService + ".SearchItems":
		want := args[0].(map[string]string)
		var unlocked, locked []dbus.ObjectPath
		for p, it := range b.items {
			if it.attrs["application"] == want["application"] && it.attrs["ref"] == want["ref"] {
				if b.locked {
					locked = append(locked, p)
				} else {
					unlocked = append(unlocked, p)
				}
			}
		}
		return dbus.Store([]any{unlocked, locked}, out...)
	case ifaceItem + ".GetSecret":
		it, ok := b.items[path]
		if !ok {
			return errors.New("no such item")
		}
		if args[0].(dbus.ObjectPath) != fakeSession {
			return errors.New("bad session")
		}
		return dbus.Store([]any{Secret{Session: fakeSession, Parameters: []byte{}, Value: it.value, ContentType: "text/plain"}}, out...)
	case ifaceCollection + ".CreateItem":
		if path != fakeCollection {
			return errors.New("no such collection")
		}
		props := args[0].(map[string]dbus.Variant)
		secret := args[1].(Secret)
		if b.promptOnCr {
			return dbus.Store([]any{noObject, dbus.ObjectPath("/org/freedesktop/secrets/prompt/p1")}, out...)
		}
		attrs := props[ifaceItem+".Attributes"].Value().(map[string]string)
		label := props[ifaceItem+".Label"].Value().(string)
		if args[2].(bool) {
			for p, it := range b.items {
				if it.attrs["ref"] == attrs["ref"] {
					delete(b.items, p)
				}
			}
		}
		b.nextItem++
		p := dbus.ObjectPath(fmt.Sprintf("%s/%d", fakeCollection, b.nextItem))
		b.items[p] = &fakeItem{attrs: attrs, value: append([]byte(nil), secret.Value...), label: label}
		return dbus.Store([]any{p, noObject}, out...)
	case ifaceItem + ".Delete":
		if _, ok := b.items[path]; !ok {
			return errors.New("no such item")
		}
		delete(b.items, path)
		return dbus.Store([]any{noObject}, out...)
	}
	return fmt.Errorf("unexpected call %s on %s", method, path)
}

func (b *fakeBus) Property(ctx context.Context, path dbus.ObjectPath, iface, name string, out any) error {
	if b.down != nil {
		return b.down
	}
	if path == fakeCollection && iface == ifaceCollection && name == "Locked" {
		return dbus.Store([]any{b.locked}, out)
	}
	return fmt.Errorf("unexpected property %s.%s on %s", iface, name, path)
}

func TestDBusRoundTrip(t *testing.T) {
	bus := newFakeBus()
	store := NewDBus(bus)
	ctx := context.Background()
	if _, err := store.Get(ctx, "plex/main"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get before Set: %v", err)
	}
	if err := store.Set(ctx, "plex/main", "tok-1", "Bear Den TV Plex"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get(ctx, "plex/main")
	if err != nil || got != "tok-1" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := store.Set(ctx, "plex/main", "tok-2", "Bear Den TV Plex"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if len(bus.items) != 1 {
		t.Fatalf("replace must not duplicate items: %d", len(bus.items))
	}
	if got, _ := store.Get(ctx, "plex/main"); got != "tok-2" {
		t.Fatalf("Get after replace = %q", got)
	}
	for _, it := range bus.items {
		if it.attrs["application"] != attrApplication || it.attrs["ref"] != "plex/main" || it.label != "Bear Den TV Plex" {
			t.Fatalf("item attributes/label: %+v", it)
		}
	}
	if err := store.Delete(ctx, "plex/main"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "plex/main"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}
	if bus.sessions != bus.closed {
		t.Fatalf("sessions opened %d closed %d", bus.sessions, bus.closed)
	}
	if ok, reason := store.Available(ctx); !ok || reason != "" {
		t.Fatalf("Available = %v %q", ok, reason)
	}
}

func TestDBusLockedCollection(t *testing.T) {
	bus := newFakeBus()
	store := NewDBus(bus)
	ctx := context.Background()
	if err := store.Set(ctx, "plex/main", "tok", "label"); err != nil {
		t.Fatal(err)
	}
	bus.locked = true
	if _, err := store.Get(ctx, "plex/main"); !errors.Is(err, ErrLocked) {
		t.Fatalf("Get locked: %v", err)
	} else if !strings.Contains(err.Error(), "unlock it") {
		t.Fatalf("locked error must explain: %v", err)
	}
	if err := store.Set(ctx, "plex/other", "tok", "label"); !errors.Is(err, ErrLocked) {
		t.Fatalf("Set locked: %v", err)
	}
	if err := store.Delete(ctx, "plex/main"); !errors.Is(err, ErrLocked) {
		t.Fatalf("Delete locked: %v", err)
	}
	ok, reason := store.Available(ctx)
	if ok || !strings.Contains(reason, "locked") || strings.HasPrefix(reason, ErrLocked.Error()) {
		t.Fatalf("Available locked = %v %q", ok, reason)
	}
	if len(bus.items) != 1 {
		t.Fatalf("locked operations must not mutate: %d items", len(bus.items))
	}
}

func TestDBusPromptRefused(t *testing.T) {
	bus := newFakeBus()
	bus.promptOnCr = true
	store := NewDBus(bus)
	err := store.Set(context.Background(), "plex/main", "tok", "label")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("prompt must map to ErrLocked: %v", err)
	}
	if len(bus.items) != 0 {
		t.Fatal("nothing stored when a prompt is required")
	}
}

func TestDBusUnavailable(t *testing.T) {
	ctx := context.Background()
	bus := newFakeBus()
	bus.noDefault = true
	store := NewDBus(bus)
	if err := store.Set(ctx, "r", "v", "l"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no default collection: %v", err)
	}
	if ok, reason := store.Available(ctx); ok || !strings.Contains(reason, "default collection") {
		t.Fatalf("Available = %v %q", ok, reason)
	}
	bus = newFakeBus()
	bus.down = errors.New("The name org.freedesktop.secrets was not provided by any .service files")
	store = NewDBus(bus)
	if _, err := store.Get(ctx, "r"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("bus down: %v", err)
	}
	if ok, reason := store.Available(ctx); ok || !strings.Contains(reason, "org.freedesktop.secrets") {
		t.Fatalf("Available down = %v %q", ok, reason)
	}
}

func TestDBusNeverLeaksValueInErrors(t *testing.T) {
	bus := newFakeBus()
	store := NewDBus(bus)
	ctx := context.Background()
	_ = store.Set(ctx, "plex/main", "super-secret-token", "label")
	bus.locked = true
	_, err := store.Get(ctx, "plex/main")
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatal("error text carries the secret")
	}
}

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if _, err := m.Get(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := m.Set(ctx, "a", "v", "l"); err != nil {
		t.Fatal(err)
	}
	if v, err := m.Get(ctx, "a"); err != nil || v != "v" {
		t.Fatal(v, err)
	}
	m.SetLocked(true)
	if _, err := m.Get(ctx, "a"); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	if ok, reason := m.Available(ctx); ok || reason == "" {
		t.Fatal(ok, reason)
	}
	m.SetLocked(false)
	if err := m.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestUnavailableStore(t *testing.T) {
	ctx := context.Background()
	u := Unavailable{Reason: "no D-Bus session bus"}
	if _, err := u.Get(ctx, "a"); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "no D-Bus session bus") {
		t.Fatal(err)
	}
	if err := u.Set(ctx, "a", "v", "l"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if err := u.Delete(ctx, "a"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if ok, reason := u.Available(ctx); ok || reason != "no D-Bus session bus" {
		t.Fatal(ok, reason)
	}
}

func TestDetectWithoutSessionBus(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bear-den-tv-test.sock")
	store := Detect()
	if _, ok := store.(Unavailable); !ok {
		t.Fatalf("Detect without a bus must return Unavailable, got %T", store)
	}
	if ok, reason := store.Available(context.Background()); ok || reason == "" {
		t.Fatal(ok, reason)
	}
}
