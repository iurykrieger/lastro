package executor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iurykrieger/lastro/internal/aggregate"
)

const crashHintTailBytes = 4096

// synthesizeCrashHint builds a HealHint from the trailing crashHintTailBytes
// of raw.log. Used when a step exits non-zero with zero Signals — the
// per-sensor rollup would otherwise have no hint to attach.
//
// rawPath is the absolute path to <runDir>/raw.log.
func synthesizeCrashHint(rawPath string, cause error) *aggregate.HealHint {
	tail := readTail(rawPath, crashHintTailBytes)
	stderr := filterStream(tail, "stderr")
	rationale := fmt.Sprintf("stderr tail:\n%s", strings.TrimRight(stderr, "\n"))
	if cause != nil && cause.Error() != "" {
		rationale = fmt.Sprintf("%s\n\nunderlying cause: %s", rationale, cause.Error())
	}
	step := "?"
	// Use type-specific accessors when available.
	switch e := cause.(type) {
	case *SpawnError:
		step = fmt.Sprintf("%d", e.Step)
	case *TemplateError:
		step = fmt.Sprintf("%d", e.Step)
	}
	return &aggregate.HealHint{
		Summary:   fmt.Sprintf("sensor crashed at step %s (no signals emitted)", step),
		Rationale: rationale,
	}
}

// readTail returns the last n bytes of path; an empty string on error.
func readTail(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return ""
	}
	size := stat.Size()
	if size > int64(n) {
		if _, err := f.Seek(size-int64(n), io.SeekStart); err != nil {
			return ""
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(b)
}

// filterStream returns only lines from raw whose annotated stream tag
// matches `stream` (e.g. "stderr").
func filterStream(raw, stream string) string {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	var b strings.Builder
	tag := " " + stream + "] "
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "[") {
			continue
		}
		idx := strings.Index(line, tag)
		if idx < 0 {
			continue
		}
		b.WriteString(line[idx+len(tag):])
		b.WriteByte('\n')
	}
	return b.String()
}
