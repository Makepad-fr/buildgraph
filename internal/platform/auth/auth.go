package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	keyring "github.com/zalando/go-keyring"
)

const (
	defaultService = "buildgraph"
	defaultUserKey = "auth"
)

type Credentials struct {
	User     string    `json:"user"`
	Token    string    `json:"token"`
	StoredAt time.Time `json:"storedAt"`
	Source   string    `json:"source"`
}

type Store interface {
	Save(creds Credentials) error
	Load() (Credentials, error)
	Delete() error
}

type Manager struct {
	primary  Store
	fallback Store
}

func NewManager(fallbackPath string) (*Manager, error) {
	fileStore := &FileStore{Path: fallbackPath}
	return &Manager{
		primary:  &KeyringStore{},
		fallback: fileStore,
	}, nil
}

func (m *Manager) Save(creds Credentials) error {
	if creds.StoredAt.IsZero() {
		creds.StoredAt = time.Now().UTC()
	}
	if err := m.primary.Save(creds); err == nil {
		return nil
	}
	return m.fallback.Save(creds)
}

func (m *Manager) Load() (Credentials, error) {
	if creds, err := m.primary.Load(); err == nil {
		return creds, nil
	}
	return m.fallback.Load()
}

func (m *Manager) Delete() error {
	if err := m.primary.Delete(); err == nil {
		return nil
	}
	return m.fallback.Delete()
}

type KeyringStore struct{}

func (s *KeyringStore) Save(creds Credentials) error {
	creds.Source = "keyring"
	payload, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return keyring.Set(defaultService, defaultUserKey, string(payload))
}

func (s *KeyringStore) Load() (Credentials, error) {
	value, err := keyring.Get(defaultService, defaultUserKey)
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(value), &creds); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}

func (s *KeyringStore) Delete() error {
	if err := keyring.Delete(defaultService, defaultUserKey); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

type FileStore struct {
	Path string
}

func (s *FileStore) Save(creds Credentials) error {
	if s.Path == "" {
		return fmt.Errorf("file store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	creds.Source = "file"
	payload, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, payload, 0o600)
}

func (s *FileStore) Load() (Credentials, error) {
	if s.Path == "" {
		return Credentials{}, fmt.Errorf("file store path is required")
	}
	payload, err := os.ReadFile(s.Path)
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal(payload, &creds); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}

func (s *FileStore) Delete() error {
	if s.Path == "" {
		return fmt.Errorf("file store path is required")
	}
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
