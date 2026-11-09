package sitebuild

import (
	"strings"
	"testing"
)

func TestShellSyntaxAndDirectories(t *testing.T) {
	tests := []struct {
		name, source string
		temporary    bool
		want         []string
	}{
		{"quoted arguments", `printf '%s' 'not > output'; echo '$(touch fake)' # > ignored
cp 'source file' 'destination file'
(echo x > 'a path with spaces')`, true, []string{"'destination file'", "'a path with spaces'"}},
		{"quoted heredoc", "cat <<'END' > real\necho x > fake\n$(touch also-fake)\nEND\n", true, []string{"real"}},
		{"expanded heredoc", "cat <<END\n$(touch actual)\nEND\n", true, []string{"actual"}},
		{"multiline", "cp \\\n input \\\n 'multiline output'\n", true, []string{"'multiline output'"}},
		{"bare redirect", "> result", true, []string{"result"}},
		{"fd duplication", "echo x 2>&1 > result\ncat < input", true, []string{"result"}},
		{"pipeline isolation", "cd /tmp | cat\necho x > result", true, []string{"result"}},
		{"background isolation", "cd /tmp &\necho x > result", true, []string{"result"}},
		{"backticks", "DIR=`dirname \"$0\"`\ncd \"$DIR\"\necho x > result", false, []string{"result"}},
		{"directory in and-or", `cd "$(dirname "$0")" && echo x > nested`, false, []string{"nested"}},
		{"unknown reassign", `DIR=$(dirname "$0")
DIR=$UNRESOLVED
echo x > "$DIR/unknown"`, false, nil},
		{"leave checkout", "cd /tmp\necho x > safe\nmkdir safe-dir", true, nil},
		{"enter script directory", `DIR=$(dirname -- "${BASH_SOURCE[0]}")
cd -- "$DIR" && echo x > result`, false, []string{"result"}},
		{"pwd alias", `cd "$(dirname "$0")"
OUT="$PWD/results"
cd /tmp
echo x > "$OUT/file"`, false, []string{`"$OUT/file"`}},
		{"reassignment", `DIR=$(dirname "$0")
DIR=/tmp
echo x > "$DIR/safe"`, false, nil},
		{"subshell isolation", "(cd /tmp; echo x > safe)\necho x > result", true, []string{"result"}},
		{"subshell inside", `(cd "$(dirname "$0")"; echo x > nested)
echo x > caller-file`, false, []string{"nested"}},
		{"substitution isolation", `DIR=$(cd /tmp; pwd)
echo x > "$DIR/safe"
echo x > result`, true, []string{"result"}},
		{"substitution writes", `DIR=$(touch side-effect; cd /tmp; pwd)
echo x > "$DIR/safe"`, true, []string{"side-effect"}},
		{"block persists", "{ cd /tmp; }\necho x > safe", true, nil},
		{"previous directory", "cd /tmp\ncd -\necho x > result", true, []string{"result"}},
		{"branch merge", "if check; then cd /tmp; fi\necho x > possible", true, []string{"possible"}},
		{"outside branches", "if check; then cd /tmp; else cd /var/tmp; fi\necho x > safe", true, nil},
		{"unknown cd", "cd \"$UNKNOWN\"\necho x > unknown", true, nil},
		{"relative directory", "cd subdir\necho x > result", true, []string{"result"}},
		{"escape checkout", "cd ..\necho x > safe", true, nil},
		{"redirect before cd", "cd /tmp > log\necho x > safe", true, []string{"log"}},
		{"function scope", "f() { cd /tmp; echo x > safe; }\necho x > result", true, []string{"result"}},
		{"git root", `cd "$(dirname "$0")"
ROOT=$(git rev-parse --show-toplevel)
cd /tmp
echo x > "$ROOT/result"`, false, []string{`"$ROOT/result"`}},
		{"trim script filename", `DIR=${0%/*}
cd "$DIR"
echo x > result`, false, []string{"result"}},
		{"no execution", `touch /tmp/relops-inspector-must-not-execute
exit 99`, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inspectShell(pythonInput{Path: "scripts/tool.sh", Source: tt.source, TemporaryCwd: tt.temporary})
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v; want paths %q", got, tt.want)
			}
			for i, f := range got {
				if f.Rule == "shell-parse" || !strings.HasSuffix(f.Detail, ": "+tt.want[i]) {
					t.Errorf("finding %+v; want path %q", f, tt.want[i])
				}
			}
		})
	}
}

func TestShellParseError(t *testing.T) {
	got := inspectShell(pythonInput{Path: "broken.sh", Source: "echo ok\nif then\n"})
	if len(got) != 1 || got[0].Rule != "shell-parse" || got[0].Line != 2 {
		t.Fatalf("unexpected findings: %+v", got)
	}
}
