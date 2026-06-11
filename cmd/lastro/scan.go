package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/detect/branchscan"
	"github.com/iurykrieger/lastro/internal/detect/coverage"
	"github.com/iurykrieger/lastro/internal/persisthelp"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// runScanBranches is the deterministic half of /detect-use-cases: walk the
// application source, extract every logic branch, and write the inventory
// the LLM condenses into journey use cases.
func runScanBranches(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan-branches", flag.ContinueOnError)
	fs.SetOutput(stderr)
	src := fs.String("src", ".", "Application source root to scan")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}

	inv, err := branchscan.Scan(*src)
	if err != nil {
		fmt.Fprintln(stderr, "scan-branches:", err)
		return 1
	}
	raw, err := inv.Marshal()
	if err != nil {
		fmt.Fprintln(stderr, "scan-branches: marshal:", err)
		return 1
	}
	target := filepath.Join(*harnessDir, "branch-inventory.yaml")
	if err := persisthelp.AtomicWrite(target, raw); err != nil {
		fmt.Fprintln(stderr, "scan-branches: write:", err)
		return 1
	}

	summary := struct {
		Inventory     string `json:"inventory"`
		Files         int    `json:"files"`
		TotalBranches int    `json:"total_branches"`
	}{
		Inventory:     target,
		Files:         len(inv.Files),
		TotalBranches: len(inv.Branches),
	}
	_ = json.NewEncoder(stdout).Encode(summary)
	return 0
}

// runCoverage scores the on-disk use cases against the branch inventory
// and writes coverage.yaml next to it. The metric is information, not a
// gate: exit 0 whenever the inputs are readable.
func runCoverage(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	fs.SetOutput(stderr)
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}

	invPath := filepath.Join(*harnessDir, "branch-inventory.yaml")
	raw, err := os.ReadFile(invPath)
	if err != nil {
		fmt.Fprintf(stderr, "coverage: read %s (run scan-branches first): %v\n", invPath, err)
		return 1
	}
	inv, err := branchscan.Load(raw)
	if err != nil {
		fmt.Fprintln(stderr, "coverage:", err)
		return 1
	}

	useCases, err := readUseCasesShallow(filepath.Join(*harnessDir, "use-cases"))
	if err != nil {
		fmt.Fprintln(stderr, "coverage:", err)
		return 1
	}

	report := coverage.Compute(inv, useCases)
	out, err := yaml.Marshal(report)
	if err != nil {
		fmt.Fprintln(stderr, "coverage: marshal:", err)
		return 1
	}
	target := filepath.Join(*harnessDir, "coverage.yaml")
	if err := persisthelp.AtomicWrite(target, out); err != nil {
		fmt.Fprintln(stderr, "coverage: write:", err)
		return 1
	}
	_ = json.NewEncoder(stdout).Encode(report)
	return 0
}

// readUseCasesShallow parses every use-case YAML in both layouts (flat and
// journey folders) without cross-reference validation — coverage only needs
// id/journey/variation/covers, and must not fail on fixture lookups.
func readUseCasesShallow(dir string) ([]*usecase.UseCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no use cases yet: report 0% coverage
		}
		return nil, fmt.Errorf("read use cases dir %s: %w", dir, err)
	}
	var out []*usecase.UseCase
	readFile := func(path string) error {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		uc, err := usecase.UnmarshalOnly(raw)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		out = append(out, uc)
		return nil
	}
	isYAML := func(name string) bool { return filepath.Ext(name) == ".yaml" || filepath.Ext(name) == ".yml" }
	for _, e := range entries {
		if e.IsDir() {
			sub, err := os.ReadDir(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			for _, se := range sub {
				if !se.IsDir() && isYAML(se.Name()) {
					if err := readFile(filepath.Join(dir, e.Name(), se.Name())); err != nil {
						return nil, err
					}
				}
			}
			continue
		}
		if isYAML(e.Name()) {
			if err := readFile(filepath.Join(dir, e.Name())); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
