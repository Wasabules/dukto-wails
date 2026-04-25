// Package avatar serves the Dukto avatar HTTP side-channel.
//
// Per docs/PROTOCOL.md §4, every Dukto peer listens on TCP port udp_port+1
// and replies with a 64×64 PNG to a loose family of GET requests:
//
//	GET /                           → PNG
//	GET /dukto/avatar               → PNG
//	GET /dukto/avatar?<anything>    → PNG
//
// Response is HTTP/1.0, Content-Type: image/png, no keep-alive, no conditional
// GET support. The endpoint is discovered purely by convention — peers derive
// the URL from the peer's IP plus protocol.AvatarPortOffset.
//
// We render a placeholder avatar in Go (solid-colour 64×64 PNG, seeded from
// the identity string) rather than shipping a bundled asset: it keeps the
// package dependency-free and lets every peer have a visibly distinct tile
// without any user action. Replacing this with a user-provided PNG is a
// trivial follow-up (swap Renderer).
package avatar

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"dukto/internal/protocol"
)

// Renderer produces the raw PNG bytes that are served on every request. The
// function is called lazily the first time an avatar is requested; results
// are cached in-process until Server.Invalidate is called.
type Renderer func() ([]byte, error)

// Server runs the avatar HTTP endpoint. Zero value is not usable; construct
// with New.
type Server struct {
	render Renderer

	mu     sync.Mutex
	cached []byte
	ln     net.Listener
	srv    *http.Server
}

// New constructs a Server that will call render lazily. If render is nil the
// DefaultRenderer is used with an empty identity (a neutral grey tile).
func New(render Renderer) *Server {
	if render == nil {
		render = DefaultRenderer("")
	}
	return &Server{render: render}
}

// Start binds the listener on udp_port+1 and begins accepting requests. It
// returns once the listener is bound. basePort is the UDP port the
// application uses for discovery; the HTTP port is basePort + AvatarPortOffset.
func (s *Server) Start(basePort uint16) error {
	port := basePort + protocol.AvatarPortOffset
	ln, err := net.Listen("tcp4", ":"+strconv.Itoa(int(port)))
	if err != nil {
		return fmt.Errorf("avatar: bind tcp4 %d: %w", port, err)
	}
	s.mu.Lock()
	s.ln = ln
	// HTTP/1.0 semantics are close enough that http.Server's HTTP/1.1 handling
	// works fine for HTTP/1.0 clients. We disable keep-alives to match the Qt
	// reference, which closes after each response.
	s.srv = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.srv.SetKeepAlivesEnabled(false)
	s.mu.Unlock()

	go func() {
		_ = s.srv.Serve(ln)
	}()
	return nil
}

// Stop shuts down the HTTP server. Safe to call more than once.
func (s *Server) Stop() error {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Close()
}

// Invalidate drops the cached PNG so that the next request re-runs the
// renderer. Useful when the user's identity changes (buddy name rename).
func (s *Server) Invalidate() {
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
}

// SetRenderer swaps the render function. The cache is invalidated so the new
// renderer takes effect on the next request.
func (s *Server) SetRenderer(r Renderer) {
	if r == nil {
		r = DefaultRenderer("")
	}
	s.mu.Lock()
	s.render = r
	s.cached = nil
	s.mu.Unlock()
}

// ServeHTTP implements http.Handler. It matches Qt's loose routing: every
// GET request returns the PNG.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// We intentionally do not 404 on unexpected paths — the Qt reference
	// serves the avatar for `/`, `/dukto/avatar`, and `/dukto/avatar?…`, so
	// any GET on this tiny server might as well yield the PNG.
	data, err := s.data()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Connection", "close")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

func (s *Server) data() ([]byte, error) {
	s.mu.Lock()
	data := s.cached
	render := s.render
	s.mu.Unlock()
	if data != nil {
		return data, nil
	}
	fresh, err := render()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cached = fresh
	s.mu.Unlock()
	return fresh, nil
}

// BytesRenderer returns a Renderer that serves a fixed PNG payload — useful
// when the user has uploaded a custom avatar that should override the default
// initials tile. The bytes are returned verbatim each call.
func BytesRenderer(data []byte) Renderer {
	// Defensive copy so subsequent in-place mutations of the input slice
	// don't corrupt the served bytes.
	cp := make([]byte, len(data))
	copy(cp, data)
	return func() ([]byte, error) { return cp, nil }
}

