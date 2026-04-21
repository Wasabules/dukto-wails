// Package discovery implements Dukto UDP peer discovery on top of
// internal/protocol. It mirrors the behavior of the Qt Messenger class:
// periodic HELLO broadcasts across every UP IPv4 non-loopback interface,
// unicast replies, self-echo suppression, and a guard against VPN-induced
// broadcast storms.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"dukto/internal/protocol"
)

// Peer is a discovered remote Dukto instance.
type Peer struct {
	Addr      netip.Addr
	Port      uint16
	Signature string
}

// EventKind signals whether a peer was discovered or has left.
type EventKind int

const (
	EventFound EventKind = iota
	EventGone
)

// Event is emitted on the Messenger's Events channel.
type Event struct {
	Kind EventKind
	Peer Peer
}

// broadcastStormThreshold matches the Qt guard: if a single source address
// sends more than this many datagrams between two broadcast passes while it
// matches one of our local IPs, we classify it as a bad address and ignore it
// for the rest of the session. The threshold (>5) is preserved for
// interop behavior parity.
const broadcastStormThreshold = 5

// Messenger runs Dukto discovery on a single UDP socket.
//
// Zero-value is not usable; construct with New. The Messenger is safe to use
// from multiple goroutines once Start returns. Call Stop exactly once.
type Messenger struct {
	port  uint16
	sigFn func() string
	ifaces InterfaceEnumerator
	now    func() time.Time

	conn net.PacketConn

	mu       sync.Mutex
	peers    map[netip.Addr]Peer
	ports    map[uint16]struct{}
	localIPs map[netip.Addr]int
	badIPs   map[netip.Addr]struct{}
	// lastHello tracks the last time we accepted a datagram from a given
	// source IP, to enforce HelloCooldown. Kept on the messenger itself
	// rather than a separate state struct because it's cheap (a handful of
	// peers on a LAN) and needs the same lock as peers/badIPs.
	lastHello map[netip.Addr]time.Time
	cooldown  time.Duration

	events chan Event
	stop   chan struct{}
	wg     sync.WaitGroup
}

// Config configures a Messenger.
type Config struct {
	// Port is the UDP port to bind. Zero means protocol.DefaultPort.
	Port uint16

	// SignatureFunc returns the current signature to broadcast. It is called
	// on every outbound HELLO so that changes to the buddy name / hostname
	// propagate without restarting the Messenger.
	SignatureFunc func() string

	// Interfaces, if non-nil, overrides the default system interface
	// enumerator. Exposed for testing; production code leaves it nil.
	Interfaces InterfaceEnumerator

	// HelloCooldown, if > 0, drops HELLO datagrams received within this
	// window from the same source IP. Zero disables the gate. Used to
	// blunt broadcast-storm attackers without impacting legitimate peers
	// (who send ~one HELLO per 10s).
	HelloCooldown time.Duration
}

// New builds a Messenger. It does not open any socket; call Start.
func New(cfg Config) *Messenger {
	port := cfg.Port
	if port == 0 {
		port = protocol.DefaultPort
	}
	sig := cfg.SignatureFunc
	if sig == nil {
		sig = func() string { return "" }
	}
	ifaces := cfg.Interfaces
	if ifaces == nil {
		ifaces = SystemInterfaces
	}
	return &Messenger{
		port:      port,
		sigFn:     sig,
		ifaces:    ifaces,
		now:       time.Now,
		peers:     map[netip.Addr]Peer{},
		ports:     map[uint16]struct{}{protocol.DefaultPort: {}},
		localIPs:  map[netip.Addr]int{},
		badIPs:    map[netip.Addr]struct{}{},
		lastHello: map[netip.Addr]time.Time{},
		cooldown:  cfg.HelloCooldown,
		events:    make(chan Event, 16),
		stop:      make(chan struct{}),
	}
}

// SetHelloCooldown updates the per-IP rate-limit at runtime. Zero disables
// the gate. Safe to call while the messenger is running.
func (m *Messenger) SetHelloCooldown(d time.Duration) {
	m.mu.Lock()
	m.cooldown = d
	m.mu.Unlock()
}

