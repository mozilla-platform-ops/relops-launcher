package launcher

import (
	"fmt"
	"io"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

// Show a compact chooser without fetching or launching any tool.
func catalogShort(out io.Writer) error {
	c, err := catalog.Bundled()
	if err != nil {
		return err
	}
	p := terminalPalette(out)
	var b strings.Builder
	nameWidth := 0
	for _, t := range c.Tools {
		if len(t.ID) > nameWidth {
			nameWidth = len(t.ID)
		}
	}
	for _, beta := range []bool{false, true} {
		headed := false
		for _, t := range c.Tools {
			if (!beta && t.Default == nil) || (beta && t.Beta == nil) {
				continue
			}
			if !headed {
				title := "TOOLS"
				if beta {
					title = "TOOLS (BETA)"
				}
				fmt.Fprintln(&b, p.text(styleHeading, title))
				headed = true
			}
			name := t.ID
			if len(t.Aliases) > 0 {
				name = t.Aliases[0]
			}
			launch := "relops c "
			if beta {
				launch += "-b "
			}
			fmt.Fprintf(&b, "  %s%s  %s%s\n", p.text(styleCommand, t.ID), strings.Repeat(" ", nameWidth-len(t.ID)), launch, name)
		}
		if headed {
			fmt.Fprintln(&b)
		}
	}
	_, err = io.WriteString(out, b.String())
	return err
}

func catalogDisplay(args []string, out io.Writer) error {
	c, err := catalog.Bundled()
	if err != nil {
		return err
	}
	p := terminalPalette(out)
	var b strings.Builder
	switch args[0] {
	case "list", "ls":
		if len(args) != 1 {
			return fmt.Errorf("usage: relops catalog")
		}
		for _, beta := range []bool{false, true} {
			headed := false
			for _, t := range c.Tools {
				if (!beta && t.Default == nil) || (beta && t.Beta == nil) {
					continue
				}
				if !headed {
					if beta {
						fmt.Fprintln(&b, p.text(styleHeading, "TOOLS (BETA)"))
					} else {
						fmt.Fprintln(&b, p.text(styleHeading, "TOOLS"))
					}
					fmt.Fprintln(&b)
					headed = true
				}
				names := p.text(styleCommand, t.ID)
				if len(t.Aliases) > 0 {
					label := "alias: "
					if len(t.Aliases) > 1 {
						label = "aliases: "
					}
					names += " " + p.text(styleMetadata, "· "+label+strings.Join(t.Aliases, ", "))
				}
				fmt.Fprintf(&b, "  %s\n", names)
				writeWrapped(&b, t.Description, "    ", terminalWidth(out))
				name := t.ID
				if len(t.Aliases) > 0 {
					name = t.Aliases[0]
				}
				launch := "relops c "
				if beta {
					launch += "-b "
				}
				fmt.Fprintf(&b, "    %s%s [args...]\n", launch, name)
				fmt.Fprintln(&b)
			}
		}
		fmt.Fprintln(&b, "Inspect a tool: relops info NAME")
		fmt.Fprintln(&b, p.text(styleMetadata, "\nBundled catalog "+c.Revision))
	case "info":
		if len(args) != 2 {
			return fmt.Errorf("usage: relops info NAME")
		}
		t, err := c.Lookup(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s — %s\n", p.text(styleHeading, t.ID), t.Name)
		writeWrapped(&b, t.Description, "", terminalWidth(out))
		if len(t.Aliases) > 0 {
			fmt.Fprintln(&b, "Aliases: "+strings.Join(t.Aliases, ", "))
		}
		for _, beta := range []bool{false, true} {
			target, channel, err := t.Select(beta)
			if err != nil {
				continue
			}
			style := styleHeading
			if beta {
				style = styleBeta
			}
			fmt.Fprintf(&b, "\n%s\n", p.text(style, strings.ToUpper(channel)))
			field := func(label, value string) {
				if value != "" {
					writeWrapped(&b, label+": "+value, "  ", terminalWidth(out))
				}
			}
			field("Source", target.Owner+"/"+target.Repo)
			if target.Kind == "release" {
				field("Version", "latest stable release (verified checksum)")
				field("Directory", "caller")
			} else {
				ref := target.Ref
				if ref == "" {
					ref = "remote default branch"
				}
				field("Revision", ref)
				field("Path", target.Path)
				command := target.Command
				if command == "" && target.Path == "" {
					command = "automatic console entry-point selection"
				}
				field("Command", command)
				runner := target.Runner
				if runner == "" {
					runner = "auto"
				}
				field("Runner", runner)
				cwd := target.Cwd
				if cwd == "" {
					cwd = "automatic: caller for scripts, project directory for projects"
				}
				field("Directory", cwd)
			}
			field("Prerequisites", target.Prerequisites)
			field("Docs", target.Documentation)
			field("Note", target.Note)
			launch := "relops c "
			if beta {
				launch += "-b "
			}
			launch += t.ID
			fmt.Fprintf(&b, "  Launch: %s [args...]\n", p.text(styleCommand, launch))
		}
		fmt.Fprintln(&b, p.text(styleMetadata, "\nBundled catalog "+c.Revision))
	default:
		return fmt.Errorf("unknown catalog command %q; use list (alias: ls), info, or launch", args[0])
	}
	_, err = io.WriteString(out, b.String())
	return err
}

// Keep URLs and identifiers intact; wrap prose at word boundaries.
func writeWrapped(out io.Writer, text, indent string, width int) {
	if width <= 0 {
		width = 80
	}
	line := indent
	for _, word := range strings.Fields(text) {
		if line != indent && len(line)+1+len(word) > width {
			fmt.Fprintln(out, line)
			line = indent
		}
		if line != indent {
			line += " "
		}
		line += word
	}
	fmt.Fprintln(out, line)
}
