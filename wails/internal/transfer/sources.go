package transfer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"dukto/internal/protocol"
)

// Source is one element to be streamed in a session. For directories Size is
// protocol.DirectorySizeMarker and LocalPath may be empty. For files Size is
// the byte count and LocalPath is the absolute on-disk path to open. For text
// snippets LocalPath is empty and Text carries the UTF-8 payload.
type Source struct {
	Name      string
	Size      int64
	LocalPath string
	Text      string
}

// IsDirectory reports whether s is a directory element.
func (s Source) IsDirectory() bool { return s.Size == protocol.DirectorySizeMarker }

// IsText reports whether s is the text-snippet element.
func (s Source) IsText() bool { return s.Name == protocol.TextElementName }

// TextSource builds the single-element source list that represents a text
// snippet. Per the protocol, a text session must contain exactly this one
// element; callers should not mix it with filesystem sources.
func TextSource(text string) ([]Source, protocol.SessionHeader) {
	size := int64(len(text))
	return []Source{{
			Name: protocol.TextElementName,
			Size: size,
			Text: text,
		}},
		protocol.SessionHeader{TotalElements: 1, TotalSize: size}
}

// Sources walks the given local paths and returns the full element list plus
// the session header.
//
// Ordering and naming mirror the Qt reference (network/filedata.cpp):
//   - the top-level name is filepath.Base of each input path
//   - directories are emitted before their contents
//   - child names are "<top>/<rel>" with '/' as the separator regardless of OS
//   - walking is lexicographic (Go's filepath.WalkDir default)
//
// Directories contribute 0 to TotalSize; files contribute their byte count.
// Symlinks are followed via os.Stat on entry; a broken symlink surfaces as an
// error rather than a silent skip, matching Qt's "can not read" behavior.
func Sources(paths []string) ([]Source, protocol.SessionHeader, error) {
	var out []Source
	var total int64
	for _, raw := range paths {
		clean := filepath.Clean(raw)
		if clean == "." || clean == "" {
			return nil, protocol.SessionHeader{}, fmt.Errorf("transfer: empty path")
		}
		fi, err := os.Stat(clean)
		if err != nil {
			return nil, protocol.SessionHeader{}, fmt.Errorf("transfer: stat %q: %w", clean, err)
		}
		top := filepath.Base(clean)
		if fi.IsDir() {
			err := filepath.WalkDir(clean, func(cur string, d fs.DirEntry, werr error) error {
				if werr != nil {
					return werr
				}
				rel, err := filepath.Rel(clean, cur)
				if err != nil {
					return err
				}
				var wireName string
				if rel == "." {
					wireName = top
				} else {
					wireName = top + "/" + filepath.ToSlash(rel)
				}
				if d.IsDir() {
					out = append(out, Source{
						Name:      wireName,
						Size:      protocol.DirectorySizeMarker,
						LocalPath: cur,
					})
					return nil
				}
				info, err := d.Info()
				if err != nil {
					return err
				}
				// Follow symlinks explicitly: WalkDir gives us the link's
				// metadata, not the target's.
				if info.Mode()&os.ModeSymlink != 0 {
					target, err := os.Stat(cur)
					if err != nil {
						return fmt.Errorf("transfer: resolve symlink %q: %w", cur, err)
					}
					if target.IsDir() {
						// A symlinked directory would require recursing into
						// the target under wireName. The Qt reference does
						// not special-case this, so we follow the same rule:
						// don't recurse, treat as empty.
						out = append(out, Source{
							Name:      wireName,
							Size:      protocol.DirectorySizeMarker,
							LocalPath: cur,
						})
						return nil
					}
					info = target
				}
				size := info.Size()
				total += size
				out = append(out, Source{
					Name:      wireName,
					Size:      size,
					LocalPath: cur,
				})
				return nil
			})
			if err != nil {
				return nil, protocol.SessionHeader{}, err
			}
			continue
		}
		size := fi.Size()
		total += size
		out = append(out, Source{
			Name:      top,
			Size:      size,
			LocalPath: clean,
		})
	}
	if len(out) == 0 {
		return nil, protocol.SessionHeader{}, fmt.Errorf("transfer: no elements to send")
	}
	return out, protocol.SessionHeader{
		TotalElements: uint64(len(out)),
		TotalSize:     total,
	}, nil
}

// validateWireName is a defense-in-depth check for names that come off the
// wire and might be crafted to escape the destination directory.
func validateWireName(name string) error {
	if name == "" {
		return fmt.Errorf("empty name")
	}
	// The protocol uses forward slashes. Backslashes, embedded NULs, and
	// Windows-style drive letters are not part of the wire format and most
	// likely indicate either a bug on the sender or a path-traversal attempt.
	if strings.ContainsAny(name, "\x00\\") {
		return fmt.Errorf("name contains disallowed characters")
	}
	parts := strings.Split(name, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return fmt.Errorf("name contains empty or dot segment")
		}
	}
	// Absolute paths (leading slash) would have produced an empty first part
	// above and been rejected already, but keep this explicit for clarity.
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("absolute path")
	}
	return nil
}
