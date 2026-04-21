// Package platform collects the small bits of host introspection needed to
// build a Dukto discovery signature: the interactive username, the machine's
// short hostname, and the platform token string that the Qt reference
// implementation advertises.
//
// The tokens are load-bearing — peers filter and display buddies by them — so
// they are fixed strings defined in internal/protocol, not localized or
// derived from runtime.GOOS directly.
package platform

import (
	"os"
	"os/user"
	"runtime"
	"strings"

	"dukto/internal/protocol"
)

// Username returns the current interactive user's login name.
//
// Preference order: $USER / $USERNAME (so a user override in the shell wins,
// matching how Qt's Util::getSystemUser consults env vars first), then
// os/user. Falls back to "unknown" if nothing is available, rather than
// returning an error — a blank signature would be rejected by the wire-format
// validator and we want the app to keep running.
func Username() string {
	for _, env := range []string{"USER", "USERNAME"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	if u, err := user.Current(); err == nil {
		if u.Username != "" {
			// On Windows user.Current().Username is "DOMAIN\\user"; strip the
			// domain for consistency with the Qt version.
			if i := strings.LastIndex(u.Username, `\`); i >= 0 {
				return u.Username[i+1:]
			}
			return u.Username
		}
	}
	return "unknown"
}

// Hostname returns the machine's short hostname (no trailing domain).
//
// Some hosts report a fully-qualified name from os.Hostname; we trim at the
// first '.' because the Qt reference surfaces QHostInfo::localHostName which
// is already short on every platform we target.
func Hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	if i := strings.Index(h, "."); i > 0 {
		h = h[:i]
	}
	return h
}

// Name returns the fixed Dukto platform token for the current GOOS.
//
// Unknown GOOS values map to protocol.PlatformUnknown rather than panicking —
// we'd rather be discoverable with a generic label than not at all.
func Name() string {
	switch runtime.GOOS {
	case "windows":
		return protocol.PlatformWindows
	case "darwin":
		return protocol.PlatformMacintosh
	case "linux":
		return protocol.PlatformLinux
	case "android":
		return protocol.PlatformAndroid
	default:
		return protocol.PlatformUnknown
	}
}

// Signature is a convenience that returns the full signature string this host
// should advertise: "<user> at <host> (<platform>)".
func Signature() string {
	return protocol.BuildSignature(Username(), Hostname(), Name())
}
