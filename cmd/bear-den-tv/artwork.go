// `bear-den-tv artwork`: fetch app tile icons (CLI guide internal/AGENTS.md).

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"bear-den-tv/internal/applications/flatpak"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/doctor"
)

// flathubIconURL is the only place `artwork fetch` downloads from: the
// official icon each application publishes with its Flathub listing.
const flathubIconURL = "https://dl.flathub.org/repo/appstream/x86_64/icons/128x128/%s.png"

const maxIconBytes = 1 << 20

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// cmdArtwork caches official application icons for apps that are not (yet)
// installed on this machine. It runs only when the owner asks; the shell
// prefers the owner's brand folder and the installed Flatpak's own icon.
func cmdArtwork(args []string) error {
	if len(args) == 0 || args[0] != "fetch" {
		return errors.New("usage: bear-den-tv artwork fetch [--config-dir DIR]")
	}
	paths := doctor.DefaultPaths()
	fs := flag.NewFlagSet("artwork", flag.ExitOnError)
	configDir := fs.String("config-dir", paths.ConfigDir, "configuration directory listing the registered apps")
	_ = fs.Parse(args[1:])

	store, err := config.Open(config.Options{Dir: *configDir})
	if err != nil {
		return err
	}
	cfg := config.Defaults()
	if _, err := os.Stat(store.Path()); err == nil {
		raw, err := os.ReadFile(store.Path())
		if err != nil {
			return err
		}
		if cfg, err = config.Parse(raw, config.Rules{}); err != nil {
			return fmt.Errorf("reading %s: %w", store.Path(), err)
		}
	}
	dir := filepath.Join(paths.CacheDir, "brand")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	var failed error
	for _, app := range cfg.Applications {
		id := app.Launch.AppID
		if err := flatpak.ValidateAppID(id); err != nil {
			failed = errors.Join(failed, err)
			continue
		}
		dest := filepath.Join(dir, id+".png")
		if err := fetchIcon(context.Background(), client, fmt.Sprintf(flathubIconURL, id), dest); err != nil {
			failed = errors.Join(failed, fmt.Errorf("%s: %w", app.Label, err))
			continue
		}
		fmt.Printf("%s: %s\n", app.Label, dest)
	}
	return failed
}

func fetchIcon(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("flathub answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxIconBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxIconBytes || !bytes.HasPrefix(body, pngMagic) {
		return errors.New("response is not a PNG icon")
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
