package branchscan

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"

	"github.com/iurykrieger/lastro/internal/enums"
)

// analyzeGo extracts branches from one Go source file with go/ast. The
// second return is false for unparseable files (best-effort engine: a
// syntax error in user code must not abort the scan).
func analyzeGo(relPath string, src []byte) ([]Branch, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, false
	}

	var branches []Branch
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		branches = append(branches, goFuncBranches(fset, fn, relPath)...)
	}
	return branches, true
}

func goFuncBranches(fset *token.FileSet, fn *ast.FuncDecl, relPath string) []Branch {
	// Pre-pass: mark if-statements that hang off another if's else, so the
	// main walk can emit them as else-if instead of if.
	elseIf := map[*ast.IfStmt]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if s, ok := n.(*ast.IfStmt); ok {
			if chained, ok := s.Else.(*ast.IfStmt); ok {
				elseIf[chained] = true
			}
		}
		return true
	})

	var out []Branch
	emit := func(line int, kind enums.BranchKind, condition string) {
		out = append(out, Branch{
			File:      relPath,
			Line:      line,
			Kind:      kind,
			Condition: condition,
			Enclosing: fn.Name.Name,
		})
	}

	// CaseClause covers switch and type-switch; CommClause covers select.
	// Traversal is never pruned, so branches nested inside clause bodies
	// are still found.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.IfStmt:
			kind := enums.BranchIf
			if elseIf[s] {
				kind = enums.BranchElseIf
			}
			emit(fset.Position(s.If).Line, kind, renderNode(fset, s.Cond))
			if block, ok := s.Else.(*ast.BlockStmt); ok {
				emit(fset.Position(block.Lbrace).Line, enums.BranchElse, "")
			}
		case *ast.CaseClause:
			if s.List == nil {
				emit(fset.Position(s.Case).Line, enums.BranchDefault, "")
				return true
			}
			parts := make([]string, len(s.List))
			for i, e := range s.List {
				parts[i] = renderNode(fset, e)
			}
			emit(fset.Position(s.Case).Line, enums.BranchCase, strings.Join(parts, ", "))
		case *ast.CommClause:
			if s.Comm == nil {
				emit(fset.Position(s.Case).Line, enums.BranchDefault, "")
				return true
			}
			emit(fset.Position(s.Case).Line, enums.BranchCase, renderNode(fset, s.Comm))
		}
		return true
	})
	return out
}

func renderNode(fset *token.FileSet, n ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, n); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}
