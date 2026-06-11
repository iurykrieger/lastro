package branchscan

import (
	"regexp"
	"strings"

	"github.com/iurykrieger/lastro/internal/enums"
)

// heuristicExts are the extensions the line-regex analyzer accepts. Counts
// from these files are approximate (precision: heuristic) — good enough to
// drive coverage, not an exact parse.
var heuristicExts = map[string]bool{
	".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".mjs": true, ".cjs": true,
	".py": true, ".rb": true, ".php": true, ".java": true, ".cs": true,
}

var (
	elseIfRe  = regexp.MustCompile(`^\}?\s*(else\s+if|elif|elsif)\b`)
	elseRe    = regexp.MustCompile(`^\}?\s*else\b`)
	ifRe      = regexp.MustCompile(`^if\s*[\s(]`)
	caseRe    = regexp.MustCompile(`^(case|when)\s+(.+?):?\s*$`)
	defaultRe = regexp.MustCompile(`^default\s*:`)
	catchRe   = regexp.MustCompile(`^\}?\s*(catch\b|except\b|rescue\b)`)
	ternaryRe = regexp.MustCompile(`=\s*([^?;=]+?)\s*\?[^:]+:`)
)

// analyzeHeuristic extracts branches from one source line at a time. Each
// line yields at most one branch; comment lines are skipped.
func analyzeHeuristic(relPath string, src []byte) []Branch {
	var out []Branch
	emit := func(line int, kind enums.BranchKind, condition string) {
		out = append(out, Branch{File: relPath, Line: line, Kind: kind, Condition: condition})
	}

	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimSpace(raw)
		lineNo := i + 1
		if line == "" || isCommentLine(line) {
			continue
		}
		switch {
		case elseIfRe.MatchString(line):
			emit(lineNo, enums.BranchElseIf, parenCondition(line))
		case elseRe.MatchString(line):
			emit(lineNo, enums.BranchElse, "")
		case ifRe.MatchString(line):
			emit(lineNo, enums.BranchIf, parenCondition(line))
		case defaultRe.MatchString(line):
			emit(lineNo, enums.BranchDefault, "")
		case caseRe.MatchString(line):
			m := caseRe.FindStringSubmatch(line)
			emit(lineNo, enums.BranchCase, strings.TrimSpace(m[2]))
		case catchRe.MatchString(line):
			emit(lineNo, enums.BranchCatch, strings.TrimRight(strings.TrimSpace(strings.TrimPrefix(line, "}")), " {"))
		case ternaryRe.MatchString(line):
			m := ternaryRe.FindStringSubmatch(line)
			emit(lineNo, enums.BranchTernary, strings.TrimSpace(m[1]))
		}
	}
	return out
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "*") || strings.HasPrefix(line, "/*")
}

// parenCondition extracts the condition from an if/else-if line: the text
// inside the outermost parentheses when present, otherwise the line with
// the keyword and trailing block/colon punctuation stripped.
func parenCondition(line string) string {
	if i := strings.Index(line, "("); i >= 0 {
		if j := strings.LastIndex(line, ")"); j > i {
			return strings.TrimSpace(line[i+1 : j])
		}
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "}"))
	for _, kw := range []string{"else if", "elif", "elsif", "if"} {
		if cut, ok := strings.CutPrefix(rest, kw); ok {
			rest = cut
			break
		}
	}
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(rest), "{:"))
}
