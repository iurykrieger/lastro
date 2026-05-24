package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/rogpeppe/go-internal/lockedfile"
)

// registryDoc is the on-disk shape of running_sensors.json.
type registryDoc struct {
	SchemaVersion string   `json:"schema_version"`
	Entries       []Handle `json:"entries"`
}

const registrySchemaVersion = "1.0.0"

// registry wraps the read-modify-write cycle for running_sensors.json
// under a file lock provided by github.com/rogpeppe/go-internal/lockedfile.
//
// mu is kept as a struct field so that the embedded sync.Mutex inside
// lockedfile.Mutex is shared across all callers on the same *registry —
// this provides in-process mutual exclusion in addition to the OS-level
// file lock (which on Linux/flock does not block same-process openers).
type registry struct {
	path string
	mu   *lockedfile.Mutex
}

func newRegistry(runtimeRoot string) *registry {
	p := registryPath(runtimeRoot)
	return &registry{path: p, mu: lockedfile.MutexAt(p)}
}

// List returns a copy of all entries. Shared read lock.
func (r *registry) List() ([]Handle, error) {
	data, err := lockedfile.Read(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("registry: read: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var doc registryDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("registry: decode: %w", err)
	}
	out := make([]Handle, len(doc.Entries))
	copy(out, doc.Entries)
	return out, nil
}

// Find returns the entry matching (sensorID, runID), if any.
func (r *registry) Find(sensorID, runID string) (Handle, bool, error) {
	entries, err := r.List()
	if err != nil {
		return Handle{}, false, err
	}
	for _, e := range entries {
		if e.SensorID == sensorID && e.RunID == runID {
			return e, true, nil
		}
	}
	return Handle{}, false, nil
}

// Append adds h to the registry under exclusive lock.
func (r *registry) Append(h Handle) error {
	return r.mutate(func(doc *registryDoc) {
		doc.Entries = append(doc.Entries, h)
	})
}

// Remove drops the entry matching (sensorID, runID), if any, under
// exclusive lock. No error if not found.
func (r *registry) Remove(sensorID, runID string) error {
	return r.mutate(func(doc *registryDoc) {
		out := doc.Entries[:0]
		for _, e := range doc.Entries {
			if e.SensorID == sensorID && e.RunID == runID {
				continue
			}
			out = append(out, e)
		}
		doc.Entries = out
	})
}

// UpdatePID locates the entry matching (sensorID, runID) and updates its
// PID/PGID. Used when a multi-step sensor enters the next step (the new
// step has a new PID).
func (r *registry) UpdatePID(sensorID, runID string, pid, pgid int) error {
	return r.mutate(func(doc *registryDoc) {
		for i := range doc.Entries {
			if doc.Entries[i].SensorID == sensorID && doc.Entries[i].RunID == runID {
				doc.Entries[i].PID = pid
				doc.Entries[i].PGID = pgid
				return
			}
		}
	})
}

// Prune removes entries for which keep(entry) returns false. Returns the
// number removed.
func (r *registry) Prune(keep func(Handle) bool) (int, error) {
	removed := 0
	err := r.mutate(func(doc *registryDoc) {
		out := doc.Entries[:0]
		for _, e := range doc.Entries {
			if keep(e) {
				out = append(out, e)
			} else {
				removed++
			}
		}
		doc.Entries = out
	})
	return removed, err
}

// mutate performs a read-modify-write under exclusive file lock.
func (r *registry) mutate(fn func(*registryDoc)) error {
	if err := os.MkdirAll(parentDir(r.path), 0o700); err != nil {
		return fmt.Errorf("registry: mkdir: %w", err)
	}
	unlock, err := r.mu.Lock()
	if err != nil {
		return fmt.Errorf("registry: lock: %w", err)
	}
	defer unlock()

	doc := registryDoc{SchemaVersion: registrySchemaVersion}
	if data, err := os.ReadFile(r.path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("registry: decode existing: %w", err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("registry: read existing: %w", err)
	}

	fn(&doc)
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = registrySchemaVersion
	}

	tmp := r.path + ".tmp"
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("registry: encode: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("registry: write tmp: %w", err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("registry: rename: %w", err)
	}
	return nil
}

func parentDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}
