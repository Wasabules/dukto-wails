package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"dukto/internal/protocol"
)

// ReceiveEventKind tags a ReceiveEvent.
type ReceiveEventKind int

const (
	// EventSessionStart fires once per incoming session, right after the
	// session header is parsed. Header is populated; Size, LocalPath, Name
	// are zero-valued.
	EventSessionStart ReceiveEventKind = iota
	// EventDirectoryCreated fires after a directory has been mkdir'd.
	EventDirectoryCreated
	// EventFileReceived fires after a file has been fully written and closed.
	EventFileReceived
	// EventTextReceived fires when the magic text element arrives. Text is
	// populated; LocalPath is empty.
	EventTextReceived
	// EventSessionComplete fires once after the last element has been
	// processed without error.
	EventSessionComplete
)

// ReceiveEvent is delivered to the Receiver's Handler callback. It carries
// enough information for UI layers to update progress bars and open the
// received file in the shell without knowing the wire format.
type ReceiveEvent struct {
	Kind      ReceiveEventKind
	Header    protocol.SessionHeader
	Name      string
	Size      int64
	LocalPath string
	Text      string
}

// Handler is the Receiver's event sink. It is invoked synchronously from the
// same goroutine that reads the socket, so slow handlers backpressure the
// receive. A Handler may return an error to abort the session cleanly; the
// Receiver will close the connection after the current element.
type Handler func(ReceiveEvent) error

// Receiver processes one inbound TCP session at a time.
//
// Dest is the destination directory; it must exist. Filesystem collisions on
// top-level names are resolved by appending " (2)", " (3)", …  The mapping is
// remembered for the duration of a single session so that "foo/bar.txt" lands
// under "foo (2)/bar.txt" when "foo" was renamed.
//
// OnEvent may be nil, in which case no events are delivered.
type Receiver struct {
	Dest    string
	OnEvent Handler
}

// Handle reads one session from r and writes it to the filesystem.
//
// r should be a net.Conn or equivalent. Cancelling ctx closes r (as long as r
// is a net.Conn) and causes Handle to return ctx.Err().
func (rc *Receiver) Handle(ctx context.Context, r io.Reader) error {
	if rc.Dest == "" {
		return errors.New("transfer: receiver dest is empty")
	}
	destAbs, err := filepath.Abs(rc.Dest)
	if err != nil {
		return fmt.Errorf("transfer: resolve dest: %w", err)
	}
	if info, err := os.Stat(destAbs); err != nil || !info.IsDir() {
		return fmt.Errorf("transfer: dest %q is not a directory", rc.Dest)
	}

	if conn, ok := r.(net.Conn); ok && ctx != nil {
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-done:
			}
		}()
	}

	sr, err := NewReader(r)
	if err != nil {
		return fmt.Errorf("transfer: read session header: %w", err)
	}
	if err := rc.emit(ReceiveEvent{Kind: EventSessionStart, Header: sr.Header}); err != nil {
		return err
	}

	topRenames := map[string]string{}

	for {
		el, err := sr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("transfer: read element: %w", err)
		}
		if err := rc.processElement(destAbs, el, topRenames); err != nil {
			return err
		}
	}

	return rc.emit(ReceiveEvent{Kind: EventSessionComplete, Header: sr.Header})
}

