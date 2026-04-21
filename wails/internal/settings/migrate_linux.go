//go:build linux

package settings

import (
	"bufio"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// qtConfRelPath is where QSettings("msec.it", "Dukto") lands on Linux. Qt
// writes an INI-style file under XDG_CONFIG_HOME (default ~/.config).
func qtConfPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "msec.it", "Dukto.conf"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "msec.it", "Dukto.conf"), nil
}

func loadQtValues() (QtValues, bool, error) {
	path, err := qtConfPath()
	if err != nil {
		return QtValues{}, false, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return QtValues{}, false, nil
		}
		return QtValues{}, false, err
	}
	defer f.Close()

	var qt QtValues
	found := false
	section := ""
	scanner := bufio.NewScanner(f)
	// Geometry blobs can be long; Qt base64-encodes them line by line.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"`)
		fullKey := key
		if section != "" && !strings.EqualFold(section, "General") {
			fullKey = section + "/" + key
		}
		if applyIniKey(&qt, fullKey, val) {
			found = true
		}
	}
	if err := scanner.Err(); err != nil {
		return qt, found, err
	}
	return qt, found, nil
}

func applyIniKey(qt *QtValues, key, val string) bool {
	switch key {
	case "DestPath":
		qt.DestPath = val
	case "BuddyName":
		qt.BuddyName = val
	case "ThemeColor":
		qt.ThemeColor = val
	case "AutoMode":
		qt.AutoMode, qt.HasAutoMode = parseIniBool(val)
	case "DarkMode":
		qt.DarkMode, qt.HasDarkMode = parseIniBool(val)
	case "Notification":
		qt.Notification, qt.HasNotification = parseIniBool(val)
	case "CloseToTray":
		qt.CloseToTray, qt.HasCloseToTray = parseIniBool(val)
	case "R5/ShowTermsOnStart":
		qt.ShowTermsOnStart, qt.HasShowTerms = parseIniBool(val)
	case "WindowPosAndSize":
		// Qt writes byte arrays as `@ByteArray(...)` on some versions, or as
		// base64 on others. Strip the wrapper if present, then try base64.
		v := val
		if strings.HasPrefix(v, "@ByteArray(") && strings.HasSuffix(v, ")") {
			v = v[len("@ByteArray(") : len(v)-1]
			qt.WindowGeometry = []byte(v)
			return true
		}
		if b, err := base64.StdEncoding.DecodeString(v); err == nil {
			qt.WindowGeometry = b
		}
	default:
		return false
	}
	return true
}

func parseIniBool(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}
