package protocol

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestWriteSessionHeader_LittleEndianLayout(t *testing.T) {
	// TotalElements = 2, TotalSize = 0x0102030405060708.
	// Expected:
	//   02 00 00 00 00 00 00 00   (u64 LE = 2)
	//   08 07 06 05 04 03 02 01   (i64 LE = 0x0102030405060708)
	var buf bytes.Buffer
	if err := WriteSessionHeader(&buf, SessionHeader{TotalElements: 2, TotalSize: 0x0102030405060708}); err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01,
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("got  % x\nwant % x", buf.Bytes(), want)
	}
}

func TestSessionHeader_RoundTrip(t *testing.T) {
	cases := []SessionHeader{
		{TotalElements: 1, TotalSize: 0},
		{TotalElements: 1, TotalSize: 5},
		{TotalElements: 42, TotalSize: 1 << 40},
	}
	for _, want := range cases {
		var buf bytes.Buffer
		if err := WriteSessionHeader(&buf, want); err != nil {
			t.Fatal(err)
		}
		got, err := ReadSessionHeader(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("round-trip\n got  %+v\n want %+v", got, want)
		}
	}
}

func TestReadSessionHeader_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
	}{
		{"zero elements", append(make([]byte, 8), 0, 0, 0, 0, 0, 0, 0, 0)},
		{"negative size", func() []byte {
			// TotalElements=1, TotalSize = -1 (all 0xFF in u64 LE).
			b := []byte{1, 0, 0, 0, 0, 0, 0, 0}
			b = append(b, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
			return b
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ReadSessionHeader(bytes.NewReader(c.buf)); !errors.Is(err, ErrInvalidStream) {
				t.Fatalf("expected ErrInvalidStream, got %v", err)
			}
		})
	}
}

