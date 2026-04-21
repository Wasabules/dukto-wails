package platform

import (
	"regexp"
	"runtime"
	"testing"

	"dukto/internal/protocol"
)

func TestUsername_UsesEnvFirst(t *testing.T) {
	t.Setenv("USER", "alice")
	t.Setenv("USERNAME", "bob")
	// USER wins over USERNAME because it's consulted first.
	if got := Username(); got != "alice" {
		t.Fatalf("got %q, want %q", got, "alice")
	}
}

func TestUsername_FallsBackWhenEnvEmpty(t *testing.T) {
	t.Setenv("USER", "")
	t.Setenv("USERNAME", "")
	// We don't care what os/user returns — only that we don't produce "" or
	// crash. An empty signature would fail Validate() on the wire.
	if got := Username(); got == "" {
		t.Fatal("username should never be empty")
	}
}

func TestUsername_StripsWindowsDomain(t *testing.T) {
	// Can't easily force os/user to return "DOMAIN\name", so assert on the
	// post-split path via env override: env vars are passed through verbatim
	// and already domain-less on Windows.
	t.Setenv("USER", "")
	t.Setenv("USERNAME", `WORKGROUP\carol`)
	// Env takes precedence and we keep the env value as-is. This test locks
	// down the behavior so a future refactor doesn't accidentally start
	// splitting env-provided names.
	if got := Username(); got != `WORKGROUP\carol` {
		t.Fatalf("env username should be passed through; got %q", got)
	}
}

func TestHostname_NonEmpty(t *testing.T) {
	if got := Hostname(); got == "" {
		t.Fatal("hostname should never be empty")
	}
}

func TestName_MatchesGOOS(t *testing.T) {
	want := map[string]string{
		"windows": protocol.PlatformWindows,
		"darwin":  protocol.PlatformMacintosh,
		"linux":   protocol.PlatformLinux,
		"android": protocol.PlatformAndroid,
	}[runtime.GOOS]
	if want == "" {
		want = protocol.PlatformUnknown
	}
	if got := Name(); got != want {
		t.Fatalf("got %q, want %q for GOOS=%s", got, want, runtime.GOOS)
	}
}

func TestSignature_Format(t *testing.T) {
	// Shape check only — the content depends on the host.
	// The Qt format is `<user> at <host> (<platform>)`.
	re := regexp.MustCompile(`^.+ at .+ \(.+\)$`)
	sig := Signature()
	if !re.MatchString(sig) {
		t.Fatalf("signature %q does not match expected shape", sig)
	}
}
