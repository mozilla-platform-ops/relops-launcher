package launcher

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWebTargets(t *testing.T) {
	for _, args := range [][]string{{"run", "pc"}, {"run", "--print-url", "hangar"}} {
		o, done, err := parse(args, &bytes.Buffer{}, "test")
		if err != nil || done || !strings.HasPrefix(o.webURL, "https://") {
			t.Fatalf("%v: %+v %v %v", args, o, done, err)
		}
	}
	for _, args := range [][]string{{"run", "--ref", "x", "pc"}, {"run", "--cwd", "caller", "pc"}, {"run", "pc", "--help"}, {"run", "--offline", "pc"}, {"run", "--print-url", "ct"}, {"repo", "--print-url", "example"}} {
		if _, _, err := parse(args, &bytes.Buffer{}, "test"); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestOpenWeb(t *testing.T) {
	for _, tc := range []struct {
		name, platform, display, wantCommand string
		printOnly, fail                      bool
		wantWarning                          bool
	}{
		{"mac", "darwin", "", "open", false, false, false},
		{"linux", "linux", ":0", "xdg-open", false, false, false},
		{"headless", "linux", "", "", false, false, true},
		{"print", "darwin", "", "", true, false, false},
		{"failure", "darwin", "", "open", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			called := ""
			code, err := openWeb(context.Background(), "https://example.com/", tc.printOnly, tc.platform, func(k string) string {
				if k == "DISPLAY" {
					return tc.display
				}
				return ""
			}, &out, &stderr, func(_ context.Context, cmd, url string) error {
				called = cmd
				if url != "https://example.com/" {
					t.Errorf("URL = %q", url)
				}
				if tc.fail {
					return errors.New("failed")
				}
				return nil
			})
			if err != nil || code != 0 || out.String() != "https://example.com/\n" || called != tc.wantCommand || (stderr.Len() > 0) != tc.wantWarning {
				t.Fatalf("code=%d err=%v out=%q stderr=%q command=%q", code, err, out.String(), stderr.String(), called)
			}
		})
	}
}
