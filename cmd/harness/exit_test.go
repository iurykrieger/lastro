package main

import (
	"context"
	"errors"
	"testing"
)

func TestExitCodeFor(t *testing.T) {
	type tc struct {
		name string
		err  error
		ctx  func() context.Context
		want int
	}
	cases := []tc{
		{
			name: "nil error -> 0",
			err:  nil,
			ctx:  context.Background,
			want: ExitOK,
		},
		{
			name: "VerdictFailError -> 1",
			err:  VerdictFailError,
			ctx:  context.Background,
			want: ExitFail,
		},
		{
			name: "VerdictInconclusiveError -> 2",
			err:  VerdictInconclusiveError,
			ctx:  context.Background,
			want: ExitInconclusive,
		},
		{
			name: "*UsageError -> 64",
			err:  &UsageError{Msg: "supply --use-case or --all"},
			ctx:  context.Background,
			want: ExitUsage,
		},
		{
			name: "ErrUnimplemented -> 70",
			err:  ErrUnimplemented,
			ctx:  context.Background,
			want: ExitSoftware,
		},
		{
			name: "context.Canceled with no signal -> 130",
			err:  context.Canceled,
			ctx:  context.Background,
			want: ExitInterrupt,
		},
		{
			name: "context.Canceled with SIGTERM -> 143",
			err:  context.Canceled,
			ctx: func() context.Context {
				return SetCancelSignal(context.Background(), "SIGTERM")
			},
			want: ExitSignalTerm,
		},
		{
			name: "wrapped VerdictFailError -> 1",
			err:  errwrap("validate failed: %w", VerdictFailError),
			ctx:  context.Background,
			want: ExitFail,
		},
		{
			name: "unknown error -> 70",
			err:  errors.New("kaboom"),
			ctx:  context.Background,
			want: ExitSoftware,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := exitCodeFor(c.err, c.ctx())
			if got != c.want {
				t.Fatalf("exitCodeFor = %d, want %d", got, c.want)
			}
		})
	}
}

// errwrap is a small helper to construct wrapped errors inline in the
// test table without pulling fmt.Errorf calls into each case.
func errwrap(format string, errs ...error) error {
	args := make([]any, len(errs))
	for i, e := range errs {
		args[i] = e
	}
	return wrappedErr{msg: format, inner: errs[0]}
}

type wrappedErr struct {
	msg   string
	inner error
}

func (w wrappedErr) Error() string { return w.msg }
func (w wrappedErr) Unwrap() error { return w.inner }
