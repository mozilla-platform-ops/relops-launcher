package launcher

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const wordmark = `           __
  _______ / /__  ___  ___
 / __/ -_) / _ \/ _ \(_-<
/_/  \__/_/\___/ .__/___/
              /_/`

// Query the output terminal; COLUMNS is a fallback for terminals without winsize.
func terminalWidth(out io.Writer) int {
	if f, ok := out.(*os.File); ok {
		var size struct{ rows, cols, x, y uint16 }
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
		if errno == 0 && size.cols > 0 {
			return int(size.cols)
		}
	}
	n, _ := strconv.Atoi(os.Getenv("COLUMNS"))
	return n
}

type helpRow struct{ name, description string }

func writeHelp(out io.Writer, mode, version string) error {
	p := terminalPalette(out)
	limit := terminalWidth(out)
	if limit <= 0 {
		limit = 80
	}
	var b strings.Builder
	heading := func(s string) { fmt.Fprintln(&b, p.text(styleHeading, s)) }
	paragraph := func(s string) { fmt.Fprintln(&b, s); fmt.Fprintln(&b) }
	rows := func(title string, entries ...helpRow) {
		if title != "" {
			heading(title)
		}
		width := 0
		for _, r := range entries {
			if len(r.name) > width {
				width = len(r.name)
			}
		}
		for _, r := range entries {
			if limit < 80 || width+4 >= limit/2 {
				fmt.Fprintf(&b, "  %s\n", p.text(styleCommand, r.name))
				writeWrapped(&b, r.description, "    ", limit)
			} else {
				indent := strings.Repeat(" ", width+4)
				var description strings.Builder
				writeWrapped(&description, r.description, indent, limit)
				fmt.Fprintf(&b, "  %s%s  %s", p.text(styleCommand, r.name), strings.Repeat(" ", width-len(r.name)), strings.TrimPrefix(description.String(), indent))
			}
		}
		fmt.Fprintln(&b)
	}
	common := []helpRow{{"--offline", "Use cached code or the last resolved release; no network."}, {"-v, --verbose", "Show full provenance and live setup logs on stderr."}, {"--version", "Print launcher version."}, {"-h, --help", "Show help."}}
	source := []helpRow{{"--ref REF", "Pushed branch, tag, or full commit SHA; default: remote branch."}, {"--runner RUNNER", "auto, python3, bash, or sh; default: auto."}, {"--command NAME", "Select a uv project's console entry point."}, {"--cwd DIR", "caller or repo; default: caller for scripts, project for projects."}}
	switch mode {
	case "root":
		width := terminalWidth(out)
		if isTerminal(out) && os.Getenv("TERM") != "dumb" && (width == 0 || width >= 40) {
			paragraph(p.text(styleHeading, wordmark))
		} else {
			heading("relops")
		}
		paragraph("Run your team's tools.  " + p.text(styleMetadata, "Version "+version))
		rows("QUICK START", helpRow{"relops catalog list", "Find a tool."}, helpRow{"relops catalog info fr", "See requirements and details."}, helpRow{"relops c fr --help", "Show Fleetroll's help."})
		rows("COMMANDS", helpRow{"c [options] NAME [args...]", "Run a catalog tool (alias: catalog launch)."}, helpRow{"catalog list", "List available tools (alias: catalog ls)."}, helpRow{"catalog info NAME", "Inspect a tool."}, helpRow{"repo [options] REPO[/PATH] [args...]", "Run from a repository."})
		rows("COMMON OPTIONS", common...)
		heading("ARGUMENTS")
		paragraph("  Launcher options go before the target; all later arguments go to the tool.\n  relops c --offline fr --help\n           launcher     tool")
		heading("MORE HELP")
		paragraph("  relops c --help       Catalog selection and source/release options.\n  relops repo --help    Repository execution and environment rules.\n  relops catalog --help Catalog discovery.\n\n  Direct execution also works: relops [options] REPO[/PATH] [args...]\n  Bare fleetbench runs its latest stable release; use repo for its source.")
	case "catalog-index":
		heading("CATALOG")
		paragraph("Find a tool, check its requirements, and run it by name or alias.")
		rows("QUICK START", helpRow{"relops catalog ls", "Find a tool and its aliases."}, helpRow{"relops catalog info fr", "See Fleetroll's requirements and targets."}, helpRow{"relops c fr --help", "Show Fleetroll's help."})
		heading("RUN A TOOL")
		paragraph("  relops c is shorthand for relops catalog launch. These are equivalent:")
		rows("", helpRow{"relops c fr --help", "Use the short form."}, helpRow{"relops catalog launch fr --help", "Use the full form."})
		paragraph("  NAME accepts a tool's catalog name or alias: fr = fleetroll, fb = fleetbench.\n  Add -b before NAME for beta tools: relops c -b replay-tc-pool --help")
		rows("COMMANDS", helpRow{"relops catalog list", "List tools and aliases (alias: ls)."}, helpRow{"relops catalog info NAME", "Inspect requirements and targets."}, helpRow{"relops c [options] NAME [args...]", "Run a tool from the catalog."})
		heading("ARGUMENTS")
		paragraph("  Launcher options go before NAME; all later arguments go to the tool.\n  relops c --offline fr --help\n           launcher     tool")
		rows("MORE HELP", helpRow{"relops c --help", "Catalog selection and source/release options."}, helpRow{"relops --help", "All launcher commands."})
		paragraph("Listing and inspecting the bundled catalog need no network access.\nLaunching a tool may fetch source or download a release unless --offline.")
	case "catalog-list", "catalog-ls":
		heading("USAGE")
		paragraph("  relops catalog list\n  relops catalog ls\n\nList bundled tools, aliases, and channels without network access.\nRun a tool with relops c NAME [args...]; beta-only tools require -b.")
	case "catalog-info":
		heading("USAGE")
		paragraph("  relops catalog info NAME\n\nInspect a tool by name or alias, including requirements and launch commands.\nExample: relops catalog info fr")
	case "catalog", "repo":
		heading("USAGE")
		if mode == "catalog" {
			paragraph("  relops c [options] NAME [tool-arguments...]\n  relops catalog launch [options] NAME [tool-arguments...]")
			rows("CATALOG SELECTION", helpRow{"-b, --beta", "Select the configured beta target; cannot combine with --ref."})
		} else {
			paragraph("  relops repo [options] REPO[/PATH] [tool-arguments...]\n\nUse repo to select source execution, including repositories named c, catalog,\nrepo, or fleetbench. Git with GitHub SSH access is required.")
			rows("REPOSITORY", helpRow{"--owner OWNER", "GitHub user or organization; default: mozilla-platform-ops."})
		}
		rows("COMMON OPTIONS", common...)
		rows("SOURCE EXECUTION", source...)
		if mode == "catalog" {
			rows("RELEASE UPDATES", helpRow{"--refresh", "Check latest release now; respects rate-limit cooldowns."})
			paragraph("Release targets reject --ref, --runner, --command, and --cwd repo.\n--refresh requires an online release target. Catalog launches reject --owner.\nMissing channels fail; beta selection never falls back to default.")
			rows("EXAMPLES", helpRow{"relops c fr --help", "Show tool help."}, helpRow{"relops c -b replay-tc-pool -h", "Select a beta tool."}, helpRow{"relops c --ref my-feature fr --help", "Try a pushed source branch."}, helpRow{"relops c fleetbench cpu --mode quick --json", "Run a verified release binary."})
		} else {
			rows("EXAMPLES", helpRow{"relops repo --ref my-feature relops-infra/path/to/check.sh --verbose", "Pass --verbose to the tool."}, helpRow{"relops repo --runner python3 example/check.py ./hosts.csv", "Use caller-relative input."})
			heading("ENVIRONMENTS")
			paragraph("Auto detects shebangs and Python scripts. Pipfile projects use installed\nPipenv; pyproject.toml + uv.lock projects use installed uv. Locked runtime\ndependencies are synced automatically. An explicit --runner bypasses setup.\nProject targets select the matching or sole console entry point; use\n--command when selection is ambiguous.")
		}
		heading("EXECUTION")
		paragraph("Launcher options must precede the target. All following arguments go to\nthe tool, including --help. Source runs fetch before execution unless --offline.\nEach source checkout is removed on exit: save outputs outside it. Release\nbinaries run in the caller's directory. Tools run with your normal permissions.")
	}
	_, err := io.WriteString(out, b.String())
	return err
}
