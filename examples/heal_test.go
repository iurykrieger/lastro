//go:build integration

package examples_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/examples/validator"
)

// TestCriterion6_HealOnBroken — plan §11.6.
// Copy the broken sample into a temp dir, validate (expect 1 failure with
// non-nil HealHint), apply the committed EditPlan via /heal, re-validate
// (expect AllPassed). Single iteration.
func TestCriterion6_HealOnBroken(t *testing.T) {
	if testing.Short() {
		t.Skip("heal test is slow")
	}

	src, err := filepath.Abs("./http-api-sample-broken")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := validator.CopyDir(src, tmp); err != nil {
		t.Fatalf("copy: %v", err)
	}
	// Strip any inherited runtime/reports state.
	_ = os.RemoveAll(filepath.Join(tmp, ".harness", "runtime"))
	_ = os.RemoveAll(filepath.Join(tmp, ".harness", "reports"))

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	// 1. First validation: expect 1 failing UC with a non-nil heal hint.
	report1, err := validator.ValidateAll(ctx, tmp, skills)
	if err != nil {
		t.Fatalf("validate (pre-heal): %v", err)
	}
	failed := report1.Failed()
	if len(failed) != 1 || failed[0].UseCaseID != "uc-create-order-bad-input" {
		t.Fatalf("pre-heal: want 1 fail in uc-create-order-bad-input, got %+v", failed)
	}
	if failed[0].HealHint == nil {
		t.Fatalf("pre-heal: HealHint must be non-nil")
	}

	// 2. Apply the committed EditPlan via /heal.
	editPlan, err := os.ReadFile(filepath.Join(tmp, "heal-fixture", "editplan.json"))
	if err != nil {
		t.Fatalf("read editplan: %v", err)
	}
	cmd := exec.CommandContext(ctx, skills.Heal, "uc-create-order-bad-input")
	cmd.Dir = tmp
	cmd.Stdin = bytes.NewReader(editPlan)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("/heal failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	// 3. Re-validate; expect all-pass.
	report2, err := validator.ValidateAll(ctx, tmp, skills)
	if err != nil {
		t.Fatalf("validate (post-heal): %v", err)
	}
	if !report2.AllPassed() {
		t.Fatalf("post-heal: not all passed — summary=%+v failed=%+v", report2.Summary, report2.Failed())
	}
}
