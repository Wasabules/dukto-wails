// Package eff bundles the EFF Short Wordlist 1 (1296 words, ~10.34 bits/word)
// and exposes helpers for the Dukto v2 pairing flow:
//
//   - Generate(n) → n random words for the user to read out / scan.
//   - DerivePSK(passphrase) → 32-byte PSK suitable for Noise XXpsk2.
//
// The wordlist is published by the Electronic Frontier Foundation under CC-BY
// and copied verbatim into eff_short_wordlist.txt. Source:
// https://www.eff.org/files/2016/09/08/eff_short_wordlist_1.txt
package eff

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/hkdf"
)

//go:embed eff_short_wordlist.txt
var raw string

var Words = parseWords(raw)

func parseWords(blob string) []string {
	lines := strings.Split(strings.TrimSpace(blob), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		w := strings.TrimSpace(l)
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// PSKInfo is the fixed HKDF info string for v2 pairing PSKs. Bumping this
// invalidates every pairing in flight, so it lives forever.
const PSKInfo = "DUKTO-PSK-v1"

// JoinSeparator is the single character separating words in a passphrase.
// Both peers must canonicalise to this character before deriving the PSK.
const JoinSeparator = "-"

// Generate returns n random EFF short words joined by [JoinSeparator]. n must
// be in [3, 8]; 5 is the design-doc default (~51.7 bits of entropy).
func Generate(n int) (string, error) {
	if n < 3 || n > 8 {
		return "", fmt.Errorf("eff: word count %d out of range [3,8]", n)
	}
	picks := make([]string, n)
	var idx [2]byte
	for i := 0; i < n; i++ {
		// Rejection-sampled draw from [0, len(Words)) using uint16 — bias
		// is < 1/65536 of the way the modulo falls, which is good enough
		// for ephemeral pairing PSKs.
		for {
			if _, err := rand.Read(idx[:]); err != nil {
				return "", fmt.Errorf("eff: rand: %w", err)
			}
			n := int(binary.LittleEndian.Uint16(idx[:]))
			if n >= 65536-(65536%len(Words)) {
				continue // reject to keep the distribution uniform
			}
			picks[i] = Words[n%len(Words)]
			break
		}
	}
	return strings.Join(picks, JoinSeparator), nil
}

// Canonicalise normalises a user-entered passphrase: lowercases it, splits on
// whitespace / dashes / underscores, drops empty parts, and rejoins with
// [JoinSeparator]. Both peers run this before [DerivePSK] so capitalisation
// or punctuation drift in either direction doesn't break the handshake.
func Canonicalise(s string) string {
	splitter := func(r rune) bool {
		return r == ' ' || r == '\t' || r == '-' || r == '_' || r == ','
	}
	parts := strings.FieldsFunc(strings.ToLower(s), splitter)
	return strings.Join(parts, JoinSeparator)
}

// DerivePSK runs HKDF-SHA256 with the canonical passphrase as IKM, the
// fixed [PSKInfo] string as info, and a zero-length salt — yielding a
// 32-byte PSK suitable for Noise XXpsk2. Mirrors the formula on the
// Android side; both stacks must match bytewise.
func DerivePSK(passphrase string) ([]byte, error) {
	canon := Canonicalise(passphrase)
	if canon == "" {
		return nil, errors.New("eff: empty passphrase")
	}
	hash := sha256.New
	r := hkdf.New(hash, []byte(canon), nil, []byte(PSKInfo))
	key := make([]byte, 32)
	if _, err := r.Read(key); err != nil {
		return nil, fmt.Errorf("eff: hkdf: %w", err)
	}
	return key, nil
}

// Verify exposes a constant-time byte compare for tests and call-sites
// that want to compare two derived PSKs. Plain byte slices in Go don't
// have one in stdlib outside crypto/subtle.
func Verify(a, b []byte) bool { return hmac.Equal(a, b) }
