package branchscan

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// skipDirNames are directory names never worth scanning: dependencies,
// build output, and the harness's own state. Dot-directories are skipped
// by prefix.
var skipDirNames = map[string]bool{
	"vendor": true, "node_modules": true, "dist": true,
	"build": true, "target": true, "testdata": true,
}

// testFileRe matches test sources in heuristic languages
// (app.test.js, app.spec.ts, test_app.py, app_test.rb, ...).
var testFileRe = regexp.MustCompile(`(\.(test|spec)\.[a-z]+$)|(^test_)|(_test\.[a-z]+$)`)

// Scan walks root and returns the branch inventory for every supported
// source file beneath it. Walk order is lexical, ids are content-derived,
// and the result marshals byte-identically across runs.
func Scan(root string) (*Inventory, error) {
	inv := &Inventory{
		SchemaVersion: "1.0.0",
		SourceRoot:    filepath.ToSlash(filepath.Clean(root)),
		Files:         []FileEntry{},
		Branches:      []Branch{},
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (skipDirNames[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		base := filepath.Base(path)
		ext := filepath.Ext(path)

		switch {
		case ext == ".go":
			if strings.HasSuffix(base, "_test.go") {
				return nil
			}
			src, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			branches, ok := analyzeGo(rel, src)
			if !ok {
				return nil // unparseable user code: skip, never abort
			}
			inv.Files = append(inv.Files, FileEntry{Path: rel, Precision: PrecisionAST})
			inv.Branches = append(inv.Branches, branches...)
		case heuristicExts[ext]:
			if testFileRe.MatchString(base) {
				return nil
			}
			src, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			inv.Files = append(inv.Files, FileEntry{Path: rel, Precision: PrecisionHeuristic})
			inv.Branches = append(inv.Branches, analyzeHeuristic(rel, src)...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	assignIDs(inv.Branches)
	return inv, nil
}
