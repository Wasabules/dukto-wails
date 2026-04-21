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
	// EventElementRejected fires when a single element is refused by a
	// defense-in-depth check (bad wire name, bad extension, depth cap,
	// per-session file cap). Name is the offending element name; Text
	// carries a short human reason. The session as a whole still aborts,
	// so callers typically surface this as a toast alongside the
	// subsequent receive:error.
	EventElementRejected
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
	// RemoteAddr is the "ip:port" of the peer that opened this session. Set
	// by Handle when the io.Reader is a net.Conn; empty otherwise. The UI
	// uses it to thread received items by sender.
	RemoteAddr string
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
//
// OnProgress, if set, is called with cumulative bytes received during the
// session. It fires both mid-file (for sub-file granularity on large files)
// and at element boundaries. The final call of the session reports
// (TotalSize, TotalSize).
type Receiver struct {
	Dest       string
	OnEvent    Handler
	OnProgress ProgressFunc

	// RejectExtensions is a set of lowercase extensions (without dot) that,
	// if matched against an incoming file's name, cause Handle to abort with
	// a "rejected" error *before* any bytes hit disk. Empty means accept all.
	// Applied per-element, so a mixed session with one rejected file tears
	// down the whole receive — intentional, to match "I don't trust this
	// sender" semantics.
	RejectExtensions map[string]struct{}

	// MaxSessionBytes, if > 0, causes Handle to refuse any session whose
	// SessionHeader.TotalSize exceeds the limit. Evaluated before any element
	// is read. 0 disables the cap.
	MaxSessionBytes int64

	// MaxFilesPerSession, if > 0, caps the number of non-text elements in
	// one session. Trips EventElementRejected + ErrTooManyFiles on the
	// first element past the cap.
	MaxFilesPerSession int

	// MaxPathDepth, if > 0, caps '/' segments in any element name. Trips
	// EventElementRejected + ErrPathTooDeep.
	MaxPathDepth int

	// AllowSession, if set, is consulted after the session header is parsed
	// and before the first element is read. Returning a non-nil error aborts
	// the session cleanly. Used by the host to apply dynamic policy (e.g.
	// disk-free guard) that needs the total bytes advertised by the sender.
	AllowSession func(protocol.SessionHeader) error

	// bytesRead is the running byte counter, shared across all counting
	// readers built during a single Handle() call. Reset per session in Handle.
	bytesRead int64
	// remoteAddr is captured from the net.Conn at the start of Handle and
	// stamped onto every ReceiveEvent. Empty for non-conn readers (tests).
	remoteAddr string
}

// ErrSessionTooLarge is returned by Handle when a session's total size would
// exceed the configured MaxSessionBytes. ErrRejectedExtension fires when a
// file element's extension is in RejectExtensions. Both are exported so the
// UI can distinguish policy rejections from real IO failures.
var (
	ErrSessionTooLarge    = errors.New("transfer: session exceeds size cap")
	ErrRejectedExtension  = errors.New("transfer: rejected by extension policy")
	ErrTooManyFiles       = errors.New("transfer: session exceeds file-count cap")
	ErrPathTooDeep        = errors.New("transfer: element path too deep")
	ErrInvalidName        = errors.New("transfer: element name failed validation")
)

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

	if conn, ok := r.(net.Conn); ok {
		if ra := conn.RemoteAddr(); ra != nil {
			rc.remoteAddr = ra.String()
		}
		if ctx != nil {
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
	}

	sr, err := NewReader(r)
	if err != nil {
		return fmt.Errorf("transfer: read session header: %w", err)
	}
	if rc.MaxSessionBytes > 0 && sr.Header.TotalSize > rc.MaxSessionBytes {
		return fmt.Errorf("%w: %d > %d", ErrSessionTooLarge, sr.Header.TotalSize, rc.MaxSessionBytes)
	}
	if rc.AllowSession != nil {
		if err := rc.AllowSession(sr.Header); err != nil {
			return err
		}
	}
	rc.bytesRead = 0
	if err := rc.emit(ReceiveEvent{Kind: EventSessionStart, Header: sr.Header}); err != nil {
		return err
	}

	topRenames := map[string]string{}
	stride := progressStride(sr.Header.TotalSize)
	var fileCount int

	for {
		el, err := sr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("transfer: read element: %w", err)
		}
		if err := rc.processElement(destAbs, el, topRenames, sr.Header.TotalSize, stride, &fileCount); err != nil {
			return err
		}
	}

	// Terminal progress event so the UI bar hits 100% even if the last file's
	// io.Copy didn't naturally land on a stride boundary.
	if rc.OnProgress != nil && sr.Header.TotalSize > 0 {
		rc.OnProgress(rc.bytesRead, sr.Header.TotalSize)
	}

	return rc.emit(ReceiveEvent{Kind: EventSessionComplete, Header: sr.Header})
}

