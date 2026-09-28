// Command bdtv-probe prints the desktop/application probe report as JSON. Run
// it on the TV machine inside (or pointed at) the graphical session:
//
//	DISPLAY=:0 bdtv-probe
//
// It observes only; nothing is launched, activated, killed, or injected.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"bear-den-tv/internal/platform/probe"
)

func main() {
	timeout := flag.Duration("timeout", 30*time.Second, "overall probe timeout")
	compact := flag.Bool("compact", false, "print single-line JSON")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report := probe.Report(ctx)

	enc := json.NewEncoder(os.Stdout)
	if !*compact {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "bdtv-probe:", err)
		os.Exit(1)
	}
}
