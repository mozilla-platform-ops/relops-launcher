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
		versionLabel := p.text(styleMetadata, "v"+version)
		if isTerminal(out) && os.Getenv("TERM") != "dumb" && (width == 0 || width >= 40) {
			lines := strings.Split(wordmark, "\n")
			logoWidth := 0
			for _, line := range lines {
				if len(line) > logoWidth {
					logoWidth = len(line)
				}
			}
			for i, line := range lines {
				fmt.Fprint(&b, p.text(styleHeading, line))
				// The final row is the p's descender; align with the lettering above it.
				if i == len(lines)-2 && (width == 0 || logoWidth+4+len(version) <= width) {
					fmt.Fprint(&b, strings.Repeat(" ", logoWidth-len(line)+3), versionLabel)
				}
				fmt.Fprintln(&b)
			}
			if width > 0 && logoWidth+4+len(version) > width {
				fmt.Fprintln(&b, versionLabel)
			}
		} else {
			fmt.Fprintln(&b, p.text(styleHeading, "relops")+"   "+versionLabel)
		}
		fmt.Fprintln(&b)
		paragraph("RelOps tools. One command.")
		rows("QUICK START", helpRow{"relops run", "Browse tools and launch commands."}, helpRow{"relops catalog", "Read tool descriptions (alias: c)."}, helpRow{"relops run ct --help", "Show task creator options."})
		rows("COMMANDS", helpRow{"run", "Browse tools and launch commands (alias: r)."}, helpRow{"run [options] NAME [args...]", "Run a catalog tool (alias: r)."}, helpRow{"catalog", "Read tool descriptions (alias: c)."}, helpRow{"info NAME", "Inspect requirements and targets (alias: i)."}, helpRow{"repo [options] REPO[/PATH] [args...]", "Run from a repository."})
		rows("COMMON OPTIONS", common...)
		heading("ARGUMENTS")
		paragraph("  Launcher options go before the target; all later arguments go to the tool.\n  relops run --offline fr --help\n             launcher     tool")
		heading("MORE HELP")
		paragraph("  relops run --help     Catalog selection and source/release options.\n  relops repo --help    Repository execution and environment rules.\n  relops catalog --help Catalog discovery.")
	case "catalog-index":
		heading("CATALOG")
		paragraph("Find a tool, check its requirements, and run it by name or alias.")
		rows("QUICK START", helpRow{"relops run", "Browse tools and launch commands."}, helpRow{"relops catalog", "Read tool descriptions (alias: c)."}, helpRow{"relops run ct --help", "Show task creator options."})
		heading("RUN A TOOL")
		paragraph("  Run relops run without a name to browse available tools.\n  Use run to launch a tool, or r for the same action with fewer keystrokes:")
		rows("", helpRow{"relops run ct --help", "Show task creator options."}, helpRow{"relops r ct --help", "Use the shortcut."})
		paragraph("  NAME accepts a tool's catalog name or alias: fr = fleetroll, fb = fleetbench.\n  Add -b before NAME for beta tools: relops run -b replay-tc-pool --help")
		rows("COMMANDS", helpRow{"relops run", "Browse tools and launch commands."}, helpRow{"relops catalog", "Read tool descriptions (alias: c)."}, helpRow{"relops info NAME", "Inspect requirements and targets (alias: i)."}, helpRow{"relops run [options] NAME [args...]", "Run a tool from the catalog."})
		heading("ARGUMENTS")
		paragraph("  Launcher options go before NAME; all later arguments go to the tool.\n  relops run --offline fr --help\n             launcher     tool")
		paragraph("Catalog shortcut: relops c.\nCompatibility aliases: relops catalog launch, relops catalog list,\nrelops catalog ls, and relops catalog info NAME.")
		rows("MORE HELP", helpRow{"relops run --help", "Catalog selection and source/release options."}, helpRow{"relops --help", "All launcher commands."})
		paragraph("Listing and inspecting the bundled catalog need no network access.\nLaunching a tool may fetch source or download a release unless --offline.")
	case "catalog-list", "catalog-ls":
		heading("USAGE")
		paragraph("  relops catalog\n  relops c\n  relops catalog list\n  relops catalog ls\n\nList bundled tools, aliases, and channels without network access.\nRun a tool with relops run NAME [args...]; beta-only tools require -b.")
	case "catalog-info":
		heading("USAGE")
		paragraph("  relops info NAME\n  relops i NAME\n  relops catalog info NAME (compatibility alias)\n\nInspect a tool by name or alias, including requirements and launch commands.\nExample: relops info fr")
	case "catalog", "repo":
		if mode == "repo" {
			heading("REPOSITORY TOOLS")
			paragraph("Run a script or Python project directly from a GitHub repository.")
			rows("QUICK START",
				helpRow{"relops repo android-tools/worker_health/create_tc_task.py --help", "Show a repository script's help."},
				helpRow{"relops repo fleetroll_mvp --help", "Run a Python project's console command."},
				helpRow{"relops repo --ref my-feature REPO/PATH --help", "Try a pushed branch, tag, or commit."})
		}
		heading("USAGE")
		if mode == "catalog" {
			paragraph("  relops run [options] NAME [tool-arguments...]\n  relops r [options] NAME [tool-arguments...]\n\nCompatibility alias: relops catalog launch.\nRun without a name to browse available tools.")
			rows("CATALOG SELECTION", helpRow{"-b, --beta", "Select the configured beta target; cannot combine with --ref."})
		} else {
			paragraph("  relops repo [options] REPO[/PATH] [tool-arguments...]\n\nUse repo to select source execution, including repositories named run, r, c,\ncatalog, info, i, repo, or fleetbench. Git with GitHub SSH access is required.")
			rows("REPOSITORY", helpRow{"--owner OWNER", "GitHub user or organization; default: mozilla-platform-ops."})
		}
		rows("COMMON OPTIONS", common...)
		rows("SOURCE EXECUTION", source...)
		if mode == "catalog" {
			rows("WEB TOOLS", helpRow{"--print-url", "Print a web tool's URL without opening a browser."})
			rows("RELEASE UPDATES", helpRow{"--refresh", "Check latest release now; respects rate-limit cooldowns."})
			paragraph("Release targets reject --ref, --runner, --command, and --cwd repo.\n--refresh requires an online release target. Catalog launches reject --owner.\nMissing channels fail; beta selection never falls back to default.")
			rows("EXAMPLES", helpRow{"relops run fr --help", "Show tool help."}, helpRow{"relops run -b replay-tc-pool -h", "Select a beta tool."}, helpRow{"relops run --ref my-feature fr --help", "Try a pushed source branch."}, helpRow{"relops run fleetbench cpu --mode quick --json", "Run a verified release binary."})
		} else {
			heading("ARGUMENTS")
			paragraph("  Launcher options go before REPO[/PATH]; later arguments go to the tool.\n  relops repo --offline fleetroll_mvp --help\n              launcher               tool\n\n  Scripts use your current directory by default; project targets use their\n  project directory. Use --cwd caller or --cwd repo to override this.")
		}
		if mode == "repo" {
			heading("NOTE")
			paragraph("  Temporary checkouts are removed on exit. Save outputs outside them.")
			rows("MORE HELP", helpRow{"relops catalog", "Find tools you can run by name or alias."}, helpRow{"relops --help", "All launcher commands."})
		} else {
			heading("EXECUTION")
			paragraph("Launcher options must precede the target. All following arguments go to\nthe tool, including --help. Source runs fetch before execution unless --offline.\nEach source checkout is removed on exit: save outputs outside it.\nTools run with your normal permissions.")
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}
