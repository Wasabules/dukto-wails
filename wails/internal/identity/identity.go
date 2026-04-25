// Package identity manages this install's long-term Ed25519 keypair.
//
// The keypair is generated lazily on first call to LoadOrGenerate and
// persisted under the dukto config dir. It is the cryptographic anchor for
// the v2 encrypted overlay (see docs/SECURITY_v2.md):
//
//   - M1 (this milestone): the keypair exists and the fingerprint is shown
//     to the user. No network use yet.
//   - M2: the public key signs UDP discovery datagrams (0x06/0x07).
//   - M3: the static keypair authenticates Noise XX TCP handshakes.
//
// The private key is stored as PKCS#8 DER under file mode 0600, alongside
// settings.json. A future hardening pass will move it to the OS keychain
// (DPAPI / Keychain / libsecret); the file path is retained so a downgrade
// can still find the key.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Identity wraps an Ed25519 keypair plus its derived fingerprint.
type Identity struct {
	// Public is the 32-byte Ed25519 public key. Safe to share.
	Public ed25519.PublicKey

	// Private is the 64-byte Ed25519 private key (seed || public). Never
	// log, never serialise outside of LoadOrGenerate's persistence path.
	Private ed25519.PrivateKey
}

// Fingerprint returns the canonical user-visible 16-character RFC4648
// base32 of the first 10 bytes of SHA-256(pub_key), grouped as XXXX-XXXX-...
//
// 80 bits of fingerprint comfortably exceed the security level our threat
// model requires for collision resistance (no global namespace, only a
// handful of paired peers per device).
func (id Identity) Fingerprint() string {
	return Fingerprint(id.Public)
}

// Fingerprint exposes the standalone derivation so peers' fingerprints can
// be computed from received public keys without instantiating a full
// Identity.
func Fingerprint(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h[:10])
	// Format as XXXX-XXXX-XXXX-XXXX (4 groups of 4).
	var b strings.Builder
	for i, r := range enc {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// LoadOrGenerate reads the identity stored at path. If the file is missing
// it generates a fresh keypair and persists it under file mode 0600.
//
// Errors from this function should be treated as fatal: an unreadable
// identity at startup means the security stack can't initialise. We do not
// silently regenerate over a corrupted file — a previous fingerprint
// might already be pinned by the user's peers.
func LoadOrGenerate(path string) (Identity, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return generateAndPersist(path)
	}
	if err != nil {
		return Identity{}, fmt.Errorf("identity: read %s: %w", path, err)
	}
	id, err := decode(data)
	if err != nil {
		return Identity{}, fmt.Errorf("identity: decode %s: %w (refuse to regenerate over a non-empty file — move it aside if you really want a new key)", path, err)
	}
	return id, nil
}

func generateAndPersist(path string) (Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, fmt.Errorf("identity: generate: %w", err)
	}
	id := Identity{Public: pub, Private: priv}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Identity{}, fmt.Errorf("identity: mkdir %s: %w", filepath.Dir(path), err)
	}
	encoded := encode(id)
	// Write atomically with a temp file then rename, so a crash mid-write
	// can't leave us with a half-baked identity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return Identity{}, fmt.Errorf("identity: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return Identity{}, fmt.Errorf("identity: rename: %w", err)
	}
	return id, nil
}

// encode serialises the keypair as a PEM-wrapped 64-byte private key blob.
// We do not use PKCS#8 deliberately: it's an Ed25519 raw seed, no need to
// add ASN.1 framing. The PEM wrapper serves only to make the file
// human-recognisable on disk.
func encode(id Identity) []byte {
	block := &pem.Block{
		Type:  "DUKTO ED25519 PRIVATE KEY",
		Bytes: id.Private,
	}
	return pem.EncodeToMemory(block)
}

func decode(data []byte) (Identity, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "DUKTO ED25519 PRIVATE KEY" {
		return Identity{}, errors.New("not a DUKTO ED25519 PRIVATE KEY PEM block")
	}
	if len(block.Bytes) != ed25519.PrivateKeySize {
		return Identity{}, fmt.Errorf("private key length %d, expected %d", len(block.Bytes), ed25519.PrivateKeySize)
	}
	priv := ed25519.PrivateKey(block.Bytes)
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return Identity{}, errors.New("private key did not yield an ed25519 public key")
	}
	return Identity{Public: pub, Private: priv}, nil
}