// Start opens the UDP socket and begins the receive loop. It returns once the
// socket is bound; the loop runs in a background goroutine until ctx is
// cancelled or Stop is called.
func (m *Messenger) Start(ctx context.Context) error {
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: int(m.port)}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("dukto discovery: bind udp4 %d: %w", m.port, err)
	}
	m.conn = conn

	m.wg.Add(1)
	go m.readLoop(ctx)
	return nil
}

// Stop shuts down the Messenger. It broadcasts a goodbye, closes the socket,
// and waits for the receive goroutine to exit. Safe to call more than once;
// subsequent calls are no-ops.
func (m *Messenger) Stop() error {
	select {
	case <-m.stop:
		return nil
	default:
	}
	close(m.stop)
	// Best-effort goodbye; ignore errors since we're shutting down anyway.
	_ = m.SayGoodbye()
	var err error
	if m.conn != nil {
		err = m.conn.Close()
	}
	m.wg.Wait()
	close(m.events)
	return err
}

// Events returns the channel on which EventFound and EventGone are delivered.
// It is closed when the Messenger stops.
func (m *Messenger) Events() <-chan Event { return m.events }

// Peers returns a snapshot of the current peer list (default-port peers only,
// matching the Qt Messenger's tracking behavior — peers announced via
// HELLO_PORT_* are exposed to callers via Events but not tracked here).
func (m *Messenger) Peers() []Peer {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Peer, 0, len(m.peers))
	for _, p := range m.peers {
		out = append(out, p)
	}
	return out
}

// SayHello broadcasts a HELLO from every UP IPv4 non-loopback interface on
// every known port (default port + every port observed from PORT peers).
func (m *Messenger) SayHello() error {
	msg := protocol.BuddyMessage{
		Type:      protocol.HelloBroadcastType(m.port),
		Port:      m.port,
		Signature: m.sigFn(),
	}
	return m.broadcast(msg)
}

// SayGoodbye broadcasts a GOODBYE. Called implicitly by Stop.
func (m *Messenger) SayGoodbye() error {
	return m.broadcast(protocol.Goodbye())
}

// broadcast sends msg on every UP IPv4 non-loopback interface's broadcast
// address, once per known target port. It also refreshes the "own IPs" map so
// the receive loop can suppress self-echoes.
func (m *Messenger) broadcast(msg protocol.BuddyMessage) error {
	if m.conn == nil {
		return errors.New("dukto discovery: not started")
	}
	ifaces, err := m.ifaces()
	if err != nil {
		return fmt.Errorf("dukto discovery: enumerate interfaces: %w", err)
	}

	payload := msg.Serialize()

	m.mu.Lock()
	// Refresh self-echo counters for this broadcast pass.
	m.localIPs = make(map[netip.Addr]int, len(ifaces))
	ports := make([]uint16, 0, len(m.ports))
	for p := range m.ports {
		ports = append(ports, p)
	}
	m.mu.Unlock()

	var firstErr error
	for _, iface := range ifaces {
		if !iface.IP.Is4() || iface.IP.IsLoopback() {
			continue
		}
		m.mu.Lock()
		if _, bad := m.badIPs[iface.IP]; bad {
			m.mu.Unlock()
			continue
		}
		m.localIPs[iface.IP] = 0
		m.mu.Unlock()

		for _, port := range ports {
			dst := &net.UDPAddr{IP: iface.Broadcast.AsSlice(), Port: int(port)}
			if _, werr := m.conn.WriteTo(payload, dst); werr != nil && firstErr == nil {
				firstErr = werr
			}
		}
	}
	return firstErr
}

// readLoop drains the UDP socket until Stop is called or ctx is cancelled.
func (m *Messenger) readLoop(ctx context.Context) {
	defer m.wg.Done()
	buf := make([]byte, 65536)
	for {
		select {
		case <-m.stop:
			return
		case <-ctx.Done():
			return
		default:
		}
		// Periodic deadline so Stop/ctx cancellation is observed.
		_ = m.conn.SetReadDeadline(m.now().Add(500 * time.Millisecond))
		n, src, err := m.conn.ReadFrom(buf)
		if err != nil {
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				continue
			}
			// Any non-timeout error (incl. closed socket) means we're done.
			return
		}
		udp, ok := src.(*net.UDPAddr)
		if !ok {
			continue
		}
		ipa, ok := netip.AddrFromSlice(udp.IP)
		if !ok {
			continue
		}
		ipa = ipa.Unmap()
		m.handleDatagram(buf[:n], ipa)
	}
}

