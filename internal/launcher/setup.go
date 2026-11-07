package launcher

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
)

// Spool setup diagnostics to disk rather than retaining potentially large
// installer logs in memory. Stream them in verbose mode, or replay on failure.
func (a *app) runSetup(ctx context.Context, o options, cmd *exec.Cmd, capture bool) ([]byte, int, error) {
	log, err := os.CreateTemp(a.cache, "setup-*.log")
	if err != nil {
		return nil, 1, err
	}
	defer os.Remove(log.Name())
	defer log.Close()
	var diagnostics io.Writer = log
	if o.verbose {
		diagnostics = a.errOut
	}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = diagnostics, diagnostics
	if capture {
		cmd.Stdout = &output
	}
	code, err := runTool(ctx, cmd)
	if (err != nil || code != 0) && capture {
		_, _ = a.errOut.Write(output.Bytes())
	}
	if (err != nil || code != 0) && !o.verbose {
		if _, seekErr := log.Seek(0, io.SeekStart); seekErr == nil {
			_, _ = io.Copy(a.errOut, log)
		}
	}
	return output.Bytes(), code, err
}
