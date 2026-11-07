package launcher

import (
	"fmt"
	"strings"
)

// Color launcher diagnostics, leaving tool streams and redirected logs alone.
func (a *app) provenancef(format string, args ...any) {
	a.styledLine(styleProvenance, format, args...)
}

func (a *app) statusf(format string, args ...any) {
	a.styledLine(styleMetadata, format, args...)
}

func (a *app) styledLine(style, format string, args ...any) {
	line := fmt.Sprintf("relops: "+format, args...)
	p := terminalPalette(a.errOut)
	if p.enabled {
		line = strings.ReplaceAll(line, "(beta)", "\x1b["+styleBeta+"m(beta)\x1b["+style+"m")
	}
	fmt.Fprintln(a.errOut, p.text(style, line))
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
