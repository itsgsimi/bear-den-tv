// Memory and Unavailable secret stores for tests and hosts without a keyring.

package secrets

import (
	"context"
	"fmt"
	"sync"
)

// Memory is an in-process Store for tests and --dev runs. Values never leave
// the process. SetLocked simulates a locked keyring.
type Memory struct {
	mu     sync.Mutex
	values map[string]string
	locked bool
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory { return &Memory{values: map[string]string{}} }

// SetLocked makes every operation fail with ErrLocked while locked is true.
func (m *Memory) SetLocked(locked bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locked = locked
}

// Get implements Store.
func (m *Memory) Get(ctx context.Context, ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked {
		return "", lockedError()
	}
	v, ok := m.values[ref]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set implements Store.
func (m *Memory) Set(ctx context.Context, ref, value, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked {
		return lockedError()
	}
	m.values[ref] = value
	return nil
}

// Delete implements Store.
func (m *Memory) Delete(ctx context.Context, ref string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked {
		return lockedError()
	}
	if _, ok := m.values[ref]; !ok {
		return ErrNotFound
	}
	delete(m.values, ref)
	return nil
}

// Available implements Store.
func (m *Memory) Available(ctx context.Context) (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked {
		return false, lockedReason
	}
	return true, ""
}

// Unavailable is a Store that always fails with ErrUnavailable and the given
// reason. The coordinator uses it when no keyring can be reached so the
// connector reports a clear message instead of a nil store.
type Unavailable struct {
	Reason string
}

// Get implements Store.
func (u Unavailable) Get(ctx context.Context, ref string) (string, error) {
	return "", u.err()
}

// Set implements Store.
func (u Unavailable) Set(ctx context.Context, ref, value, label string) error { return u.err() }

// Delete implements Store.
func (u Unavailable) Delete(ctx context.Context, ref string) error { return u.err() }

// Available implements Store.
func (u Unavailable) Available(ctx context.Context) (bool, string) { return false, u.Reason }

func (u Unavailable) err() error { return fmt.Errorf("%w: %s", ErrUnavailable, u.Reason) }

const lockedReason = "the default keyring is locked; unlock it in the desktop session and retry"

func lockedError() error { return fmt.Errorf("%w: %s", ErrLocked, lockedReason) }