// DefaultRenderer returns a Renderer that produces a 64×64 PNG with a solid
// background derived from identity (usually the signature string), plus two
// initials drawn as simple block-shaped glyphs.
//
// The goal is to make every peer's tile visibly distinct without shipping a
// font or third-party image library; it is not meant to be pretty. When we
// add real user avatars, swap this out via Server.SetRenderer.
func DefaultRenderer(identity string) Renderer {
	return func() ([]byte, error) {
		img := renderIdentityAvatar(identity)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("avatar: encode png: %w", err)
		}
		return buf.Bytes(), nil
	}
}

const avatarSize = 64

func renderIdentityAvatar(identity string) *image.RGBA {
	bg := paletteFor(identity)
	img := image.NewRGBA(image.Rect(0, 0, avatarSize, avatarSize))
	// Fill background.
	for y := range avatarSize {
		for x := range avatarSize {
			img.Set(x, y, bg)
		}
	}
	// Draw two initials as a centred 5×8 blocky monogram. We purposefully
	// keep the font embedded as hard-coded bitmaps: no external deps, and
	// only a tiny set of glyphs is needed.
	letters := initials(identity)
	const glyphW, glyphH = 5, 8
	const scale = 4 // 5×8 at 4× → 20×32, two letters → 40 wide → centred in 64.
	totalW := (glyphW*2 + 1) * scale
	startX := (avatarSize - totalW) / 2
	startY := (avatarSize - glyphH*scale) / 2
	fg := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	for i, r := range letters {
		bits := glyph(r)
		offX := startX + i*(glyphW+1)*scale
		for gy := range glyphH {
			for gx := range glyphW {
				if bits[gy]&(1<<(glyphW-1-gx)) == 0 {
					continue
				}
				for sy := range scale {
					for sx := range scale {
						img.Set(offX+gx*scale+sx, startY+gy*scale+sy, fg)
					}
				}
			}
		}
	}
	return img
}

// paletteFor picks a deterministic pleasant-ish colour from identity. It is
// deliberately high-saturation so initials in white remain legible.
func paletteFor(identity string) color.RGBA {
	if identity == "" {
		return color.RGBA{0x60, 0x70, 0x80, 0xFF}
	}
	h := sha1.Sum([]byte(identity))
	// Use the first byte as hue bucket; fix S and V in HSV space.
	hue := float64(h[0]) / 255.0 * 360.0
	return hsvToRGB(hue, 0.55, 0.72)
}

func hsvToRGB(h, s, v float64) color.RGBA {
	c := v * s
	x := c * (1 - absf(modf(h/60.0, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return color.RGBA{
		R: uint8((r + m) * 255),
		G: uint8((g + m) * 255),
		B: uint8((b + m) * 255),
		A: 0xFF,
	}
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func modf(a, b float64) float64 {
	r := a - float64(int(a/b))*b
	if r < 0 {
		r += b
	}
	return r
}

// initials extracts up to two initials from a signature of the form
// "User at Host (Platform)". Falls back to the first two letters of the
// whole string, or "??" if nothing usable is present.
func initials(sig string) [2]rune {
	var out [2]rune
	out[0], out[1] = '?', '?'
	if sig == "" {
		return out
	}
	name := sig
	if i := strings.Index(name, " at "); i > 0 {
		name = name[:i]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return out
	}
	parts := strings.Fields(name)
	switch {
	case len(parts) >= 2:
		out[0] = firstRune(parts[0])
		out[1] = firstRune(parts[1])
	case len(parts) == 1:
		rs := []rune(parts[0])
		out[0] = rs[0]
		if len(rs) > 1 {
			out[1] = rs[1]
		} else {
			out[1] = rs[0]
		}
	}
	return normalizeInitials(out)
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return '?'
}

func normalizeInitials(rs [2]rune) [2]rune {
	for i, r := range rs {
		if r >= 'a' && r <= 'z' {
			rs[i] = r - 32
		}
		if _, ok := font[rs[i]]; !ok {
			rs[i] = '?'
		}
	}
	return rs
}

// glyph returns the 8-row bitmap for r. Each row is 5 bits, MSB = leftmost.
func glyph(r rune) [8]uint8 {
	if g, ok := font[r]; ok {
		return g
	}
	return font['?']
}
