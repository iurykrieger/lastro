package branchscan

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func kindsOf(branches []Branch) []enums.BranchKind {
	out := make([]enums.BranchKind, len(branches))
	for i, b := range branches {
		out[i] = b.Kind
	}
	return out
}

func findBranch(t *testing.T, branches []Branch, kind enums.BranchKind, condition string) Branch {
	t.Helper()
	for _, b := range branches {
		if b.Kind == kind && b.Condition == condition {
			return b
		}
	}
	t.Fatalf("no branch kind=%q condition=%q in %+v", kind, condition, branches)
	return Branch{}
}

func TestGoAnalyzer_ExtractsAllBranchKinds(t *testing.T) {
	inv, err := Scan("testdata/goapp")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	want := map[enums.BranchKind]int{
		enums.BranchIf:      2, // n < 0, err != nil
		enums.BranchElseIf:  1, // n == 0
		enums.BranchElse:    1,
		enums.BranchCase:    2, // "GET" and "POST","PUT"
		enums.BranchDefault: 1,
	}
	got := map[enums.BranchKind]int{}
	for _, b := range inv.Branches {
		got[b.Kind]++
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("kind %q count = %d, want %d (all: %v)", k, got[k], n, kindsOf(inv.Branches))
		}
	}

	b := findBranch(t, inv.Branches, enums.BranchIf, "n < 0")
	if b.Enclosing != "classify" {
		t.Errorf("enclosing = %q, want classify", b.Enclosing)
	}
	if b.File != "main.go" {
		t.Errorf("file = %q, want main.go", b.File)
	}
	if b.Line != 6 {
		t.Errorf("line = %d, want 6", b.Line)
	}

	multi := findBranch(t, inv.Branches, enums.BranchCase, `"POST", "PUT"`)
	if multi.Enclosing != "route" {
		t.Errorf("case enclosing = %q, want route", multi.Enclosing)
	}
}

func TestGoAnalyzer_SkipsTestFiles(t *testing.T) {
	inv, err := Scan("testdata/goapp")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, f := range inv.Files {
		if f.Path == "main_test.go" {
			t.Errorf("test file scanned: %v", f)
		}
	}
	for _, b := range inv.Branches {
		if b.File == "main_test.go" {
			t.Errorf("branch from test file: %+v", b)
		}
	}
}

func TestHeuristicAnalyzer_ExtractsJSBranches(t *testing.T) {
	inv, err := Scan("testdata/jsapp")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	want := map[enums.BranchKind]int{
		enums.BranchIf:      1,
		enums.BranchElseIf:  1,
		enums.BranchElse:    1,
		enums.BranchCase:    1,
		enums.BranchDefault: 1,
		enums.BranchCatch:   1,
		enums.BranchTernary: 1,
	}
	got := map[enums.BranchKind]int{}
	for _, b := range inv.Branches {
		got[b.Kind]++
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("kind %q count = %d, want %d (all: %v)", k, got[k], n, kindsOf(inv.Branches))
		}
	}

	if len(inv.Files) != 1 || inv.Files[0].Path != "app.js" || inv.Files[0].Precision != PrecisionHeuristic {
		t.Errorf("files = %+v, want app.js heuristic only (node_modules skipped)", inv.Files)
	}
}

func TestScan_DeterministicOutput(t *testing.T) {
	first, err := Scan("testdata")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	second, err := Scan("testdata")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	a, err := first.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	b, err := second.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Error("two scans of the same tree are not byte-identical")
	}

	idPattern := regexp.MustCompile(`^br-[a-f0-9]{12}$`)
	seen := map[string]bool{}
	for _, br := range first.Branches {
		if !idPattern.MatchString(br.ID) {
			t.Errorf("id %q does not match br-<12 hex>", br.ID)
		}
		if seen[br.ID] {
			t.Errorf("duplicate branch id %q", br.ID)
		}
		seen[br.ID] = true
	}
}

func TestLoad_RoundTrips(t *testing.T) {
	inv, err := Scan("testdata/goapp")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	raw, err := inv.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := Load(raw)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(back.Branches) != len(inv.Branches) {
		t.Errorf("round-trip branch count = %d, want %d", len(back.Branches), len(inv.Branches))
	}
	if back.SchemaVersion != "1.0.0" {
		t.Errorf("schema_version = %q, want 1.0.0", back.SchemaVersion)
	}
}
