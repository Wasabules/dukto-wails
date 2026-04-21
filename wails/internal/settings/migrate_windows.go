//go:build windows

package settings

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// qtRegPath is where QSettings("msec.it", "Dukto") lands on Windows. Qt
// creates the subkey `R5` implicitly the first time "R5/ShowTermsOnStart"
// is written, so that key needs a second Open to reach.
const qtRegPath = `Software\msec.it\Dukto`

func loadQtValues() (QtValues, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, qtRegPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return QtValues{}, false, nil
		}
		return QtValues{}, false, err
	}
	defer k.Close()

	var qt QtValues
	found := false

	if v, _, err := k.GetStringValue("DestPath"); err == nil {
		qt.DestPath = v
		found = true
	}
	if v, _, err := k.GetStringValue("BuddyName"); err == nil {
		qt.BuddyName = v
		found = true
	}
	if v, _, err := k.GetStringValue("ThemeColor"); err == nil {
		qt.ThemeColor = v
		found = true
	}
	if b, ok := readRegBool(k, "AutoMode"); ok {
		qt.AutoMode, qt.HasAutoMode = b, true
		found = true
	}
	if b, ok := readRegBool(k, "DarkMode"); ok {
		qt.DarkMode, qt.HasDarkMode = b, true
		found = true
	}
	if b, ok := readRegBool(k, "Notification"); ok {
		qt.Notification, qt.HasNotification = b, true
		found = true
	}
	if b, ok := readRegBool(k, "CloseToTray"); ok {
		qt.CloseToTray, qt.HasCloseToTray = b, true
		found = true
	}
	if data, _, err := k.GetBinaryValue("WindowPosAndSize"); err == nil && len(data) > 0 {
		qt.WindowGeometry = data
		found = true
	}

	// R5/ShowTermsOnStart lives in the R5 subkey — Qt writes grouped keys as
	// nested registry keys rather than slash-delimited value names.
	if r5, err := registry.OpenKey(registry.CURRENT_USER, qtRegPath+`\R5`, registry.QUERY_VALUE); err == nil {
		if b, ok := readRegBool(r5, "ShowTermsOnStart"); ok {
			qt.ShowTermsOnStart, qt.HasShowTerms = b, true
			found = true
		}
		r5.Close()
	}

	return qt, found, nil
}

// readRegBool accepts either REG_DWORD (0/1) or REG_SZ ("true"/"false"),
// since Qt has shipped both depending on version.
func readRegBool(k registry.Key, name string) (bool, bool) {
	if n, _, err := k.GetIntegerValue(name); err == nil {
		return n != 0, true
	}
	if s, _, err := k.GetStringValue(name); err == nil {
		switch s {
		case "true", "1", "True", "TRUE":
			return true, true
		case "false", "0", "False", "FALSE":
			return false, true
		}
	}
	return false, false
}
