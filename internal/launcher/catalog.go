package launcher

import (
	"fmt"
	"io"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

func catalogDisplay(args []string, out io.Writer) error {
	c, err := catalog.Bundled()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: relops catalog list")
		}
		if _, err := fmt.Fprintf(out, "Bundled catalog %s\n", c.Revision); err != nil {
			return err
		}
		for _, t := range c.Tools {
			names := t.ID
			if len(t.Aliases) > 0 {
				names += " (" + strings.Join(t.Aliases, ", ") + ")"
			}
			channels := []string{}
			if t.Default != nil {
				channels = append(channels, "default")
			}
			if t.Beta != nil {
				channels = append(channels, "beta")
			}
			if _, err := fmt.Fprintf(out, "%s [%s]\n  %s\n", names, strings.Join(channels, ", "), t.Description); err != nil {
				return err
			}
		}
		return nil
	case "info":
		if len(args) != 2 {
			return fmt.Errorf("usage: relops catalog info NAME")
		}
		t, err := c.Lookup(args[1])
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "%s — %s\n%s\nAliases: %s\nBundled catalog: %s\n", t.ID, t.Name, t.Description, strings.Join(t.Aliases, ", "), c.Revision); err != nil {
			return err
		}
		for _, beta := range []bool{false, true} {
			target, channel, err := t.Select(beta)
			if err != nil {
				continue
			}
			ref := target.Ref
			if ref == "" {
				ref = "remote default branch"
			}
			if target.Kind == "release" {
				ref = "latest stable release"
			}
			if _, err := fmt.Fprintf(out, "\n%s: %s %s/%s path=%q ref=%s\n", channel, target.Kind, target.Owner, target.Repo, target.Path, ref); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "  command=%q runner=%q cwd=%q\n  Prerequisites: %s\n  Docs: %s\n  %s\n", target.Command, target.Runner, target.Cwd, target.Prerequisites, target.Documentation, target.Note); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown catalog command %q; use list, info, or launch", args[0])
	}
}
