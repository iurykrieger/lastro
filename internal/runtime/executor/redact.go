package executor

import (
	"bytes"
	"sort"
	"sync"
)

// minRedactLen guards against registering trivial values ("1", "dev")
// whose masking would corrupt unrelated output.
const minRedactLen = 4

// redactedPlaceholder replaces each registered value occurrence.
const redactedPlaceholder = "***"

// redactor masks registered secret values in every byte stream the run
// persists (raw.log, signals.jsonl) and in the pump choke point before
// lines reach signal matching — so matched_line evidence and aggregate
// heal hints never carry a secret. Values are matched exactly (no
// encodings); registration sources are the env_file ambient view and
// ref-derived step env: values. All methods are nil-receiver-safe and
// goroutine-safe (stdout/stderr pumps write concurrently).
type redactor struct {
	mu     sync.RWMutex
	values []string // longest-first so an extended secret masks before its prefix
}

// Add registers one secret value. Short values and duplicates are ignored.
func (r *redactor) Add(v string) {
	if r == nil || len(v) < minRedactLen {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.values {
		if existing == v {
			return
		}
	}
	r.values = append(r.values, v)
	sort.Slice(r.values, func(i, j int) bool { return len(r.values[i]) > len(r.values[j]) })
}

// Apply returns b with every registered value replaced by the placeholder.
func (r *redactor) Apply(b []byte) []byte {
	if r == nil {
		return b
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.values {
		b = bytes.ReplaceAll(b, []byte(v), []byte(redactedPlaceholder))
	}
	return b
}
