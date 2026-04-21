//go:build darwin

package settings

import (
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// qtPlistPath returns the plist location QSettings("msec.it", "Dukto") uses.
// Qt rearranges the organisation name into reverse-DNS form (msec.it →
// it.msec) and appends the app name.
func qtPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Preferences", "it.msec.Dukto.plist"), nil
}

func loadQtValues() (QtValues, bool, error) {
	path, err := qtPlistPath()
	if err != nil {
		return QtValues{}, false, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return QtValues{}, false, nil
		}
		return QtValues{}, false, err
	}

	// Use plutil to convert the (often binary) plist to XML on stdout. plutil
	// ships with macOS so this has no external dep.
	out, err := exec.Command("plutil", "-convert", "xml1", "-o", "-", path).Output()
	if err != nil {
		return QtValues{}, false, err
	}
	return parsePlist(out)
}

// parsePlist walks the ordered child list of the top-level <dict> and pairs
// each <key> with the following value element, matching plist semantics.
func parsePlist(data []byte) (QtValues, bool, error) {
	type rawItem struct {
		XMLName xml.Name
		Value   string `xml:",chardata"`
	}
	type rawDict struct {
		XMLName xml.Name  `xml:"dict"`
		Items   []rawItem `xml:",any"`
	}
	type rawPlist struct {
		XMLName xml.Name `xml:"plist"`
		Dict    rawDict  `xml:"dict"`
	}

	var p rawPlist
	if err := xml.Unmarshal(data, &p); err != nil {
		return QtValues{}, false, err
	}

	var qt QtValues
	found := false
	var pendingKey string
	havePending := false
	for _, it := range p.Dict.Items {
		tag := it.XMLName.Local
		if tag == "key" {
			pendingKey = it.Value
			havePending = true
			continue
		}
		if !havePending {
			continue
		}
		havePending = false
		key := pendingKey
		switch tag {
		case "string":
			if applyPlistString(&qt, key, it.Value) {
				found = true
			}
		case "true", "false":
			b := tag == "true"
			if applyPlistBool(&qt, key, b) {
				found = true
			}
		case "integer":
			b := strings.TrimSpace(it.Value) != "0"
			if applyPlistBool(&qt, key, b) {
				found = true
			}
		case "data":
			// plist <data> is base64-encoded, possibly with whitespace. We hand
			// it to the Linux-style parser as a fallback — but simplest is to
			// just record the raw base64 and decode downstream if needed.
			raw := strings.Map(func(r rune) rune {
				if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
					return -1
				}
				return r
			}, it.Value)
			if key == "WindowPosAndSize" && raw != "" {
				qt.WindowGeometry = []byte(raw)
				found = true
			}
		}
	}
	return qt, found, nil
}

func applyPlistString(qt *QtValues, key, val string) bool {
	switch key {
	case "DestPath":
		qt.DestPath = val
	case "BuddyName":
		qt.BuddyName = val
	case "ThemeColor":
		qt.ThemeColor = val
	default:
		return false
	}
	return true
}

func applyPlistBool(qt *QtValues, key string, val bool) bool {
	switch key {
	case "AutoMode":
		qt.AutoMode, qt.HasAutoMode = val, true
	case "DarkMode":
		qt.DarkMode, qt.HasDarkMode = val, true
	case "Notification":
		qt.Notification, qt.HasNotification = val, true
	case "CloseToTray":
		qt.CloseToTray, qt.HasCloseToTray = val, true
	case "R5/ShowTermsOnStart", "R5.ShowTermsOnStart":
		qt.ShowTermsOnStart, qt.HasShowTerms = val, true
	default:
		return false
	}
	return true
}
