package agora

import (
	"encoding/base64"
	"testing"
)

func TestLocalpartNormalizesIdentity(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"Agamemnon", "agamemnon"}, {"Ágamémnôn", "agamemnon"},
		{"A\u0301game\u0301mnon", "agamemnon"}, {"ＡＧＡＭＥＭＮＯＮ", "agamemnon"},
		{" Héll-o._ １２３!", "hell-o._123"}, {"ﬀ", "ff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Localpart(tc.name)
			if err != nil || got != tc.want {
				t.Fatalf("Localpart = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, name := range []string{"", "!!!", "王", "\u0301"} {
		if _, err := Localpart(name); err == nil {
			t.Fatalf("unsupported name %q accepted", name)
		}
	}
}

func TestNewPasswordIsRandomAndCommandSafe(t *testing.T) {
	a, err := NewPassword()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewPassword()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(a[1:])
	if err != nil || len(decoded) != 32 || a == b {
		t.Fatal("password entropy or command-safe encoding failed")
	}
}
