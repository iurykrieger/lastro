package sensor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// templateRef matches ${{ ... }} interpolation expressions, which are not
// valid shell syntax. They are replaced with a literal placeholder before
// parsing so a fixture-bound step still gets its commands checked.
var templateRef = regexp.MustCompile(`\$\{\{[^}]*\}\}`)

const templateRefPlaceholder = "HARNESS_TEMPLATE_REF"

// shellBuiltins are command heads provided by the shell itself; they never
// resolve through PATH and are always considered resolvable.
var shellBuiltins = map[string]bool{
	":": true, ".": true, "[": true, "break": true, "cd": true,
	"command": true, "continue": true, "eval": true, "exec": true,
	"exit": true, "export": true, "local": true, "read": true,
	"return": true, "set": true, "shift": true, "source": true,
	"trap": true, "type": true, "ulimit": true, "umask": true,
	"unset": true, "wait": true,
}

// ResolvabilityEnv injects the environment probes ValidateStepResolvability
// uses, so tests stay deterministic. Zero values fall back to the real
// environment (exec.LookPath, current directory).
type ResolvabilityEnv struct {
	LookPath func(string) (string, error)
	RepoRoot string
}

// ValidateStepResolvability asserts that every command a run-step invokes
// resolves to a real executable on this machine (grounding invariant 3:
// sensors may only run commands that exist). It parses each step's Run
// script as POSIX shell, checks the head word of every command call
// against PATH, and verifies plain `make <target>` calls against the
// repo's Makefile. Errors are reported per step, naming the step id and
// the unresolvable command or target.
func ValidateStepResolvability(s Sensor, env ResolvabilityEnv) error {
	lookPath := env.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	root := env.RepoRoot
	if root == "" {
		root = "."
	}
	var errs []error
	for _, st := range s.Steps {
		if st.Run == "" {
			continue
		}
		calls, err := commandCalls(st.Run)
		if err != nil {
			// Unparseable scripts are not this invariant's concern; the
			// schema/template validators own syntax. Best effort: skip.
			continue
		}
		for _, call := range calls {
			head := call[0]
			if shellBuiltins[head] {
				continue
			}
			if _, err := lookPath(head); err != nil {
				errs = append(errs, fmt.Errorf(
					"step %q: command %q not found on this machine; use a stack-native tool or a self-bootstrapping form (e.g. go run <module>@latest, npx --yes <pkg>)",
					st.ID, head))
				continue
			}
			if head == "make" {
				if err := checkMakeTarget(call[1:], root); err != nil {
					errs = append(errs, fmt.Errorf("step %q: %w", st.ID, err))
				}
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// commandCalls parses a run script and returns, per command call, the
// literal words of that call ("" marks a word that is not statically
// known: expansions, quotes). Calls whose head is not a plain literal,
// is the template placeholder, or names a function declared inside the
// script itself are excluded.
func commandCalls(run string) ([][]string, error) {
	parser := syntax.NewParser()
	script := templateRef.ReplaceAllString(run, templateRefPlaceholder)
	file, err := parser.Parse(strings.NewReader(script), "run")
	if err != nil {
		return nil, err
	}
	declared := map[string]bool{}
	var calls [][]string
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.FuncDecl:
			declared[n.Name.Value] = true
		case *syntax.CallExpr:
			if len(n.Args) == 0 {
				return true
			}
			words := make([]string, len(n.Args))
			for i, w := range n.Args {
				words[i] = w.Lit()
			}
			if words[0] != "" {
				calls = append(calls, words)
			}
		}
		return true
	})
	var external [][]string
	for _, c := range calls {
		if !declared[c[0]] && c[0] != templateRefPlaceholder {
			external = append(external, c)
		}
	}
	return external, nil
}

// checkMakeTarget verifies that the target a plain `make <target>` call
// names is declared in the repo-root Makefile. Calls that redirect make
// elsewhere (-C/-f and long forms), pass variables only, name no target,
// or use a non-literal target are not statically decidable and are
// skipped.
func checkMakeTarget(args []string, repoRoot string) error {
	target := ""
	for _, a := range args {
		switch {
		case a == "-C" || a == "-f" || strings.HasPrefix(a, "--directory") || strings.HasPrefix(a, "--file") || strings.HasPrefix(a, "--makefile"):
			return nil // make reads another Makefile; not decidable here
		case a == "" || a == templateRefPlaceholder:
			return nil // non-literal target; not decidable
		case strings.HasPrefix(a, "-") || strings.Contains(a, "="):
			continue // flag or VAR=value
		default:
			target = a
		}
		if target != "" {
			break
		}
	}
	if target == "" {
		return nil // default target
	}
	targets, ok := makefileTargets(repoRoot)
	if !ok {
		return fmt.Errorf("make target %q: no Makefile found at repo root %q", target, repoRoot)
	}
	if !targets[target] {
		return fmt.Errorf("make target %q not found in the repo Makefile; use an existing target", target)
	}
	return nil
}

// makefileTargets returns the set of target names declared in the first
// Makefile found at root (GNU make's lookup order), and whether one exists.
func makefileTargets(root string) (map[string]bool, bool) {
	for _, name := range []string{"GNUmakefile", "makefile", "Makefile"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		targets := map[string]bool{}
		for _, line := range strings.Split(string(content), "\n") {
			if line == "" || line[0] == '\t' || line[0] == '#' {
				continue
			}
			idx := strings.IndexByte(line, ':')
			if idx <= 0 {
				continue
			}
			if rest := line[idx+1:]; strings.HasPrefix(rest, "=") {
				continue // := assignment, not a rule
			}
			for _, name := range strings.Fields(line[:idx]) {
				targets[name] = true
			}
		}
		return targets, true
	}
	return nil, false
}
