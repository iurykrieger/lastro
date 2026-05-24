package fixturebinder

import "testing"

func TestNormalizeEnvName(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"login", "HARNESS_FIXTURE_LOGIN"},
		{"login-basic", "HARNESS_FIXTURE_LOGIN_BASIC"},
		{"a", "HARNESS_FIXTURE_A"},
		{"x1-y2-z3", "HARNESS_FIXTURE_X1_Y2_Z3"},
		{"abc-def-ghi", "HARNESS_FIXTURE_ABC_DEF_GHI"},
	}
	for _, c := range cases {
		got := normalizeEnvName(c.id)
		if got != c.want {
			t.Errorf("normalizeEnvName(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}
