// `bear-den-tv autostart`: write or remove the XDG autostart entry
// (docs/operations.md). The entry itself is internal/platform/autostart,
// shared with the TV's toggle (IPC autostart.configure).

package main

import (
	"errors"
	"fmt"
	"os"

	"bear-den-tv/internal/platform/autostart"
)

// cmdAutostart installs or removes the login autostart entry. Owner-run only:
// it is never enabled implicitly.
func cmdAutostart(args []string) error {
	path := autostart.File()
	switch {
	case len(args) == 1 && args[0] == "enable":
		script, err := autostart.StartScript()
		if err != nil {
			return err
		}
		if err := autostart.Enable(script, path); err != nil {
			return err
		}
		fmt.Printf("autostart enabled: %s → %s --watch\n", path, script)
	case len(args) == 1 && args[0] == "disable":
		if err := autostart.Disable(path); err != nil {
			return err
		}
		fmt.Println("autostart disabled")
	case len(args) == 0 || (len(args) == 1 && args[0] == "status"):
		if raw, err := os.ReadFile(path); err == nil {
			fmt.Printf("enabled (%s)\n%s", path, raw)
		} else {
			fmt.Println("disabled")
		}
	default:
		return errors.New("usage: bear-den-tv autostart enable|disable|status")
	}
	return nil
}
