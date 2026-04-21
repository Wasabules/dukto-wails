//go:build !windows && !linux && !darwin

package settings

// loadQtValues is a no-op on platforms that never ran the Qt build — there's
// nothing to migrate from.
func loadQtValues() (QtValues, bool, error) {
	return QtValues{}, false, nil
}
