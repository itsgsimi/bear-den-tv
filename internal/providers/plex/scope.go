// Scope and owner keys, and the persistent Plex client identifier.

package plex

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ScopeKey names the cache scope for one server library: the hex SHA-256 of
// the server machineIdentifier and library id. Every cached artwork file
// lives under exactly one scope so a sign-out or account change purges only
// its own data (spec §7.2).
func ScopeKey(machineIdentifier, libraryID string) string {
	sum := sha256.Sum256([]byte("plex-scope\x00" + machineIdentifier + "\x00" + libraryID))
	return hex.EncodeToString(sum[:])
}

// OwnerKey names every scope of one server so PurgeOwner can drop them
// together without recording the machineIdentifier on disk.
func OwnerKey(machineIdentifier string) string {
	sum := sha256.Sum256([]byte("plex-owner\x00" + machineIdentifier))
	return hex.EncodeToString(sum[:])
}

var clientIdentifierPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// LoadOrCreateClientIdentifier returns the stable per-install
// X-Plex-Client-Identifier stored at path, creating a random one (32 hex
// characters, mode 0600) when the file is missing. The value is not a secret
// but must not change between runs: Plex ties issued tokens to it.
func LoadOrCreateClientIdentifier(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if !clientIdentifierPattern.MatchString(id) {
			return "", fmt.Errorf("plex: client identifier file %s is not 32 hex characters", path)
		}
		return id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return id, nil
}
