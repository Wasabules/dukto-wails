package eff

import (
	"strings"
	"testing"
)

func TestWordlistShape(t *testing.T) {
	if got := len(Words); got != 1296 {
		t.Fatalf("EFF short wordlist size %d, expected 1296", got)
	}
	// Spot-check a couple of canonical entries — if these drift, the
	// wordlist file got truncated or replaced and downstream PSKs would
	// silently diverge between peers.
	if Words[0] != "acid" {
		t.Errorf("Words[0] = %q, expected acid", Words[0])
	}
	if Words[len(Words)-1] != "zoom" {
		t.Errorf("Words[last] = %q, expected zoom", Words[len(Words)-1])
	}
}

func TestGenerateDefaults(t *testing.T) {
	pass, err := Generate(5)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(pass, JoinSeparator)
	if len(parts) != 5 {
		t.Fatalf("Generate(5) yielded %d words: %q", len(parts), pass)
	}
	for _, p := range parts {
		if !inWordlist(p) {
			t.Errorf("word %q not in EFF list", p)
		}
	}
}

func TestGenerateRejectsOutOfRange(t *testing.T) {
	if _, err := Generate(2); err == nil {
		t.Fatal("Generate(2) should error")
	}
	if _, err := Generate(9); err == nil {
		t.Fatal("Generate(9) should error")
	}
}

func TestCanonicaliseHandlesPunctuationAndCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Apple-Tiger River_OCEAN, music", "apple-tiger-river-ocean-music"},
		{"  apple  tiger  river  ", "apple-tiger-river"},
		{"apple--tiger___river", "apple-tiger-river"},
	}
	for _, c := range cases {
		if got := Canonicalise(c.in); got != c.want {
			t.Errorf("Canonicalise(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDerivePSKMatchesKnownVector locks in the bytewise output of HKDF for a
// fixed input. If the formula or the canonicaliser ever drifts, both stacks
// would silently produce different PSKs and pairings would fail. The
// expected hex is reproduced verbatim in the Android-side EffTest so a
// CI failure on either side surfaces the cross-stack divergence.
func TestDerivePSKMatchesKnownVector(t *testing.T) {
	psk, err := DerivePSK("Apple-Tiger-River-Ocean-Music")
	if err != nil {
		t.Fatal(err)
	}
	if len(psk) != 32 {
		t.Fatalf("DerivePSK length %d, expected 32", len(psk))
	}
	const expected = "1d5dc6f079bb2b26de421d51dfb3524a83519a819f7060cb0e39dfbc619b91dc"
	if got := hexEncode(psk); got != expected {
		t.Fatalf("DerivePSK known-vector drift\n  got  %s\n  want %s", got, expected)
	}
	// Same passphrase with different formatting must yield the same PSK.
	psk2, err := DerivePSK(" apple tiger  river_ocean,Music ")
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(psk, psk2) {
		t.Fatalf("DerivePSK not stable across whitespace/case variants:\n a=% x\n b=% x", psk, psk2)
	}
	// And a different passphrase must yield a different PSK.
	other, err := DerivePSK("apple-tiger-river-ocean-musics")
	if err != nil {
		t.Fatal(err)
	}
	if Verify(psk, other) {
		t.Fatal("DerivePSK collided on related passphrases")
	}
}

func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0F]
	}
	return string(out)
}

func inWordlist(w string) bool {
	for _, x := range Words {
		if x == w {
			return true
		}
	}
	return false
}
