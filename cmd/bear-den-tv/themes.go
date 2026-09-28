// `bear-den-tv themes`: list installed theme packages (docs/THEMES.md).

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	bdtv "bear-den-tv"
	"bear-den-tv/internal/themes"
)

// cmdThemes helps people designing themes (docs/THEMES.md):
//
//	themes list             installed themes (built-in and yours), and any skipped package with the reason
//	themes validate <dir>   check a theme folder you are designing (schema, files, ornaments)
//	themes path             where to put your own themes
func cmdThemes(args []string) error {
	switch {
	case len(args) == 1 && args[0] == "list":
		reg, problems := themes.LoadDefault()
		for _, t := range reg.List() {
			origin := "yours"
			if g := reg.Get(t.ID); g != nil && g.BuiltIn {
				origin = "built-in"
			}
			fmt.Printf("%-12s %-12s %s  (%s)\n", t.ID, t.Name, t.Accent, origin)
		}
		for _, p := range problems {
			fmt.Printf("skipped      %s\n", p.String())
		}
		fmt.Printf("\nyour themes: %s\n", themes.UserDir())
		return nil
	case len(args) == 1 && args[0] == "path":
		fmt.Println(themes.UserDir())
		return nil
	case len(args) == 2 && args[0] == "validate":
		dir, err := filepath.Abs(args[1])
		if err != nil {
			return err
		}
		orn, _ := fs.Sub(bdtv.Ornaments, "apps/tv-shell/assets/ornaments")
		t, errs := themes.LoadPackage(os.DirFS(dir), filepath.Base(dir), orn)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Println("  -", e)
			}
			return fmt.Errorf("%s is not a valid theme (%d problem(s))", dir, len(errs))
		}
		fmt.Printf("%s: valid theme %q (%s)\n", dir, t.ID, t.Name)
		fmt.Println("Preview it: BDTV_THEMES_DIR=" + filepath.Dir(dir) + " and pick it in Settings → Theme, or see docs/THEMES.md → Preview.")
		return nil
	}
	return errors.New("usage: bear-den-tv themes list | themes validate <dir> | themes path")
}
