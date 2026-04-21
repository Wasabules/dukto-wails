package discovery

import (
	"fmt"
	"net"
	"net/netip"
)

// Interface is an IPv4 non-loopback interface selected for broadcasting.
//
// Name is the OS interface name, used for diagnostics. IP is the interface's
// IPv4 unicast address. Broadcast is the derived directed-broadcast address
// (all host bits set within the subnet mask); discovery HELLOs are sent here.
type Interface struct {
	Name      string
	IP        netip.Addr
	Broadcast netip.Addr
}

// InterfaceEnumerator returns the set of IPv4 interfaces eligible for
// discovery broadcasts. Implementations should return only UP, non-loopback
// interfaces with a usable IPv4 address and computable broadcast address.
//
// Exposed as a function type (rather than the result of a single snapshot) so
// that the Messenger re-enumerates on every broadcast pass — this matches Qt
// Messenger behavior and picks up interface changes (VPN up/down, Wi-Fi
// reconnect) without restarting the service.
type InterfaceEnumerator func() ([]Interface, error)

// SystemInterfaces enumerates real OS interfaces via the standard library.
//
// Selection rules:
//   - skip DOWN interfaces
//   - skip loopback interfaces
//   - for each remaining interface, emit one Interface per IPv4 unicast
//     address whose mask yields a usable directed-broadcast address
//
// Addresses without an IPv4 mask (e.g. a /32 point-to-point link) are skipped:
// directed broadcast is ill-defined there and sending to 255.255.255.255 would
// be the caller's responsibility if ever needed.
func SystemInterfaces() ([]Interface, error) {
	raw, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("dukto discovery: net.Interfaces: %w", err)
	}
	var out []Interface
	for _, ifi := range raw {
		if ifi.Flags&net.FlagUp == 0 {
			continue
		}
		if ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			mask := ipnet.Mask
			if len(mask) != net.IPv4len {
				// IPv4-mapped on an IPv6 mask — derive a 4-byte mask.
				if len(mask) == net.IPv6len {
					mask = mask[12:]
				} else {
					continue
				}
			}
			bcast := directedBroadcast(ip4, mask)
			if bcast == nil {
				continue
			}
			ipAddr, ok := netip.AddrFromSlice(ip4)
			if !ok {
				continue
			}
			bcAddr, ok := netip.AddrFromSlice(bcast)
			if !ok {
				continue
			}
			out = append(out, Interface{
				Name:      ifi.Name,
				IP:        ipAddr.Unmap(),
				Broadcast: bcAddr.Unmap(),
			})
		}
	}
	return out, nil
}

// directedBroadcast returns the IPv4 directed-broadcast address for ip/mask,
// or nil for degenerate cases (nil inputs, /32 host route, or broadcast equal
// to the host — which cannot receive its own broadcast meaningfully).
func directedBroadcast(ip net.IP, mask net.IPMask) net.IP {
	if len(ip) != net.IPv4len || len(mask) != net.IPv4len {
		return nil
	}
	ones, bits := mask.Size()
	if bits != 32 || ones == 32 {
		return nil
	}
	bcast := make(net.IP, net.IPv4len)
	for i := range net.IPv4len {
		bcast[i] = ip[i] | ^mask[i]
	}
	return bcast
}
