package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JangoCG/fastmail-cli/internal/jmap"
)

type Config struct {
	SessionURL  string `json:"session_url"`
	AccountID   string `json:"account_id,omitempty"`
	Identity    string `json:"identity,omitempty"`
	DAVUsername string `json:"dav_username,omitempty"`
	DAVURL      string `json:"dav_url,omitempty"`
}

func Dir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(base, "fastmail-cli"), nil
}
func Load() (Config, error) {
	cfg := Config{SessionURL: jmap.DefaultSessionURL, DAVURL: "https://caldav.fastmail.com/"}
	dir, err := Dir()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("read fastmail config: %w", err)
	}
	return cfg, nil
}
func Save(cfg Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	return WriteJSON(filepath.Join(dir, "config.json"), cfg)
}
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".fastmail-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
