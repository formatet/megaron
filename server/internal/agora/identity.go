// Package agora provides the external Matrix account integration.
// It has no dependencies on other Megaron internal packages.
package agora

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Localpart preserves only the characters allowed by the account contract.
// An empty result is an error rather than an invented replacement identity.
func Localpart(name string) (string, error) {
	var out strings.Builder
	for _, r := range strings.ToLower(norm.NFKD.String(name)) {
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r) {
			continue
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "", errors.New("Agora name has no supported characters")
	}
	return out.String(), nil
}

// NewPassword generates a command-safe secret. Callers must never persist it.
func NewPassword() (string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", errors.New("could not generate Agora password")
	}
	return "M" + base64.RawURLEncoding.EncodeToString(secret[:]), nil
}
