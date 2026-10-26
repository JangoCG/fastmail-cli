package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigRoundTripAndPermissions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil || cfg.SessionURL == "" || cfg.DAVURL == "" {
		t.Fatal(cfg, err)
	}
	cfg.Identity = "alice@example.com"
	if err = Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil || loaded != cfg {
		t.Fatal(loaded, err)
	}
	dir, _ := Dir()
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if err = Save(Config{Identity: "bob@example.com"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal(entries)
	}
}
func TestMalformedConfigFailsInsteadOfResetting(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, _ := Dir()
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte("{"), 0600)
	if _, err := Load(); err == nil {
		t.Fatal("corrupt config ignored")
	}
}
