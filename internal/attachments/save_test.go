package attachments

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveBytesPreservesExistingAndReplacesExplicitly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoice.pdf")
	if _, err := SaveBytes(path, []byte("original"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveBytes(path, []byte("replacement"), false); err == nil {
		t.Fatal("overwrote existing file")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "original" {
		t.Fatal(string(data))
	}
	if _, err := SaveBytes(path, []byte("replacement"), true); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "replacement" {
		t.Fatal(string(data))
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary file leaked", entries)
	}
}
func TestDerivedFilenamesCannotEscapeDestination(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../../invoice.pdf", `..\..\invoice.pdf`} {
		path, err := Destination(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		if path != filepath.Join(dir, "invoice.pdf") {
			t.Fatal(path)
		}
	}
	for _, name := range []string{"", "..", "/"} {
		if _, err := PortableFilename(name); err == nil {
			t.Fatal("unsafe filename", name)
		}
	}
}
func TestSaveDoesNotFollowDestinationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "original")
	link := filepath.Join(dir, "download")
	_ = os.WriteFile(target, []byte("original"), 0600)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveBytes(link, []byte("download"), false); err == nil {
		t.Fatal("replaced symlink without --force")
	}
	if _, err := SaveBytes(link, []byte("download"), true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "original" {
		t.Fatal("followed symlink")
	}
}
