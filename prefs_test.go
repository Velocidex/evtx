//go:build windows
// +build windows

package evtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSortListWithPreference(t *testing.T) {
	resolver, err := NewWindowsMessageResolver(MessageResolverOpts{})
	if err != nil {
		t.Fatal(err)
	}

	list := []string{
		`C:\Test\cs-CZ\MpEvMsg.dll.mui`,
		`C:\Test\en-US\MpEvMsg.dll.mui`,
		`C:\Test\zh-TW\MpEvMsg.dll.mui`,
	}

	sorted := resolver.sortListWithPreference(list)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 entries, got %v", sorted)
	}
	if !strings.Contains(sorted[0], "en-US") {
		t.Fatalf("expected en-US first, got %v", sorted)
	}
}

// MUI files for message DLLs outside System32 are discovered after the
// constructor's sorting pass - they must still come back in preference
// order.
func TestExpandLocationsPrefersLanguageForLazyMUIDirs(t *testing.T) {
	tmp := t.TempDir()

	dll := filepath.Join(tmp, "MpEvMsg.dll")
	if err := os.WriteFile(dll, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	// en-US deliberately surrounded alphabetically on both sides.
	for _, lang := range []string{"cs-CZ", "en-US", "zh-TW"} {
		mui_dir := filepath.Join(tmp, lang)
		if err := os.MkdirAll(mui_dir, 0700); err != nil {
			t.Fatal(err)
		}
		mui := filepath.Join(mui_dir, "MpEvMsg.dll.mui")
		if err := os.WriteFile(mui, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	resolver, err := NewWindowsMessageResolver(MessageResolverOpts{})
	if err != nil {
		t.Fatal(err)
	}

	locations := resolver.ExpandMessageFileLocation(dll)

	var muis []string
	for _, location := range locations {
		if strings.HasSuffix(location, ".mui") {
			muis = append(muis, location)
		}
	}

	if len(muis) != 3 {
		t.Fatalf("expected 3 mui files, got %v", locations)
	}
	if !strings.Contains(muis[0], "en-US") {
		t.Fatalf("expected en-US mui first, got %v", muis)
	}
}
