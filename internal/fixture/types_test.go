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

func TestFixtureZeroValueIsUsable(t *testing.T) {
	var fx Fixture
	if fx.ID != "" || fx.Role != "" || fx.Parsed != nil || fx.Binding != nil {
		t.Errorf("zero-value Fixture should have empty fields; got %+v", fx)
	}
}

func TestBindingHoldsChannelAndSelector(t *testing.T) {
	b := Binding{
		Channel:  ChannelHTTP,
		Selector: map[string]any{"method": "POST", "path": "/orders"},
	}
	if b.Channel != ChannelHTTP {
		t.Errorf("Binding.Channel: got %q, want %q", b.Channel, ChannelHTTP)
	}
	if b.Selector["method"] != "POST" {
		t.Errorf("Binding.Selector[method]: got %v, want POST", b.Selector["method"])
	}
}

func TestSourceRefHoldsPathSymbolReason(t *testing.T) {
	r := SourceRef{Path: "src/orders.ts", Symbol: "createOrder", Reason: "handler"}
	if r.Path != "src/orders.ts" || r.Symbol != "createOrder" || r.Reason != "handler" {
		t.Errorf("SourceRef field round-trip failed: %+v", r)
	}
}

// Compile-time interface assertion: tested by the file compiling.
var _ FixtureStore = (*compileTimeStubStore)(nil)

type compileTimeStubStore struct{}

func (*compileTimeStubStore) LookupFixture(string) (Fixture, bool)  { return Fixture{}, false }
func (*compileTimeStubStore) FixturesForUseCase(string) []Fixture   { return nil }
func (*compileTimeStubStore) All() []Fixture                        { return nil }
