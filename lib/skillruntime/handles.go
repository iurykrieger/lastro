// Package skillruntime bootstraps a configured *lifecycle.Lifecycle from
// .harness/ for B5's skill scripts to use, and exposes helpers for the
// "<sensor-id>:<run-id>" handle format the skills hand to the user.
package skillruntime

import (
	"fmt"
	"strings"
)

// ulidLen is the canonical text length of a ULID per
// github.com/oklog/ulid/v2 (Crockford base32, 26 chars).
const ulidLen = 26

// ParseHandle splits a "<sensor-id>:<run-id>" string into its components
// and validates that each half is a ULID-shaped 26-char base32 token.
func ParseHandle(s string) (sensorID, runID string, err error) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return "", "", fmt.Errorf("skillruntime: handle missing ':' separator: %q", s)
	}
	sensorID, runID = s[:i], s[i+1:]
	if err := validateULIDShape(sensorID); err != nil {
		return "", "", fmt.Errorf("skillruntime: sensor id: %w", err)
	}
	if err := validateULIDShape(runID); err != nil {
		return "", "", fmt.Errorf("skillruntime: run id: %w", err)
	}
	return sensorID, runID, nil
}

// FormatHandle composes "<sensor-id>:<run-id>".
func FormatHandle(sensorID, runID string) string {
	return sensorID + ":" + runID
}

// validateULIDShape checks length and Crockford base32 alphabet.
// We do NOT parse the timestamp; B2's lifecycle uses oklog/ulid for that
// when it matters. This is a fast structural check at the boundary.
func validateULIDShape(s string) error {
	if len(s) != ulidLen {
		return fmt.Errorf("expected %d chars, got %d (%q)", ulidLen, len(s), s)
	}
	for _, r := range s {
		if !isCrockfordBase32(r) {
			return fmt.Errorf("non-base32 char %q in %q", r, s)
		}
	}
	return nil
}

func isCrockfordBase32(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'A' && r <= 'Z' && r != 'I' && r != 'L' && r != 'O' && r != 'U':
		return true
	case r >= 'a' && r <= 'z' && r != 'i' && r != 'l' && r != 'o' && r != 'u':
		return true
	}
	return false
}
