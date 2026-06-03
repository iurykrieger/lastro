package main

import (
	"fmt"
	"io"
	"os"

	"github.com/iurykrieger/lastro/internal/enums"
)

// renderValidateText writes a human-readable summary to w.
// Signature matches Render()'s renderText parameter.
func renderValidateText(w io.Writer, r *RunResult) error {
	g := pickGlyphs()
	result, ok := r.Result.(map[string]any)
	if !ok {
		fmt.Fprintln(w, "(empty result)")
		return nil
	}
	verdicts, _ := result["verdicts"].([]any)
	for _, vAny := range verdicts {
		v, ok := vAny.(map[string]any)
		if !ok {
			continue
		}
		verdict, _ := v["verdict"].(enums.Verdict)
		ucID, _ := v["use_case_id"].(string)
		conf, _ := v["confidence"].(float64)
		sensors, _ := v["sensors"].([]any)
		fmt.Fprintf(w, "%s %-30s (%s, confidence %.2f, %d sensors)\n",
			g.For(verdict), ucID, verdict, conf, len(sensors))
		for _, sAny := range sensors {
			s, ok := sAny.(map[string]any)
			if !ok {
				continue
			}
			sv, _ := s["verdict"].(enums.Verdict)
			sa, _ := s["angle"].(enums.ValidationAngle)
			sc, _ := s["confidence"].(float64)
			fmt.Fprintf(w, "  %s %-14s %s   confidence %.2f\n",
				g.For(sv), sa, sv, sc)
		}
	}
	return nil
}

type glyphs struct {
	Pass         string
	Fail         string
	Warn         string
	Inconclusive string
}

func (g glyphs) For(v enums.Verdict) string {
	switch v {
	case enums.VerdictPass:
		return g.Pass
	case enums.VerdictFail:
		return g.Fail
	case enums.VerdictWarn:
		return g.Warn
	default:
		return g.Inconclusive
	}
}

// pickGlyphs returns ASCII fallbacks when NO_COLOR is set; otherwise
// returns Unicode glyphs.
func pickGlyphs() glyphs {
	if os.Getenv("NO_COLOR") != "" {
		return glyphs{Pass: "[OK]", Fail: "[FAIL]", Warn: "[WARN]", Inconclusive: "[??]"}
	}
	return glyphs{Pass: "✓", Fail: "✗", Warn: "⚠", Inconclusive: "?"}
}