func TestReadSessionHeader_ShortRead(t *testing.T) {
	_, err := ReadSessionHeader(bytes.NewReader([]byte{1, 2, 3}))
	if err == nil || errors.Is(err, ErrInvalidStream) {
		t.Fatalf("expected io error (not ErrInvalidStream), got %v", err)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestWriteElementHeader_FileLayout(t *testing.T) {
	// Name "a", Size 1. Expected: 'a' 0x00 [i64 LE = 1]
	var buf bytes.Buffer
	if err := WriteElementHeader(&buf, ElementHeader{Name: "a", Size: 1}); err != nil {
		t.Fatal(err)
	}
	want := []byte{'a', 0x00, 1, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("got  % x\nwant % x", buf.Bytes(), want)
	}
}

func TestWriteElementHeader_DirectorySentinel(t *testing.T) {
	// Directory ⇒ size -1 ⇒ i64 LE all 0xFF.
	var buf bytes.Buffer
	if err := WriteElementHeader(&buf, ElementHeader{Name: "d", Size: DirectorySizeMarker}); err != nil {
		t.Fatal(err)
	}
	want := []byte{'d', 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("got  % x\nwant % x", buf.Bytes(), want)
	}
}

func TestWriteElementHeader_TextElement(t *testing.T) {
	// The text-snippet element uses the magic name and the UTF-8 byte length
	// of the text as Size. This test locks down the magic name spelling.
	const sample = "hello"
	var buf bytes.Buffer
	if err := WriteElementHeader(&buf, ElementHeader{Name: TextElementName, Size: int64(len(sample))}); err != nil {
		t.Fatal(err)
	}
	want := append([]byte(TextElementName), 0x00)
	want = append(want, byte(len(sample)), 0, 0, 0, 0, 0, 0, 0)
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("got  % x\nwant % x", buf.Bytes(), want)
	}
}

func TestWriteElementHeader_EmptyNameRejected(t *testing.T) {
	var buf bytes.Buffer
	err := WriteElementHeader(&buf, ElementHeader{Name: "", Size: 0})
	if !errors.Is(err, ErrInvalidStream) {
		t.Fatalf("expected ErrInvalidStream, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("no bytes should be written on rejected write; got %d", buf.Len())
	}
}

func TestElementHeader_RoundTrip(t *testing.T) {
	cases := []ElementHeader{
		{Name: "file.txt", Size: 0},
		{Name: "file.txt", Size: 12345},
		{Name: "dir", Size: DirectorySizeMarker},
		{Name: "dir/nested/deep/file.bin", Size: 1 << 30},
		{Name: TextElementName, Size: 5},
		{Name: "émoji-🔥-CJK-你好", Size: 100},
	}
	for _, want := range cases {
		var buf bytes.Buffer
		if err := WriteElementHeader(&buf, want); err != nil {
			t.Fatal(err)
		}
		got, err := ReadElementHeader(bufio.NewReader(&buf))
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("round-trip\n got  %+v\n want %+v", got, want)
		}
	}
}

func TestReadElementHeader_RejectsInvalidSize(t *testing.T) {
	// Name "x\0" followed by i64 LE = -2 (invalid; only -1, 0, >0 allowed).
	buf := []byte{'x', 0x00, 0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	_, err := ReadElementHeader(bufio.NewReader(bytes.NewReader(buf)))
	if !errors.Is(err, ErrInvalidStream) {
		t.Fatalf("expected ErrInvalidStream, got %v", err)
	}
}

func TestReadElementHeader_RejectsEmptyName(t *testing.T) {
	// "\0" + 8 zero bytes ⇒ empty name.
	buf := append([]byte{0x00}, make([]byte, 8)...)
	_, err := ReadElementHeader(bufio.NewReader(bytes.NewReader(buf)))
	if !errors.Is(err, ErrInvalidStream) {
		t.Fatalf("expected ErrInvalidStream, got %v", err)
	}
}

func TestElementHeader_IsDirectoryIsText(t *testing.T) {
	if !(ElementHeader{Name: "d", Size: -1}).IsDirectory() {
		t.Error("IsDirectory should be true for size -1")
	}
	if (ElementHeader{Name: "d", Size: 0}).IsDirectory() {
		t.Error("IsDirectory should be false for size 0")
	}
	if !(ElementHeader{Name: TextElementName}).IsText() {
		t.Error("IsText should be true for the magic name")
	}
	if (ElementHeader{Name: "text.txt"}).IsText() {
		t.Error("IsText should be false for arbitrary filenames")
	}
}

// TestFullTransferRoundTrip constructs a full session — header + a directory +
// a nested file + a text snippet — and verifies it decodes back to the same
// structure. This is the closest thing to an end-to-end wire-format test
// without actual sockets, and corresponds to the scenarios called out in
// docs/PROTOCOL.md §7.
func TestFullTransferRoundTrip(t *testing.T) {
	fileData := []byte("file contents")
	textData := []byte("hello 🌍")

	var buf bytes.Buffer
	mustWriteSessionHeader(t, &buf, SessionHeader{
		TotalElements: 3,
		TotalSize:     int64(len(fileData) + len(textData)),
	})
	mustWriteElementHeader(t, &buf, ElementHeader{Name: "mydir", Size: DirectorySizeMarker})
	mustWriteElementHeader(t, &buf, ElementHeader{Name: "mydir/file.bin", Size: int64(len(fileData))})
	buf.Write(fileData)
	mustWriteElementHeader(t, &buf, ElementHeader{Name: TextElementName, Size: int64(len(textData))})
	buf.Write(textData)

	r := bufio.NewReader(&buf)
	hdr, err := ReadSessionHeader(r)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.TotalElements != 3 || hdr.TotalSize != int64(len(fileData)+len(textData)) {
		t.Fatalf("unexpected session header: %+v", hdr)
	}

	dir, err := ReadElementHeader(r)
	if err != nil {
		t.Fatal(err)
	}
	if dir.Name != "mydir" || !dir.IsDirectory() {
		t.Fatalf("expected mydir directory, got %+v", dir)
	}

	file, err := ReadElementHeader(r)
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != "mydir/file.bin" || file.Size != int64(len(fileData)) {
		t.Fatalf("unexpected file header: %+v", file)
	}
	gotFile := make([]byte, file.Size)
	if _, err := io.ReadFull(r, gotFile); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotFile, fileData) {
		t.Fatalf("file data mismatch\n got  %q\n want %q", gotFile, fileData)
	}

	text, err := ReadElementHeader(r)
	if err != nil {
		t.Fatal(err)
	}
	if !text.IsText() || text.Size != int64(len(textData)) {
		t.Fatalf("unexpected text header: %+v", text)
	}
	gotText := make([]byte, text.Size)
	if _, err := io.ReadFull(r, gotText); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotText, textData) {
		t.Fatalf("text mismatch\n got  %q\n want %q", gotText, textData)
	}
}

func mustWriteSessionHeader(t *testing.T, w io.Writer, h SessionHeader) {
	t.Helper()
	if err := WriteSessionHeader(w, h); err != nil {
		t.Fatal(err)
	}
}

func mustWriteElementHeader(t *testing.T, w io.Writer, h ElementHeader) {
	t.Helper()
	if err := WriteElementHeader(w, h); err != nil {
		t.Fatal(err)
	}
}
