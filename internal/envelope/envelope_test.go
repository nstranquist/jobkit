package envelope

import (
	"errors"
	"testing"
)

func TestExitCodeMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{New(CodeInvalidArgs, "bad"), 2},
		{New(CodeNotFound, "missing"), 3},
		{New(CodeIOFailed, "io"), 1},
		{New(CodeInternal, "boom"), 1},
		{errors.New("plain"), 1},
	}
	for _, tc := range cases {
		if got := ExitCode(tc.err); got != tc.want {
			t.Fatalf("ExitCode(%v)=%d want %d", tc.err, got, tc.want)
		}
	}
}

func TestWithHint(t *testing.T) {
	e := New(CodeInvalidArgs, "nope").WithHint("try help")
	if e.Hint != "try help" {
		t.Fatalf("hint = %q", e.Hint)
	}
	if e.Error() != "nope" {
		t.Fatalf("Error() = %q", e.Error())
	}
}

func TestNewf(t *testing.T) {
	e := Newf(CodeNotFound, "missing %s", "profile")
	if e.Message != "missing profile" {
		t.Fatalf("message = %q", e.Message)
	}
}
