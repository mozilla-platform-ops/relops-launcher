package launcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

func (a *app) runWeb(ctx context.Context, o options) (int, error) {
	return openWeb(ctx, o.webURL, o.printURL, runtime.GOOS, os.Getenv, a.out, a.errOut, func(ctx context.Context, command, url string) error {
		return exec.CommandContext(ctx, command, url).Run()
	})
}

func openWeb(ctx context.Context, url string, printOnly bool, platform string, getenv func(string) string, out, errOut io.Writer, open func(context.Context, string, string) error) (int, error) {
	if _, err := fmt.Fprintln(out, url); err != nil {
		return 1, err
	}
	if printOnly {
		return 0, nil
	}
	command := ""
	switch platform {
	case "darwin":
		command = "open"
	case "linux":
		if getenv("DISPLAY") != "" || getenv("WAYLAND_DISPLAY") != "" {
			command = "xdg-open"
		}
	}
	if command == "" {
		fmt.Fprintln(errOut, "relops: no graphical browser session detected; open the URL manually")
		return 0, nil
	}
	if err := open(ctx, command, url); err != nil {
		if ctx.Err() != nil {
			return 1, ctx.Err()
		}
		fmt.Fprintf(errOut, "relops: could not open browser (%v); open the URL manually\n", err)
	}
	return 0, nil
}
