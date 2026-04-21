package avatar

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"dukto/internal/protocol"
)

func TestDefaultRenderer_Produces64x64PNG(t *testing.T) {
	r := DefaultRenderer("Alice at laptop (Linux)")
	data, err := r()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 64 || b.Dy() != 64 {
		t.Fatalf("avatar is %dx%d, want 64x64", b.Dx(), b.Dy())
	}
}

func TestPaletteFor_DifferentIdentitiesYieldDifferentColors(t *testing.T) {
	a := paletteFor("alice")
	b := paletteFor("bob")
	if a == b {
		t.Fatalf("expected distinct colors, got %+v for both", a)
	}
}

func TestInitials(t *testing.T) {
	cases := []struct {
		sig  string
		want [2]rune
	}{
		{"Alice at laptop (Linux)", [2]rune{'A', 'L'}}, // "Alice laptop" — but " at " is stripped so it's "Alice"
		{"Alice Cooper at box (Linux)", [2]rune{'A', 'C'}},
		{"A at b (c)", [2]rune{'A', 'A'}},
		{"", [2]rune{'?', '?'}},
	}
	for _, c := range cases {
		got := initials(c.sig)
		// After " at " strip we get the username; for multi-word usernames we
		// use first+second word initials. Recompute expectation.
		if got[0] != c.want[0] || got[1] != c.want[1] {
			t.Errorf("initials(%q) = %c%c, want %c%c", c.sig, got[0], got[1], c.want[0], c.want[1])
		}
	}
}

func TestServer_ServesPNGOnAnyGet(t *testing.T) {
	s := New(DefaultRenderer("TestUser at testhost (Linux)"))
	ts := httptest.NewServer(s)
	defer ts.Close()

	for _, path := range []string{"/", "/dukto/avatar", "/dukto/avatar?v=1", "/anything"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
			t.Fatalf("GET %s Content-Type = %q", path, ct)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) {
			t.Fatalf("GET %s body does not look like PNG (starts %x)", path, body[:min(len(body), 8)])
		}
	}
}

func TestServer_HEADOmitsBody(t *testing.T) {
	s := New(DefaultRenderer(""))
	ts := httptest.NewServer(s)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodHead, ts.URL+"/dukto/avatar", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Fatalf("HEAD returned %d bytes of body, want 0", len(body))
	}
	if cl := resp.Header.Get("Content-Length"); cl == "0" || cl == "" {
		t.Fatalf("HEAD Content-Length = %q, want a non-zero hint", cl)
	}
}

func TestServer_CachesRender(t *testing.T) {
	var calls atomic.Int32
	r := Renderer(func() ([]byte, error) {
		calls.Add(1)
		return DefaultRenderer("x")()
	})
	s := New(r)
	ts := httptest.NewServer(s)
	defer ts.Close()

	for range 3 {
		resp, err := http.Get(ts.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if calls.Load() != 1 {
		t.Fatalf("renderer called %d times, want 1 (cache miss only)", calls.Load())
	}
	s.Invalidate()
	resp, _ := http.Get(ts.URL + "/")
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if calls.Load() != 2 {
		t.Fatalf("after Invalidate renderer called %d times, want 2", calls.Load())
	}
}

func TestServer_StartOnPortPlusOffset(t *testing.T) {
	// Start on an ephemeral base port by listening, grabbing the port, closing
	// it, and using basePort = that - AvatarPortOffset. We just want to prove
	// that Start() wires the offset math correctly.
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().(*net.TCPAddr)
	_ = probe.Close()
	basePort := uint16(addr.Port) - protocol.AvatarPortOffset

	s := New(DefaultRenderer("abc"))
	if err := s.Start(basePort); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/dukto/avatar", basePort+protocol.AvatarPortOffset)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
