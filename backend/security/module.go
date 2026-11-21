// Consolidated source; original file boundaries are marked below.
package security

import (
	subtle "crypto/subtle"
	strings "strings"
)

// ---- Source: backend/security/secure_compare.go ----
// SecureCompare performs constant-time comparison of two secret strings.
// Returns false when either operand is empty or lengths differ.
func SecureCompare(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
