// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitea.dev/cli/internal/settings"

	"github.com/zalando/go-keyring"
)

// legacyKeyringService holds the tokens that earlier versions stored under an opaque Identity.
const legacyKeyringService = "branchspire-cli"

// keyringService names the instance of a token in the platform credential manager, as
// branchspire-cli:host[:port][/path], keeping the scheme only for plain HTTP so that it differs from
// the HTTPS instance of the same host.
func keyringService(instance string) string {
	return "branchspire-cli:" + strings.TrimPrefix(instance, "https://")
}

// Credential holds the secret token material kept outside instance metadata.
type Credential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitzero"`
}

// Keyring is the operating-system credential storage boundary.
type Keyring interface {
	Get(string, string) (string, error)
	Set(string, string, string) error
	Delete(string, string) error
}

// SystemKeyring delegates to the platform credential manager.
type SystemKeyring struct{}

// Get retrieves a secret from the platform credential manager.
func (SystemKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }

// Set persists a secret in the platform credential manager.
func (SystemKeyring) Set(service, user, value string) error { return keyring.Set(service, user, value) }

// Delete removes only the selected instance/account credential.
func (SystemKeyring) Delete(service, user string) error { return keyring.Delete(service, user) }

// errNoKeyring rejects the keyring backend on a Store without a Keyring.
var errNoKeyring = errors.New("keyring storage unavailable")

// Store selects secure storage or explicitly opted-in private files.
type Store struct {
	Dir     string
	Keyring Keyring
	// Denied, when set, is returned instead of any stored credential.
	Denied error
}

// Identity builds a stable, opaque key from an instance URL and account.
func Identity(instance, user string) string {
	sum := sha256.Sum256([]byte(instance + "\x00" + user))
	return hex.EncodeToString(sum[:])
}

func (s *Store) filename(instance, user string) string {
	return filepath.Join(s.Dir, "credentials", Identity(instance, user)+".json")
}

// Load returns a credential without exposing storage contents in errors.
func (s *Store) Load(instance, user, backend string) (Credential, error) {
	if s.Denied != nil {
		return Credential{}, s.Denied
	}
	var data []byte
	var err error
	switch backend {
	case settings.StoreKeyring:
		if s.Keyring == nil {
			return Credential{}, errNoKeyring
		}
		value, loadErr := s.keyringGet(instance, user)
		data, err = []byte(value), loadErr
	case settings.StoreFile:
		data, err = settings.ReadFile(s.filename(instance, user))
	default:
		err = errors.New("unsupported credential store")
	}
	if errors.Is(err, keyring.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return Credential{}, errors.New("credential unavailable; run bh auth login for this instance and account")
	}
	if err != nil {
		return Credential{}, fmt.Errorf("credential storage unavailable: %w", err)
	}
	var credential Credential
	if err := json.Unmarshal(data, &credential); err != nil {
		return Credential{}, errors.New("invalid stored credential; run bh auth login")
	}
	if credential.AccessToken == "" {
		return Credential{}, errors.New("stored credential has no access token")
	}
	return credential, nil
}

// Save persists credentials without falling back to a different backend.
func (s *Store) Save(instance, user, backend string, credential Credential) error {
	if credential.AccessToken == "" {
		return errors.New("access token cannot be empty")
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	switch backend {
	case settings.StoreKeyring:
		if s.Keyring == nil {
			return errNoKeyring
		}
		if err := s.Keyring.Set(keyringService(instance), user, string(data)); err != nil {
			return fmt.Errorf("secure credential storage unavailable; use --credential-store file only if you accept private file storage: %w", err)
		}
		_ = s.Keyring.Delete(legacyKeyringService, Identity(instance, user)) // best effort: the token is saved
		return nil
	case settings.StoreFile:
		if err := os.MkdirAll(filepath.Dir(s.filename(instance, user)), 0o700); err != nil {
			return err
		}
		return settings.AtomicWrite(s.filename(instance, user), data)
	default:
		return errors.New("unsupported credential store")
	}
}

// Delete removes only the selected instance/account credential.
func (s *Store) Delete(instance, user, backend string) error {
	switch backend {
	case settings.StoreKeyring:
		if s.Keyring == nil {
			return errNoKeyring
		}
		for _, entry := range [][2]string{{keyringService(instance), user}, {legacyKeyringService, Identity(instance, user)}} {
			if err := s.Keyring.Delete(entry[0], entry[1]); err != nil && !errors.Is(err, keyring.ErrNotFound) {
				return fmt.Errorf("failed to remove credential from secure storage: %w", err)
			}
		}
		return nil
	case settings.StoreFile:
		err := os.Remove(s.filename(instance, user))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	default:
		return fmt.Errorf("unsupported credential store %q", backend)
	}
}

// keyringGet reads the token of an account, moving one that an earlier version stored under its
// legacy name to the readable one. A failed move leaves the legacy entry to be read next time.
func (s *Store) keyringGet(instance, user string) (string, error) {
	value, err := s.Keyring.Get(keyringService(instance), user)
	if !errors.Is(err, keyring.ErrNotFound) {
		return value, err
	}
	legacy := Identity(instance, user)
	value, err = s.Keyring.Get(legacyKeyringService, legacy)
	if err != nil {
		return value, err
	}
	// Another process may have saved a newer token, such as a refreshed one, in the meantime.
	if _, err := s.Keyring.Get(keyringService(instance), user); errors.Is(err, keyring.ErrNotFound) && s.Keyring.Set(keyringService(instance), user, value) == nil {
		_ = s.Keyring.Delete(legacyKeyringService, legacy)
	}
	return value, nil
}
