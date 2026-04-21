package history

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dukto/internal/settings"
)

func newStore(t *testing.T) *settings.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := settings.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAppendAndPayload(t *testing.T) {
	s := newStore(t)
	at := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	it := settings.HistoryItem{Kind: "file", Name: "a.bin", Path: "/x/a.bin", At: at, From: "1.2.3.4:1234"}
	if err := Append(s, it); err != nil {
		t.Fatal(err)
	}
	list := All(s)
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}
	got := list[0]
	if got["kind"] != "file" || got["name"] != "a.bin" {
		t.Fatalf("payload mismatch: %+v", got)
	}
	if got["at"].(int64) != at.UnixMilli() {
		t.Fatalf("at = %v, want %d", got["at"], at.UnixMilli())
	}
}

func TestAppendCap(t *testing.T) {
	s := newStore(t)
	for range Cap + 5 {
		if err := Append(s, settings.HistoryItem{Kind: "file", Name: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(s.Values().History); got != Cap {
		t.Fatalf("history len = %d, want %d", got, Cap)
	}
}

func TestClear(t *testing.T) {
	s := newStore(t)
	_ = Append(s, settings.HistoryItem{Kind: "text", Text: "hi"})
	if err := Clear(s); err != nil {
		t.Fatal(err)
	}
	if len(s.Values().History) != 0 {
		t.Fatal("expected empty history after Clear")
	}
}

func TestExportJSON(t *testing.T) {
	s := newStore(t)
	_ = Append(s, settings.HistoryItem{Kind: "text", Text: "hello", At: time.Unix(0, 0).UTC()})

	out := filepath.Join(t.TempDir(), "out.json")
	if _, err := Export(s, "json", out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got []settings.HistoryItem
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("bad json: %v (body=%s)", err, raw)
	}
	if len(got) != 1 || got[0].Text != "hello" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestExportCSV(t *testing.T) {
	s := newStore(t)
	_ = Append(s, settings.HistoryItem{Kind: "file", Name: "a.bin", Path: "/x/a.bin", At: time.Unix(0, 0).UTC(), From: "1.2.3.4"})
	out := filepath.Join(t.TempDir(), "out.csv")
	if _, err := Export(s, "csv", out); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want header + 1", len(rows))
	}
	if rows[0][0] != "kind" {
		t.Fatalf("header = %v", rows[0])
	}
	if rows[1][0] != "file" || rows[1][1] != "a.bin" {
		t.Fatalf("row = %v", rows[1])
	}
}

func TestExportUnknown(t *testing.T) {
	s := newStore(t)
	out := filepath.Join(t.TempDir(), "x.txt")
	if _, err := Export(s, "xml", out); err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("expected unknown-format error, got %v", err)
	}
}
