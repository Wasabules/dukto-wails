package transfer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"dukto/internal/protocol"
)

func TestWriter_EmitsDeclaredElementsThenDone(t *testing.T) {
	var buf bytes.Buffer
	hdr := protocol.SessionHeader{TotalElements: 3, TotalSize: 5}
	w, err := NewWriter(&buf, hdr)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteDir("d"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("d/a.txt", 5, strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("d/empty", 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Done(); err != nil {
		t.Fatal(err)
	}

	// Now read it back and verify structure.
	sr, err := NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if sr.Header != hdr {
		t.Fatalf("header roundtrip: got %+v want %+v", sr.Header, hdr)
	}
	want := []struct {
		name string
		size int64
		body string
	}{
		{"d", protocol.DirectorySizeMarker, ""},
		{"d/a.txt", 5, "hello"},
		{"d/empty", 0, ""},
	}
	for i, w := range want {
		el, err := sr.Next()
		if err != nil {
			t.Fatalf("Next[%d]: %v", i, err)
		}
		if el.Header.Name != w.name || el.Header.Size != w.size {
			t.Fatalf("element[%d] = %+v, want name=%q size=%d", i, el.Header, w.name, w.size)
		}
		if el.Data == nil {
			if w.body != "" {
				t.Fatalf("element[%d] has nil Data but expected body %q", i, w.body)
			}
			continue
		}
		body, err := io.ReadAll(el.Data)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != w.body {
			t.Fatalf("element[%d] body = %q, want %q", i, body, w.body)
		}
	}
	if _, err := sr.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("after last element, Next = %v, want io.EOF", err)
	}
}

func TestWriter_RejectsOverrun(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, protocol.SessionHeader{TotalElements: 1, TotalSize: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteDir("a"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteDir("b"); !errors.Is(err, ErrStreamOverrun) {
		t.Fatalf("expected ErrStreamOverrun, got %v", err)
	}
}

func TestWriter_DoneDetectsShortSession(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, protocol.SessionHeader{TotalElements: 2, TotalSize: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteDir("a"); err != nil {
		t.Fatal(err)
	}
	if err := w.Done(); err == nil {
		t.Fatal("Done should have errored on short session")
	}
}

func TestReader_AutoDrainsSkippedElement(t *testing.T) {
	// Write one file and one dir; read without consuming the file body, then
	// verify the dir is still decoded correctly.
	var buf bytes.Buffer
	w, err := NewWriter(&buf, protocol.SessionHeader{TotalElements: 2, TotalSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("f.bin", 5, strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteDir("d"); err != nil {
		t.Fatal(err)
	}
	if err := w.Done(); err != nil {
		t.Fatal(err)
	}

	sr, _ := NewReader(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if _, err := sr.Next(); err != nil {
		t.Fatal(err)
	}
	// Skip reading the file body.
	el, err := sr.Next()
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if el.Header.Name != "d" || !el.Header.IsDirectory() {
		t.Fatalf("expected dir, got %+v", el.Header)
	}
}

func TestSources_FlattensDirectoryDepthFirst(t *testing.T) {
	root := t.TempDir()
	pick := filepath.Join(root, "top")
	mustMkdir(t, pick)
	mustMkdir(t, filepath.Join(pick, "sub"))
	mustWriteFile(t, filepath.Join(pick, "a.txt"), "aaa")
	mustWriteFile(t, filepath.Join(pick, "sub", "b.txt"), "bbbb")
	// Empty directory should be emitted with size -1.
	mustMkdir(t, filepath.Join(pick, "empty_dir"))

	srcs, hdr, err := Sources([]string{pick})
	if err != nil {
		t.Fatal(err)
	}
	var gotNames []string
	for _, s := range srcs {
		gotNames = append(gotNames, s.Name)
	}
	wantOrder := []string{"top", "top/a.txt", "top/empty_dir", "top/sub", "top/sub/b.txt"}
	if strings.Join(gotNames, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("order mismatch:\n got  %v\n want %v", gotNames, wantOrder)
	}
	if hdr.TotalElements != 5 || hdr.TotalSize != 7 {
		t.Fatalf("header = %+v, want elements=5 size=7", hdr)
	}
	for i, want := range []struct {
		name string
		size int64
	}{
		{"top", protocol.DirectorySizeMarker},
		{"top/a.txt", 3},
		{"top/empty_dir", protocol.DirectorySizeMarker},
		{"top/sub", protocol.DirectorySizeMarker},
		{"top/sub/b.txt", 4},
	} {
		if srcs[i].Name != want.name || srcs[i].Size != want.size {
			t.Fatalf("src[%d] = %+v, want name=%q size=%d", i, srcs[i], want.name, want.size)
		}
	}
}

func TestSources_MultipleTopLevelInputs(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.bin")
	mustWriteFile(t, a, "aa")
	mustWriteFile(t, b, "bbb")
	srcs, hdr, err := Sources([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 2 || hdr.TotalElements != 2 || hdr.TotalSize != 5 {
		t.Fatalf("unexpected sources/header: %+v / %+v", srcs, hdr)
	}
	if srcs[0].Name != "a.txt" || srcs[1].Name != "b.bin" {
		t.Fatalf("names = [%s %s]", srcs[0].Name, srcs[1].Name)
	}
}

func TestSend_EndToEnd_OverBytesBuffer(t *testing.T) {
	// Build sources from disk, Send to a buffer, then Receive from that
	// buffer into a fresh destination directory. Verify the file content
	// ends up bit-identical.
	src := t.TempDir()
	mustMkdir(t, filepath.Join(src, "mydir"))
	mustWriteFile(t, filepath.Join(src, "mydir", "hello.txt"), "hello world")

	srcs, hdr, err := Sources([]string{filepath.Join(src, "mydir")})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Send(&buf, srcs, hdr); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	var files []string
	rc := Receiver{
		Dest: dst,
		OnEvent: func(ev ReceiveEvent) error {
			if ev.Kind == EventFileReceived {
				files = append(files, ev.LocalPath)
			}
			return nil
		},
	}
	if err := rc.Handle(context.Background(), bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %v", files)
	}
	got, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Fatalf("file body = %q", got)
	}
}

func TestReceiver_RenamesTopLevelOnCollision(t *testing.T) {
	dst := t.TempDir()
	// Pre-create "foo" as an existing directory.
	mustMkdir(t, filepath.Join(dst, "foo"))

	var buf bytes.Buffer
	sw, _ := NewWriter(&buf, protocol.SessionHeader{TotalElements: 2, TotalSize: 3})
	_ = sw.WriteDir("foo")
	_ = sw.WriteFile("foo/a.txt", 3, strings.NewReader("abc"))
	_ = sw.Done()

	var created string
	rc := Receiver{Dest: dst, OnEvent: func(ev ReceiveEvent) error {
		if ev.Kind == EventFileReceived {
			created = ev.LocalPath
		}
		return nil
	}}
	if err := rc.Handle(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dst, "foo (2)", "a.txt"); created != want {
		t.Fatalf("file created at %q, want %q", created, want)
	}
}

func TestReceiver_RenamesTopLevelFileOnCollision(t *testing.T) {
	dst := t.TempDir()
	mustWriteFile(t, filepath.Join(dst, "note.txt"), "old")

	var buf bytes.Buffer
	sw, _ := NewWriter(&buf, protocol.SessionHeader{TotalElements: 1, TotalSize: 3})
	_ = sw.WriteFile("note.txt", 3, strings.NewReader("new"))
	_ = sw.Done()

	var created string
	rc := Receiver{Dest: dst, OnEvent: func(ev ReceiveEvent) error {
		if ev.Kind == EventFileReceived {
			created = ev.LocalPath
		}
		return nil
	}}
	if err := rc.Handle(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dst, "note (2).txt"); created != want {
		t.Fatalf("file created at %q, want %q", created, want)
	}
	// Old file untouched.
	if b, _ := os.ReadFile(filepath.Join(dst, "note.txt")); string(b) != "old" {
		t.Fatalf("original file was overwritten: %q", b)
	}
}

func TestReceiver_RejectsPathTraversal(t *testing.T) {
	dst := t.TempDir()
	var buf bytes.Buffer
	sw, _ := NewWriter(&buf, protocol.SessionHeader{TotalElements: 1, TotalSize: 3})
	_ = sw.WriteFile("../evil.txt", 3, strings.NewReader("xxx"))
	_ = sw.Done()

	rc := Receiver{Dest: dst}
	err := rc.Handle(context.Background(), &buf)
	if err == nil {
		t.Fatal("expected traversal rejection")
	}
	// Verify nothing was written outside dst.
	if _, err := os.Stat(filepath.Join(filepath.Dir(dst), "evil.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("evil.txt was written outside dest")
	}
}

func TestReceiver_TextSnippet(t *testing.T) {
	dst := t.TempDir()
	var buf bytes.Buffer
	sw, _ := NewWriter(&buf, protocol.SessionHeader{TotalElements: 1, TotalSize: 5})
	_ = sw.WriteText("hello")
	_ = sw.Done()

	var text string
	rc := Receiver{Dest: dst, OnEvent: func(ev ReceiveEvent) error {
		if ev.Kind == EventTextReceived {
			text = ev.Text
		}
		return nil
	}}
	if err := rc.Handle(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if text != "hello" {
		t.Fatalf("text = %q", text)
	}
	// No files should have been created.
	entries, _ := os.ReadDir(dst)
	if len(entries) != 0 {
		t.Fatalf("text session should not create files, got %v", entries)
	}
}

func TestDial_And_Receive_OverRealSockets(t *testing.T) {
	// Smoke test that the TCP-level Dial and the Receiver actually compose
	// over a real loopback socket. Uses a tiny session so the test is fast.
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	dst := t.TempDir()
	var wg sync.WaitGroup
	var received string
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.Close()
		rc := Receiver{Dest: dst, OnEvent: func(ev ReceiveEvent) error {
			if ev.Kind == EventTextReceived {
				received = ev.Text
			}
			return nil
		}}
		if err := rc.Handle(context.Background(), conn); err != nil {
			t.Errorf("handle: %v", err)
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	peer, _ := net.ResolveTCPAddr("tcp4", addr.String())
	// Build netip.AddrPort from the listener address.
	tcpAddr := net.TCPAddrFromAddrPort(peer.AddrPort())
	_ = tcpAddr
	if err := SendText(context.Background(), peer.AddrPort(), "ping"); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if received != "ping" {
		t.Fatalf("received = %q", received)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
