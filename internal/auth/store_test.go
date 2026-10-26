package auth

import (
	"os"
	"runtime"
	"testing"
)

func TestEndpointScopedPrivateCredentials(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("FASTMAIL_NO_KEYRING", "1")
	a := Store{Endpoint: "https://api.fastmail.com/jmap/session"}
	b := Store{Endpoint: "https://example.com/jmap/session"}
	if storage, err := a.Set("token", "private-token"); err != nil || storage != "file" {
		t.Fatal(storage, err)
	}
	if value, err := a.Get("token"); err != nil || value != "private-token" {
		t.Fatal(value, err)
	}
	if value, err := b.Get("token"); err != nil || value != "" {
		t.Fatal("credentials crossed endpoints", value, err)
	}
	if value, _ := a.Get("dav"); value != "" {
		t.Fatal("API token used as app password")
	}
	path, _ := a.file("token")
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if err := a.Delete("token"); err != nil {
		t.Fatal(err)
	}
	if err := a.Delete("token"); err != nil {
		t.Fatal(err)
	}
	if value, _ := a.Get("token"); value != "" {
		t.Fatal(value)
	}
}
func TestInsecureCredentialFileRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("FASTMAIL_NO_KEYRING", "1")
	s := Store{Endpoint: "https://example.com"}
	_, _ = s.Set("token", "token")
	path, _ := s.file("token")
	_ = os.Chmod(path, 0644)
	if _, err := s.Get("token"); err == nil {
		t.Fatal("insecure credential permissions accepted")
	}
}
