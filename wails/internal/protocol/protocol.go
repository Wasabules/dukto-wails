// Package protocol implements the Dukto wire format: UDP peer discovery and
// TCP file/text transfer, compatible with the Qt Dukto app.
//
// See docs/PROTOCOL.md at the repository root for the authoritative spec.
package protocol

import "fmt"

// DefaultPort is the default UDP discovery and TCP transfer port.
const DefaultPort uint16 = 4644

// AvatarPortOffset is added to the UDP port to get the avatar HTTP port.
// The Qt app serves a 64×64 PNG at http://<peer>:<DefaultPort+1>/dukto/avatar.
const AvatarPortOffset uint16 = 1

// Platform tokens that legacy Dukto peers recognise in the discovery signature.
// Using anything else will cause other clients to render the peer with the
// "unknown" logo. Keep in sync with Qt Platform::getPlatformName.
const (
	PlatformWindows   = "Windows"
	PlatformMacintosh = "Macintosh"
	PlatformLinux     = "Linux"
	PlatformAndroid   = "Android"
	PlatformUnknown   = "Unknown"
)

// BuildSignature composes the UDP HELLO signature string used for peer
// identification: "<user> at <host> (<platform>)". The " at " literal and the
// parentheses around the platform are load-bearing — legacy peers display the
// full string verbatim in their UI.
func BuildSignature(user, host, platform string) string {
	return fmt.Sprintf("%s at %s (%s)", user, host, platform)
}
