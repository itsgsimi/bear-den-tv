// SecretStore seam with memory and unavailable implementations
// (docs/security.md).

package storage

import (
	"context"
	"errors"
	"sync"
)

// ErrSecretNotFound is returned by SecretStore.Get for an unknown reference.
var ErrSecretNotFound = errors.New("storage: secret not found")

// ErrSecretsUnavailable wraps the reason a secret backend cannot be used
// (locked or absent keyring). A locked keyring never blocks launch or remote
// control; only the optional connector is disabled.
var ErrSecretsUnavailable = errors.New("storage: secret store unavailable")

// SecretStore holds connector credentials by opaque reference. config.json
// stores only the reference. Implementations: Memory (tests, dev), Unavailable
// (no usable backend), and a desktop Secret Service backend outside this package.
type SecretStore interface {
	Get(ctx context.Context, ref string) (string, error)
	Set(ctx context.Context, ref, secret string) error
	Delete(ctx context.Context, ref string) error
	// Available reports whether the backend can serve requests and, if not, why.
	Available() (bool, string)
}

// MemorySecrets keeps secrets in process memory only.
type MemorySecrets struct {
	mu sync.Mutex
	m  map[string]string
}

// NewMemorySecrets returns an empty in-memory store.
func NewMemorySecrets() *MemorySecrets { return &MemorySecrets{m: map[string]string{}} }

// Get implements SecretStore.
func (s *MemorySecrets) Get(_ context.Context, ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[ref]
	if !ok {
		return "", ErrSecretNotFound
	}
	return v, nil
}

// Set implements SecretStore.
func (s *MemorySecrets) Set(_ context.Context, ref, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[ref] = secret
	return nil
}

// Delete implements SecretStore.
func (s *MemorySecrets) Delete(_ context.Context, ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, ref)
	return nil
}

// Available implements SecretStore.
func (s *MemorySecrets) Available() (bool, string) { return true, "" }

// UnavailableSecrets refuses every operation with a stated reason.
type UnavailableSecrets struct {
	Reason string
}

// Get implements SecretStore.
func (u UnavailableSecrets) Get(context.Context, string) (string, error) {
	return "", errors.Join(ErrSecretsUnavailable, errors.New(u.Reason))
}

// Set implements SecretStore.
func (u UnavailableSecrets) Set(context.Context, string, string) error {
	return errors.Join(ErrSecretsUnavailable, errors.New(u.Reason))
}

// Delete implements SecretStore.
func (u UnavailableSecrets) Delete(context.Context, string) error {
	return errors.Join(ErrSecretsUnavailable, errors.New(u.Reason))
}

// Available implements SecretStore.
func (u UnavailableSecrets) Available() (bool, string) { return false, u.Reason }
