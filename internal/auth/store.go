package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/zalando/go-keyring"

	"github.com/JangoCG/fastmail-cli/internal/config"
)

const service = "fastmail-cli"

type Store struct{ Endpoint string }

func (s Store) key(kind string) string {
	h := sha256.Sum256([]byte(s.Endpoint))
	return kind + "-" + hex.EncodeToString(h[:])
}
func (s Store) file(kind string) (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, s.key(kind)+".json"), nil
}
func (s Store) Get(kind string) (string, error) {
	path, err := s.file(kind)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err == nil {
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return "", fmt.Errorf("credential file must have mode 0600: %s", path)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", readErr
		}
		var value string
		err = json.Unmarshal(data, &value)
		return value, err
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if os.Getenv("FASTMAIL_NO_KEYRING") != "1" {
		value, err := keyring.Get(service, s.key(kind))
		if err == nil {
			return value, nil
		}
	}
	return "", nil
}

// Set returns the selected storage so login can report a keyring fallback.
func (s Store) Set(kind, value string) (string, error) {
	path, err := s.file(kind)
	if err != nil {
		return "", err
	}
	if os.Getenv("FASTMAIL_NO_KEYRING") != "1" {
		if err = keyring.Set(service, s.key(kind), value); err == nil {
			if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
			return "keyring", nil
		}
	}
	// A fallback file takes precedence over any old, unavailable keyring entry.
	return "file", config.WriteJSON(path, value)
}
func (s Store) Delete(kind string) error {
	var keyErr error
	if os.Getenv("FASTMAIL_NO_KEYRING") != "1" {
		keyErr = keyring.Delete(service, s.key(kind))
		if errors.Is(keyErr, keyring.ErrNotFound) {
			keyErr = nil
		}
	}
	path, err := s.file(kind)
	if err != nil {
		return errors.Join(keyErr, err)
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return errors.Join(keyErr, err)
}
