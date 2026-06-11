package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAppFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const appGo = `package app

func Handle(ok bool) string {
	if ok {
		return "yes"
	} else {
		return "no"
	}
}
`

func TestScanBranchesThenCoverage_EndToEnd(t *testing.T) {
	repo := t.TempDir()
	harness := filepath.Join(repo, ".harness")
	writeAppFile(t, repo, "app.go", appGo)

	var stdout, stderr bytes.Buffer
	code := run([]string{"lastro", "scan-branches", "--src", repo, "--harness-dir", harness}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("scan-branches exit %d, stderr: %s", code, stderr.String())
	}
	var summary struct {
		Files         int `json:"files"`
		TotalBranches int `json:"total_branches"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("summary JSON: %v (%s)", err, stdout.String())
	}
	if summary.Files != 1 || summary.TotalBranches != 2 {
		t.Fatalf("summary = %+v, want 1 file / 2 branches (if + else)", summary)
	}

	inventory, err := os.ReadFile(filepath.Join(harness, "branch-inventory.yaml"))
	if err != nil {
		t.Fatalf("inventory not written: %v", err)
	}
	idLine := ""
	for _, line := range strings.Split(string(inventory), "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		if strings.HasPrefix(trimmed, "id: br-") {
			idLine = strings.TrimPrefix(trimmed, "id: ")
			break
		}
	}
	if idLine == "" {
		t.Fatalf("no branch id found in inventory:\n%s", inventory)
	}

	// One journeyed use case covering the first branch → 50% coverage.
	ucYAML := `schema_version: 2.0.0
id: uc-handle-yes
title: Handle returns yes
archetype_scope: [library]
entry_points:
  - id: handle
    archetype: library
    spec: {symbol: Handle}
given: ["g"]
when: ["w"]
then: ["t"]
journey: handling
variation: success
covers: [` + idLine + `]
`
	writeAppFile(t, harness, filepath.Join("use-cases", "handling", "uc-handle-yes.yaml"), ucYAML)

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"lastro", "coverage", "--harness-dir", harness}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("coverage exit %d, stderr: %s", code, stderr.String())
	}
	var report struct {
		TotalBranches   int     `json:"total_branches"`
		CoveredBranches int     `json:"covered_branches"`
		CoveragePercent float64 `json:"coverage_percent"`
		Journeys        []struct {
			Journey      string `json:"journey"`
			SuccessCount int    `json:"success_count"`
		} `json:"journeys"`
		Uncovered []struct {
			ID string `json:"id"`
		} `json:"uncovered"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("report JSON: %v (%s)", err, stdout.String())
	}
	if report.TotalBranches != 2 || report.CoveredBranches != 1 || report.CoveragePercent != 50.0 {
		t.Fatalf("report = %+v, want 2 total / 1 covered / 50.0%%", report)
	}
	if len(report.Journeys) != 1 || report.Journeys[0].Journey != "handling" || report.Journeys[0].SuccessCount != 1 {
		t.Fatalf("journeys = %+v", report.Journeys)
	}
	if len(report.Uncovered) != 1 {
		t.Fatalf("uncovered = %+v, want exactly the else branch", report.Uncovered)
	}
	if _, err := os.Stat(filepath.Join(harness, "coverage.yaml")); err != nil {
		t.Fatalf("coverage.yaml not written: %v", err)
	}
}

func TestCoverage_MissingInventoryFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lastro", "coverage", "--harness-dir", t.TempDir()}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "scan-branches") {
		t.Errorf("stderr should point at scan-branches, got: %s", stderr.String())
	}
}