func (rc *Receiver) processElement(destAbs string, el Element, topRenames map[string]string) error {
	// Text snippets bypass the filesystem entirely.
	if el.IsText() {
		var sb strings.Builder
		if el.Data != nil {
			if _, err := io.Copy(&sb, el.Data); err != nil {
				return fmt.Errorf("transfer: read text body: %w", err)
			}
		}
		return rc.emit(ReceiveEvent{
			Kind: EventTextReceived,
			Name: el.Header.Name,
			Size: el.Header.Size,
			Text: sb.String(),
		})
	}

	if err := validateWireName(el.Header.Name); err != nil {
		return fmt.Errorf("transfer: reject element name %q: %w", el.Header.Name, err)
	}

	effectiveName := rewriteTopLevel(el.Header.Name, topRenames)
	fullPath := filepath.Join(destAbs, filepath.FromSlash(effectiveName))
	if !isUnder(destAbs, fullPath) {
		return fmt.Errorf("transfer: element %q escapes dest", el.Header.Name)
	}

	top, _, hasRest := strings.Cut(el.Header.Name, "/")

	if el.IsDirectory() {
		// Only the top-level of a selection gets collision-renamed.
		if !hasRest {
			renamed, err := uniqueDir(fullPath)
			if err != nil {
				return fmt.Errorf("transfer: mkdir %q: %w", fullPath, err)
			}
			topRenames[top] = filepath.Base(renamed)
			return rc.emit(ReceiveEvent{
				Kind:      EventDirectoryCreated,
				Name:      el.Header.Name,
				Size:      el.Header.Size,
				LocalPath: renamed,
			})
		}
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			return fmt.Errorf("transfer: mkdir %q: %w", fullPath, err)
		}
		return rc.emit(ReceiveEvent{
			Kind:      EventDirectoryCreated,
			Name:      el.Header.Name,
			Size:      el.Header.Size,
			LocalPath: fullPath,
		})
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("transfer: mkdir parent of %q: %w", fullPath, err)
	}

	finalPath := fullPath
	// Top-level file (no '/' in name) may collide with an existing file; apply
	// the same " (2)" scheme Qt uses.
	if !hasRest {
		var err error
		finalPath, err = uniqueFile(fullPath)
		if err != nil {
			return fmt.Errorf("transfer: pick unique name for %q: %w", fullPath, err)
		}
	}

	f, err := os.Create(finalPath)
	if err != nil {
		return fmt.Errorf("transfer: create %q: %w", finalPath, err)
	}
	if el.Data != nil && el.Header.Size > 0 {
		if _, err := io.Copy(f, el.Data); err != nil {
			f.Close()
			_ = os.Remove(finalPath)
			return fmt.Errorf("transfer: write %q: %w", finalPath, err)
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("transfer: close %q: %w", finalPath, err)
	}
	return rc.emit(ReceiveEvent{
		Kind:      EventFileReceived,
		Name:      el.Header.Name,
		Size:      el.Header.Size,
		LocalPath: finalPath,
	})
}

func (rc *Receiver) emit(ev ReceiveEvent) error {
	if rc.OnEvent == nil {
		return nil
	}
	return rc.OnEvent(ev)
}

// rewriteTopLevel swaps the first path segment with its session-specific
// renamed version if one was recorded.
func rewriteTopLevel(name string, renames map[string]string) string {
	if len(renames) == 0 {
		return name
	}
	top, rest, hasRest := strings.Cut(name, "/")
	renamed, ok := renames[top]
	if !ok {
		return name
	}
	if !hasRest {
		return renamed
	}
	return renamed + "/" + rest
}

// uniqueDir picks a directory path that does not yet exist, appending " (N)"
// for N = 2, 3, …, and creates it. Returns the path that was actually made.
func uniqueDir(path string) (string, error) {
	candidate := path
	for n := 2; ; n++ {
		err := os.Mkdir(candidate, 0o755)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		candidate = fmt.Sprintf("%s (%d)", path, n)
		if n > 1_000_000 {
			return "", fmt.Errorf("too many name collisions for %q", path)
		}
	}
}

// uniqueFile returns a file path that does not yet exist. Unlike uniqueDir it
// does not create the file; the caller is expected to os.Create the returned
// path, which races with other writers but matches Qt behavior (prepareFilesystem).
func uniqueFile(path string) (string, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return path, nil
	} else if err != nil {
		return "", err
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for n := 2; n < 1_000_000; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("too many name collisions for %q", path)
}

// isUnder reports whether target is inside (or equal to) root after cleaning.
// Used as a defense-in-depth guard against wire names crafted to escape the
// destination directory.
func isUnder(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	rel = filepath.ToSlash(rel)
	return !strings.HasPrefix(rel, "../") && rel != ".."
}