func (rc *Receiver) processElement(destAbs string, el Element, topRenames map[string]string, total, stride int64, fileCount *int) error {
	// Text snippets bypass the filesystem entirely.
	if el.IsText() {
		var sb strings.Builder
		if el.Data != nil {
			src := rc.wrapProgress(el.Data, total, stride)
			if _, err := io.Copy(&sb, src); err != nil {
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
		rc.emitReject(el.Header.Name, "invalid-name", err.Error())
		return fmt.Errorf("%w: %q: %v", ErrInvalidName, el.Header.Name, err)
	}
	if rc.MaxPathDepth > 0 {
		depth := strings.Count(el.Header.Name, "/") + 1
		if depth > rc.MaxPathDepth {
			rc.emitReject(el.Header.Name, "path-too-deep", fmt.Sprintf("depth %d exceeds cap %d", depth, rc.MaxPathDepth))
			return fmt.Errorf("%w: %d > %d", ErrPathTooDeep, depth, rc.MaxPathDepth)
		}
	}
	if !el.IsDirectory() && len(rc.RejectExtensions) > 0 {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(el.Header.Name), "."))
		if _, blocked := rc.RejectExtensions[ext]; blocked {
			rc.emitReject(el.Header.Name, "extension", ext)
			return fmt.Errorf("%w: %s", ErrRejectedExtension, ext)
		}
	}
	if !el.IsDirectory() && rc.MaxFilesPerSession > 0 {
		*fileCount++
		if *fileCount > rc.MaxFilesPerSession {
			rc.emitReject(el.Header.Name, "too-many-files", fmt.Sprintf("%d > %d", *fileCount, rc.MaxFilesPerSession))
			return fmt.Errorf("%w: %d > %d", ErrTooManyFiles, *fileCount, rc.MaxFilesPerSession)
		}
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
		src := rc.wrapProgress(el.Data, total, stride)
		if _, err := io.Copy(f, src); err != nil {
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

// emitReject is a convenience helper so the four defense-in-depth checks
// above don't each need to build the event boilerplate.
func (rc *Receiver) emitReject(name, reason, detail string) {
	if rc.OnEvent == nil {
		return
	}
	_ = rc.emit(ReceiveEvent{
		Kind: EventElementRejected,
		Name: name,
		Text: reason + ": " + detail,
	})
}

func (rc *Receiver) emit(ev ReceiveEvent) error {
	if rc.OnEvent == nil {
		return nil
	}
	if ev.RemoteAddr == "" {
		ev.RemoteAddr = rc.remoteAddr
	}
	return rc.OnEvent(ev)
}

// wrapProgress returns r, possibly wrapped in a counter, so callers don't
// branch on OnProgress at every io.Copy site. A nil OnProgress falls through
// without any allocation.
func (rc *Receiver) wrapProgress(r io.Reader, total, stride int64) io.Reader {
	if rc.OnProgress == nil {
		return r
	}
	return &countingReader{
		r:       r,
		counter: &rc.bytesRead,
		total:   total,
		cb:      rc.OnProgress,
		stride:  stride,
	}
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
