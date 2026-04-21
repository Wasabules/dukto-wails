//go:build windows

package settings

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// TestReadRegBool_DWORDAndString exercises both encodings Qt has historically
// used for boolean keys: REG_DWORD 0/1 and REG_SZ "true"/"false".
func TestReadRegBool_DWORDAndString(t *testing.T) {
	const path = `Software\dukto-test-readRegBool`
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
	})

	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()

	if err := k.SetDWordValue("DwordTrue", 1); err != nil {
		t.Fatal(err)
	}
	if err := k.SetDWordValue("DwordFalse", 0); err != nil {
		t.Fatal(err)
	}
	if err := k.SetStringValue("StringTrue", "true"); err != nil {
		t.Fatal(err)
	}
	if err := k.SetStringValue("StringFalse", "false"); err != nil {
		t.Fatal(err)
	}
	if err := k.SetStringValue("StringGarbage", "maybe"); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		want    bool
		wantOk  bool
	}{
		{"DwordTrue", true, true},
		{"DwordFalse", false, true},
		{"StringTrue", true, true},
		{"StringFalse", false, true},
		{"StringGarbage", false, false},
		{"Missing", false, false},
	}
	for _, tc := range cases {
		got, ok := readRegBool(k, tc.name)
		if got != tc.want || ok != tc.wantOk {
			t.Errorf("%s: got (%v,%v), want (%v,%v)", tc.name, got, ok, tc.want, tc.wantOk)
		}
	}
}