// handleDatagram is the decision logic called once per received datagram.
// Separated from the socket loop for unit testing.
func (m *Messenger) handleDatagram(data []byte, src netip.Addr) {
	m.mu.Lock()
	if _, bad := m.badIPs[src]; bad {
		m.mu.Unlock()
		return
	}
	if count, isSelf := m.localIPs[src]; isSelf {
		count++
		m.localIPs[src] = count
		if count > broadcastStormThreshold {
			m.badIPs[src] = struct{}{}
		}
		m.mu.Unlock()
		return
	}
	// Per-source cooldown: drop datagrams arriving inside the configured
	// window. Evaluated after the self-echo check so our own broadcasts
	// don't use up a peer's slot.
	if m.cooldown > 0 {
		now := m.now()
		if last, ok := m.lastHello[src]; ok && now.Sub(last) < m.cooldown {
			m.mu.Unlock()
			return
		}
		m.lastHello[src] = now
	}
	m.mu.Unlock()

	msg, err := protocol.ParseBuddyMessage(data)
	if err != nil {
		return
	}
	m.dispatch(msg, src)
}

// dispatch applies the Qt Messenger::processMessage decision table.
func (m *Messenger) dispatch(msg protocol.BuddyMessage, src netip.Addr) {
	switch msg.Type {
	case protocol.MsgHelloBroadcast, protocol.MsgHelloUnicast:
		peer := Peer{Addr: src, Port: protocol.DefaultPort, Signature: msg.Signature}
		m.mu.Lock()
		m.peers[src] = peer
		m.mu.Unlock()
		if msg.Type == protocol.MsgHelloBroadcast {
			m.sendUnicastHello(src, protocol.DefaultPort)
		}
		m.emit(Event{Kind: EventFound, Peer: peer})

	case protocol.MsgGoodbye:
		m.mu.Lock()
		peer, ok := m.peers[src]
		if ok {
			delete(m.peers, src)
		}
		m.mu.Unlock()
		if ok {
			m.emit(Event{Kind: EventGone, Peer: peer})
		}

	case protocol.MsgHelloPortBroadcast, protocol.MsgHelloPortUnicast:
		peer := Peer{Addr: src, Port: msg.Port, Signature: msg.Signature}
		// Matches Qt: PORT peers aren't stored in the peers map, but their
		// port is tracked so future broadcasts reach them.
		m.mu.Lock()
		m.ports[msg.Port] = struct{}{}
		m.mu.Unlock()
		if msg.Type == protocol.MsgHelloPortBroadcast {
			m.sendUnicastHello(src, msg.Port)
		}
		m.emit(Event{Kind: EventFound, Peer: peer})
	}
}

// UnicastHello sends a HELLO to a specific addr:port. Unlike SayHello (which
// broadcasts), this is used to manually poke a peer that isn't reachable via
// UDP broadcast — typically one on a different subnet added via settings.
// Errors are swallowed the same way sendUnicastHello does; the UI should
// retry on the next tick.
func (m *Messenger) UnicastHello(addr netip.Addr, port uint16) {
	if port == 0 {
		port = protocol.DefaultPort
	}
	m.sendUnicastHello(addr, port)
}

// sendUnicastHello sends a HELLO reply to (addr, port). Picks HELLO_UNICAST
// or HELLO_PORT_UNICAST based on the local bind port.
func (m *Messenger) sendUnicastHello(addr netip.Addr, port uint16) {
	if m.conn == nil {
		return
	}
	msg := protocol.BuddyMessage{
		Type:      protocol.HelloUnicastType(m.port),
		Port:      m.port,
		Signature: m.sigFn(),
	}
	dst := &net.UDPAddr{IP: addr.AsSlice(), Port: int(port)}
	_, _ = m.conn.WriteTo(msg.Serialize(), dst)
}

// emit pushes an event, dropping the oldest if the buffer is full. Dropping
// under backpressure is preferable to blocking the receive loop.
func (m *Messenger) emit(ev Event) {
	select {
	case m.events <- ev:
	default:
		// Drop one to make room.
		select {
		case <-m.events:
		default:
		}
		select {
		case m.events <- ev:
		default:
		}
	}
}
