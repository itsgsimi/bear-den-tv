// Command bear-den-tv is the Bear Den TV coordinator and its local admin CLI
// (docs/decisions/0002-one-cli-binary-embedded-remote.md).
//
//	bear-den-tv session              run the coordinator in the graphical session
//	bear-den-tv dev                  run with a fake desktop on loopback (development)
//	bear-den-tv doctor [--probe]     print diagnostics as JSON
//	bear-den-tv pair                 show a pairing invitation (code + URL)
//	bear-den-tv devices [revoke ID]  list or revoke paired phones
//	bear-den-tv remote enable|disable --interface IF --accept-lan-exposure
//	bear-den-tv artwork fetch        cache official app icons from Flathub
//	bear-den-tv autostart enable|disable|status   start Bear Den at every desktop login
//	bear-den-tv shortcut enable|disable|status    add Bear Den TV to the desktop and app menu
//	bear-den-tv apps detect|tune|probe            playback detection per app; best settings
//	bear-den-tv themes list|validate DIR|path     installed themes; check a theme; owner themes dir
//	bear-den-tv weather status|search Q|set Q|off local weather on the running coordinator
//	bear-den-tv plex status|sign-in|server ID|libraries ID...|cancel|sign-out  Plex sign-in on the running coordinator
//	bear-den-tv version              print the version
package main

import (
	"fmt"
	"os"
)

// Version is stamped at build time with -ldflags "-X main.Version=...".
var Version = "0.1.0-dev"

func usage() {
	fmt.Fprint(os.Stderr, `usage: bear-den-tv <command> [flags]

commands:
  session   run the coordinator (supervises the TV shell)
  dev       run with a fake desktop, loopback remote, and optional DEMO fixtures
  doctor    print diagnostics as JSON
  pair      issue a pairing invitation on the running coordinator
  devices   list paired phones, or "devices revoke <id|*>"
  remote    "remote enable --interface IF --accept-lan-exposure" or "remote disable"
  artwork   "artwork fetch": cache the official Flathub icons of the registered apps
  autostart "autostart enable|disable|status": start Bear Den at desktop login
  shortcut  "shortcut enable|disable|status": Bear Den TV icon on the desktop and in the app menu
  apps      "apps detect [--apply] [--json] [--tier T]" (alias "apps tune"): playback detection and best settings;
            "apps probe [--json]": only what each app decodes in hardware here
  themes    "themes list" | "themes validate <dir>" | "themes path": theme packages (docs/THEMES.md)
  weather   "weather status" | "weather search QUERY" | "weather off" |
            "weather set [--units celsius|fahrenheit] [--no-scene] QUERY [INDEX]": local weather (Open-Meteo)
  plex      "plex status" | "plex sign-in" | "plex server ID" | "plex libraries ID..." | "plex cancel" |
            "plex sign-out": sign the TV in to Plex for the Home rows (the code also shows on the TV)
  version   print the version
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "session":
		err = cmdSession(args, false)
	case "dev":
		err = cmdSession(args, true)
	case "doctor":
		err = cmdDoctor(args)
	case "pair":
		err = cmdPair(args)
	case "devices":
		err = cmdDevices(args)
	case "remote":
		err = cmdRemote(args)
	case "artwork":
		err = cmdArtwork(args)
	case "autostart":
		err = cmdAutostart(args)
	case "shortcut":
		err = cmdShortcut(args)
	case "apps":
		err = cmdApps(args)
	case "themes":
		err = cmdThemes(args)
	case "weather":
		err = cmdWeather(args)
	case "plex":
		err = cmdPlex(args)
	case "version", "--version":
		fmt.Println(Version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "bear-den-tv: unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bear-den-tv:", err)
		os.Exit(1)
	}
}
