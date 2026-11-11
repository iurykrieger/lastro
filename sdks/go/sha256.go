package lastro

import (
	"crypto/sha256"
	"encoding/hex"
	"unicode/utf16"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// utf16Units is s as JavaScript sees it: offsets in intent maps count UTF-16 code units.
func utf16Units(s string) []uint16 { return utf16.Encode([]rune(s)) }

func fromUTF16(u []uint16) string { return string(utf16.Decode(u)) }
