package fixture

import "testing"

func TestRoleValues(t *testing.T) {
	cases := map[Role]string{
		RoleInput:              "input",
		RoleExpectedOutput:     "expected-output",
		RoleExpectedSideEffect: "expected-side-effect",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("Role %q: got %q, want %q", want, string(got), want)
		}
	}
}

func TestChannelValues(t *testing.T) {
	cases := map[Channel]string{
		ChannelHTTP:    "http",
		ChannelCLIArgs: "cli-args",
		ChannelEvent:   "event",
		ChannelStdout:  "stdout",
		ChannelLogLine: "log-line",
		ChannelDBRow:   "db-row",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("Channel %q: got %q, want %q", want, string(got), want)
		}
	}
}
