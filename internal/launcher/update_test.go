package launcher

import (
	"bytes"
	"testing"
)

func TestUpdateArguments(t *testing.T) {
	for _, tc := range []struct {
		args             []string
		bad, check, done bool
	}{{[]string{"update"}, false, false, false}, {[]string{"update", "--check"}, false, true, false}, {[]string{"update", "--help"}, false, false, true}, {[]string{"update", "--offline"}, true, false, false}, {[]string{"--owner", "other", "update"}, true, false, false}, {[]string{"update", "--check", "extra"}, true, false, false}} {
		var out bytes.Buffer
		o, done, e := parse(tc.args, &out, "1.0.0")
		if (e != nil) != tc.bad || done != tc.done || o.updateCheck != tc.check {
			t.Fatalf("%v: %+v %v %v", tc.args, o, done, e)
		}
	}
}
