// `bear-den-tv open-url URL` and `bear-den-tv links enable|disable|status`
// (docs/operations.md "Sign-ins on the TV"; internal/platform/links). While
// Bear Den handles the desktop's http and https links, every link an app
// opens comes to open-url: the running coordinator shows it on the TV when
// Bear Den or one of its apps is in front (IPC link.open), and anything else
// goes on to the browser that was the default before.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"bear-den-tv/internal/platform/links"
	"bear-den-tv/internal/shellipc"
)

// cmdOpenURL is the desktop's handler for a web link.
func cmdOpenURL(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: bear-den-tv open-url URL")
	}
	link := args[0]
	if handled, _ := openOnTV(socketPath(""), link); handled {
		return nil
	}
	return links.Forward(links.DefaultPaths(), link, links.Start)
}

// openOnTV asks the running coordinator to show link on the TV. Any failure
// (no coordinator, no answer within 3 s) is "not handled": the link then
// opens in the desktop's own browser, as if Bear Den were not there.
func openOnTV(socket, link string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := dialCLI(ctx, socket)
	if err != nil {
		return false, ""
	}
	defer conn.Close()
	const id = "cli-open-url"
	if err := conn.Send(shellipc.LinkOpen{Type: shellipc.TypeLinkOpen, RequestID: id, URL: link}); err != nil {
		return false, ""
	}
	res, err := conn.Result(id, 3*time.Second)
	if err != nil || !res.OK {
		return false, ""
	}
	data, _ := res.Data.(map[string]any)
	reason, _ := data["reason"].(string)
	return data["handled"] == true, reason
}

// cmdLinks turns Bear Den's link handling on or off, or says where it is.
func cmdLinks(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: bear-den-tv links enable|disable|status")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := links.DefaultPaths()
	switch args[0] {
	case "enable":
		changed, err := links.Enable(ctx, p, executable(), links.ExecRunner)
		if err != nil {
			return err
		}
		if changed {
			fmt.Println("Bear Den now handles web links: an app's sign-in shows on the TV; other links open in your usual browser.")
		} else {
			fmt.Println("Bear Den already handles web links.")
		}
	case "disable":
		if err := links.Disable(ctx, p, links.ExecRunner); err != nil {
			return err
		}
		st, _ := links.Load(p)
		fmt.Printf("Web links open in %s again; Bear Den will not take them over until `bear-den-tv links enable`.\n", orNone(st.Previous))
	case "status":
		on, st, err := links.Status(ctx, p, links.ExecRunner)
		if err != nil {
			return err
		}
		switch {
		case on:
			fmt.Printf("on: links Bear Den does not show on the TV open in %s\n", orNone(st.Previous))
		case st.Off:
			fmt.Println("off (turned off with `bear-den-tv links disable`)")
		default:
			fmt.Println("off: the running session turns it on at its next start")
		}
	default:
		return errors.New("usage: bear-den-tv links enable|disable|status")
	}
	return nil
}

func orNone(id string) string {
	if id == "" {
		return "the desktop's default browser"
	}
	return id
}

// executable is this binary's path with symlinks resolved ("" if unknown).
func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r
	}
	return exe
}

// ensureLinks makes Bear Den the desktop's link handler at session start,
// unless the owner turned it off; a failure is logged, never fatal.
func ensureLinks(ctx context.Context, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	changed, err := links.Ensure(ctx, links.DefaultPaths(), executable(), links.ExecRunner)
	switch {
	case err != nil:
		log.Warn("links: could not make Bear Den the handler for web links", "err", err)
	case changed:
		st, _ := links.Load(links.DefaultPaths())
		log.Info("links: Bear Den now handles web links; others go on to the previous browser", "previous", st.Previous)
	}
}
