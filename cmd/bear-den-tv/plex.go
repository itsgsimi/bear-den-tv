// `bear-den-tv plex ...` (Plex sign-in on the running coordinator over IPC,
// contracts/ipc.md plex.*) and the session wiring of internal/plexlink,
// including the dev-only --dev-plex-fake (docs/operations.md, "Plex").

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/doctor"
	"bear-den-tv/internal/plexlink"
	"bear-den-tv/internal/providers/plex"
	"bear-den-tv/internal/providers/plex/plexfake"
	"bear-den-tv/internal/secrets"
	"bear-den-tv/internal/shellipc"
)

const plexUsage = `usage: bear-den-tv plex status | sign-in | server ID | libraries ID... | cancel | sign-out`

// newPlexLink builds the Plex connector for a session. A real session gets
// the desktop keyring (or, when it is locked or missing, a private file under
// $XDG_DATA_HOME/bear-den-tv/secrets) and plex.tv, but no traffic until the owner signs in.
// dev gets one only with --dev-plex-fake: an in-memory keyring and a local
// fake plex.tv/server with DEMO titles, which links a code on its third poll.
// The returned stop closes the fake.
func newPlexLink(f sessionFlags, paths doctor.Paths, store *config.Store, log *slog.Logger) (*plexlink.Manager, func(), error) {
	stop := func() {}
	opts := plexlink.Options{
		Config: store, Logger: log,
		ArtworkDir:    filepath.Join(paths.CacheDir, "plex-artwork"),
		ArtworkBudget: int64(store.Current().Cache.ArtworkMaxMiB) << 20,
	}
	switch {
	case f.dev && f.devPlexFake:
		fake, err := plexfake.New(plexfake.Options{LinkAfterPolls: 3})
		if err != nil {
			return nil, stop, err
		}
		stop = fake.Close
		opts.AccountURL = fake.URL
		opts.Secrets = secrets.NewMemory()
		opts.ClientIdentifier = "000000000000000000000000000de300"
		log.Info("dev: Plex sign-in talks to a local fake with DEMO titles", "url", fake.URL)
	case f.dev:
		return nil, stop, nil // dev never contacts plex.tv
	default:
		id, err := plex.LoadOrCreateClientIdentifier(filepath.Join(paths.DataDir, "plex-client-id"))
		if err != nil {
			return nil, stop, err
		}
		opts.ClientIdentifier = id
		// The desktop keyring when it is unlocked, else a private file
		// (a TV that logs in automatically never unlocks its keyring).
		opts.Secrets = secrets.NewFallback(secrets.Detect(), secrets.NewFile(filepath.Join(paths.DataDir, "secrets"), "plex-"))
	}
	m, err := plexlink.New(opts)
	if err != nil {
		stop()
		return nil, func() {}, err
	}
	return m, stop, nil
}

func cmdPlex(args []string) error {
	if len(args) == 0 {
		return errors.New(plexUsage)
	}
	fs := flag.NewFlagSet("plex "+args[0], flag.ExitOnError)
	sock := socketFlag(fs)
	_ = fs.Parse(args[1:])
	rest := fs.Args()
	id := fmt.Sprintf("cli-plex-%d", time.Now().UnixNano())
	var msg shellipc.Message
	switch args[0] {
	case "status":
		return plexStatus(*sock)
	case "sign-in":
		msg = shellipc.PlexSignIn{Type: shellipc.TypePlexSignIn, RequestID: id}
	case "cancel":
		msg = shellipc.PlexCancel{Type: shellipc.TypePlexCancel, RequestID: id}
	case "sign-out":
		msg = shellipc.PlexSignOut{Type: shellipc.TypePlexSignOut, RequestID: id}
	case "server":
		if len(rest) != 1 {
			return errors.New(plexUsage)
		}
		msg = shellipc.PlexChooseServer{Type: shellipc.TypePlexChooseServer, RequestID: id, ServerID: rest[0]}
	case "libraries":
		if len(rest) == 0 {
			return errors.New(plexUsage)
		}
		msg = shellipc.PlexChooseLibraries{Type: shellipc.TypePlexChooseLibraries, RequestID: id, LibraryIDs: rest}
	default:
		return errors.New(plexUsage)
	}
	if _, err := call(*sock, msg, id); err != nil {
		return err
	}
	return plexStatus(*sock)
}

func currentPlex(sock string) (contract.Plex, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dialCLI(ctx, sock)
	if err != nil {
		return contract.Plex{}, err
	}
	defer conn.Close()
	if conn.State.Plex == nil {
		if conn.State.Session.Locked {
			return contract.Plex{}, errors.New("the TV session is locked; Plex is hidden until it is unlocked")
		}
		return contract.Plex{}, errors.New("this coordinator has no Plex connector (bear-den-tv dev needs --dev-plex-fake)")
	}
	return *conn.State.Plex, nil
}

func plexStatus(sock string) error {
	p, err := currentPlex(sock)
	if err != nil {
		return err
	}
	fmt.Printf("status: %s\n", p.Status)
	if p.Code != nil && p.LinkURL != nil {
		fmt.Printf("code: %s (type it at %s)\n", *p.Code, *p.LinkURL)
	}
	if p.Server != nil {
		fmt.Printf("server: %s\n", *p.Server)
	}
	for _, s := range p.Servers {
		where := "remote"
		if s.Local {
			where = "home network"
		}
		fmt.Printf("server %s\t%s (%s)\n", s.ID, s.Name, where)
	}
	for _, l := range p.Libraries {
		mark := " "
		if l.Selected {
			mark = "x"
		}
		fmt.Printf("library [%s] %s\t%s (%s)\n", mark, l.ID, l.Title, l.Kind)
	}
	if p.Message != "" {
		fmt.Printf("message: %s\n", strings.TrimSpace(p.Message))
	}
	return nil
}
