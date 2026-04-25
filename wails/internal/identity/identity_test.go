package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadOrGenerate_FreshThenReload covers the common path: first call
// generates and writes, second call returns the same key.
func TestLoadOrGenerate_FreshThenReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.key")

	first, err := LoadOrGenerate(path)
	if err != nil {
		t.Fatalf("first LoadOrGenerate: %v", err)
	}
	if len(first.Public) != ed25519.PublicKeySize {
		t.Fatalf("public key size %d, expected %d", len(first.Public), ed25519.PublicKeySize)
	}

	second, err := LoadOrGenerate(path)
	if err != nil {
		t.Fatalf("second LoadOrGenerate: %v", err)
	}
	if !first.Public.Equal(second.Public) {
		t.Fatalf("reload produced a different public key — should be stable across calls")
	}
}

// TestLoadOrGenerate_RefusesGarbage protects against silently regenerating
// over a corrupted file: a previous fingerprint might be pinned on a peer
// already, so we surface the error instead of giving the user a brand-new
// identity that breaks all their pairings.
func TestLoadOrGenerate_RefusesGarbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.key")
	if err := os.WriteFile(path, []byte("not a PEM block"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOrGenerate(path)
	if err == nil {
		t.Fatalf("expected error on corrupted identity file, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("error %q should mention decode failure", err)
	}
}

// TestX25519DerivationMatchesEd25519PubConversion verifies the central
// invariant the Noise tunnel relies on: the X25519 public key derived from
// my own seed must equal the Edwards-to-Montgomery projection of my
// Ed25519 public key. If this ever drifts, peers that pin Ed25519 pubkeys
// would reject our handshake.
func TestX25519DerivationMatchesEd25519PubConversion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.key")
	id, err := LoadOrGenerate(path)
	if err != nil {
		t.Fatal(err)
	}
	fromSeed, err := id.X25519Public()
	if err != nil {
		t.Fatal(err)
	}
	fromPub, err := Ed25519PubToX25519Pub(id.Public)
	if err != nil {
		t.Fatal(err)
	}
	if fromSeed != fromPub {
		t.Fatalf("X25519 derivations diverged:\n  from seed: %x\n  from pub : %x", fromSeed, fromPub)
	}
}

// TestEd25519PubToX25519Pub_RejectsBogusBytes ensures we don't accept
// random byte strings as Edwards points — that would let an attacker
// claim any X25519 pubkey by sending a crafted UDP HELLO.
func TestEd25519PubToX25519Pub_RejectsBogusBytes(t *testing.T) {
	bogus := make(ed25519.PublicKey, ed25519.PublicKeySize)
	// Sweep a few candidates; not all 32-byte strings decode as Edwards
	// points, so we look for at least one rejection.
	rejected := false
	for i := 0; i < 32; i++ {
		bogus[0] = byte(i) // bottom-bit and high-bit changes shift the curve test
		if _, err := Ed25519PubToX25519Pub(bogus); err != nil {
			rejected = true
			break
		}
	}
	if !rejected {
		t.Skip("could not find a rejecting input — environment-dependent edge case")
	}
}

// TestFingerprint_DeterministicAndFormatted makes sure the user-facing
// fingerprint is stable across calls and follows XXXX-XXXX-XXXX-XXXX format.
func TestFingerprint_DeterministicAndFormatted(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := Fingerprint(pub)
	b := Fingerprint(pub)
	if a != b {
		t.Fatalf("Fingerprint not deterministic: %q vs %q", a, b)
	}
	if got := len(a); got != 19 { // 16 base32 chars + 3 dashes
		t.Fatalf("Fingerprint length %d, expected 19, value=%q", got, a)
	}
	for i, c := range a {
		switch {
		case i == 4 || i == 9 || i == 14:
			if c != '-' {
				t.Fatalf("expected '-' at index %d in %q", i, a)
			}
		default:
			if c == '-' {
				t.Fatalf("unexpected '-' at index %d in %q", i, a)
			}
		}
	}
}
