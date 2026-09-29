// `bear-den-tv badges status|on|off|reset`: Den badges on the running
// coordinator over the shell socket (achievements.configure and
// achievements.reset in contracts/ipc.md; internal/achievements). The CLI
// never edits config.json or the state database itself.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"bear-den-tv/internal/shellipc"
)

const badgesUsage = `usage: bear-den-tv badges status
       bear-den-tv badges on|off
       bear-den-tv badges reset`

func cmdBadges(args []string) error {
	if len(args) == 0 {
		return errors.New(badgesUsage)
	}
	fs := flag.NewFlagSet("badges "+args[0], flag.ExitOnError)
	sock := socketFlag(fs)
	_ = fs.Parse(args[1:])
	id := "cli-badges-" + fmt.Sprint(time.Now().UnixNano())
	switch args[0] {
	case "status":
		return badgesStatus(*sock)
	case "on", "off":
		on := args[0] == "on"
		if _, err := call(*sock, shellipc.AchievementsConfigure{Type: shellipc.TypeAchievementsConfigure, RequestID: id, Enabled: on}, id); err != nil {
			return err
		}
		if on {
			fmt.Println("Badges on.")
		} else {
			fmt.Println("Badges off: nothing is counted. Earned badges stay until reset.")
		}
		return nil
	case "reset":
		if _, err := call(*sock, shellipc.AchievementsReset{Type: shellipc.TypeAchievementsReset, RequestID: id}, id); err != nil {
			return err
		}
		fmt.Println("Badges reset: every counter and earned badge was deleted.")
		return nil
	}
	return errors.New(badgesUsage)
}

func badgesStatus(sock string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dialCLI(ctx, sock)
	if err != nil {
		return err
	}
	defer conn.Close()
	a := conn.State.Achievements
	if a == nil {
		if conn.State.Session.Locked {
			return errors.New("the TV session is locked; badges are hidden until it is unlocked")
		}
		return errors.New("this coordinator does not report badges")
	}
	fmt.Printf("counting: %v\n", map[bool]string{true: "on", false: "off"}[a.Enabled])
	earned := map[string]string{}
	for _, e := range a.Earned {
		earned[e.ID] = e.Day
	}
	fmt.Printf("earned: %d of %d\n", len(a.Earned), len(a.Progress))
	for _, p := range a.Progress {
		if d, ok := earned[p.ID]; ok {
			fmt.Printf("  %-16s earned %s\n", p.ID, d)
		} else {
			fmt.Printf("  %-16s %d/%d\n", p.ID, p.Count, p.Goal)
		}
	}
	return nil
}
