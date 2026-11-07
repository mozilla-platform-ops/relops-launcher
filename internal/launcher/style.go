package launcher

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

const (
	styleHeading    = "1;32"
	styleCommand    = "1"
	styleMetadata   = "2"
	styleBeta       = "33"
	styleError      = "31"
	styleProvenance = "32"
)

type palette struct{ enabled bool }

func terminalPalette(out io.Writer) palette {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return palette{}
	}
	return palette{isTerminal(out)}
}

func isTerminal(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	var group int32
	return terminalGroup(file, syscall.TIOCGPGRP, &group) == nil && group > 0
}

func (p palette) text(style, text string) string {
	if !p.enabled || text == "" {
		return text
	}
	return "\x1b[" + style + "m" + text + "\x1b[0m"
}

func writeError(out io.Writer, err error) {
	fmt.Fprintf(out, "relops: %s %s\n", terminalPalette(out).text(styleError, "error:"), err)
}
