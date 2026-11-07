package launcher

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

// Color launcher diagnostics, leaving tool streams and redirected logs alone.
func (a *app) provenancef(format string, args ...any) {
	a.styledLine("36", format, args...)
}

func (a *app) statusf(format string, args ...any) {
	a.styledLine("2", format, args...)
}

func (a *app) styledLine(style, format string, args ...any) {
	line := fmt.Sprintf("relops: "+format, args...)
	if file, ok := a.errOut.(*os.File); ok && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" {
		var group int32
		if terminalGroup(file, syscall.TIOCGPGRP, &group) == nil {
			fmt.Fprintf(a.errOut, "\x1b[%sm%s\x1b[0m\n", style, line)
			return
		}
	}
	fmt.Fprintln(a.errOut, line)
}

func launchName(o options) string {
	name := o.catalogName
	if name == "" {
		name = o.owner + "/" + o.repo
		if o.path != "" {
			name += "/" + o.path
		}
	}
	if o.catalogChannel == "beta" {
		name += " (beta)"
	}
	return name
}

func (a *app) sourceHeader(o options, c checkout) {
	ref := strings.TrimPrefix(c.ref, "refs/heads/")
	if strings.HasPrefix(ref, "refs/tags/") {
		ref = "tag:" + strings.TrimPrefix(ref, "refs/tags/")
	}
	sha := c.sha
	if len(sha) > 8 {
		sha = sha[:8]
	}
	revision := ref + "@" + sha
	if commitID.MatchString(ref) {
		revision = sha
	}
	suffix := ""
	if o.offline {
		suffix = " (cached)"
	}
	a.provenancef("%s · %s%s", launchName(o), revision, suffix)
	if o.verbose {
		a.provenancef("%s/%s ref=%s commit=%s (%s)", o.owner, o.repo, c.ref, c.sha, resolutionMode(o.offline))
	}
}
