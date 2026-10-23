package configcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrStaleVersion = errors.New("configuration version is older than the active version")
	ErrDigestMismatch = errors.New("configuration digest does not match payload")
	ErrInvalidConfig = errors.New("configuration is invalid")
)

type Config struct {
	Version int `json:"version"`
	SchemaVersion string `json:"schema_version"`
	Digest string `json:"digest"`
	Config json.RawMessage `json:"config"`
}

type Validator func(schemaVersion string, payload json.RawMessage) error
type Activator func(context.Context, Config) error
type Fetcher interface { FetchConfig(context.Context) (Config, error) }

type Store struct {
	mu sync.Mutex
	path string
	active *Config
}

func New(path string) (*Store, error) {
	if path == "" { return nil, errors.New("config cache path is required") }
	s := &Store{path:path}
	c, err := readConfig(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) { return nil, fmt.Errorf("read cached config: %w", err) }
	if err == nil { s.active = &c }
	return s, nil
}

func (s *Store) Active() (Config, bool) {
	s.mu.Lock(); defer s.mu.Unlock()
	if s.active == nil { return Config{}, false }
	return clone(*s.active), true
}

func (s *Store) Restore(ctx context.Context, validate Validator, activate Activator) (bool, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	if s.active == nil { return false, nil }
	c := clone(*s.active)
	if err := verify(c); err != nil { return false, fmt.Errorf("cached config integrity check: %w", err) }
	if validate != nil {
		if err := validate(c.SchemaVersion, c.Config); err != nil { return false, fmt.Errorf("cached config validation: %w", err) }
	}
	if activate != nil {
		if err := activate(ctx, c); err != nil { return false, fmt.Errorf("activate cached config: %w", err) }
	}
	return true, nil
}

// Refresh only replaces the durable cache after the candidate verifies, validates,
// and activates. Activator must be atomic: on error the previous config stays active.
func (s *Store) Refresh(ctx context.Context, fetcher Fetcher, validate Validator, activate Activator) (bool, error) {
	if fetcher == nil { return false, errors.New("config fetcher is required") }
	s.mu.Lock(); defer s.mu.Unlock()
	c, err := fetcher.FetchConfig(ctx)
	if err != nil { return false, fmt.Errorf("fetch config: %w", err) }
	if err := verify(c); err != nil { return false, err }
	if s.active != nil {
		if c.Version < s.active.Version { return false, ErrStaleVersion }
		if c.Version == s.active.Version && c.Digest == s.active.Digest { return false, nil }
		if c.Version == s.active.Version { return false, fmt.Errorf("%w: same version has different digest", ErrInvalidConfig) }
	}
	if validate != nil {
		if err := validate(c.SchemaVersion, c.Config); err != nil { return false, fmt.Errorf("%w: %v", ErrInvalidConfig, err) }
	}
	tmp, err := writeTemp(s.path, c)
	if err != nil { return false, fmt.Errorf("stage config cache: %w", err) }
	defer os.Remove(tmp)
	if activate != nil {
		if err := activate(ctx, clone(c)); err != nil { return false, fmt.Errorf("activate config: %w", err) }
	}
	if err := os.Rename(tmp, s.path); err != nil { return false, fmt.Errorf("commit config cache: %w", err) }
	if dir, err := os.Open(filepath.Dir(s.path)); err == nil { _ = dir.Sync(); _ = dir.Close() }
	copy := clone(c); s.active = &copy
	return true, nil
}

func verify(c Config) error {
	if c.Version < 1 || c.SchemaVersion == "" || len(c.Config) == 0 || !json.Valid(c.Config) {
		return fmt.Errorf("%w: missing version/schema or invalid JSON payload", ErrInvalidConfig)
	}
	sum := sha256.Sum256(c.Config)
	if c.Digest != hex.EncodeToString(sum[:]) { return ErrDigestMismatch }
	return nil
}
func clone(c Config) Config { c.Config = append(json.RawMessage(nil), c.Config...); return c }
func readConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path); if err != nil { return c, err }
	if err := json.Unmarshal(b, &c); err != nil { return c, err }
	if err := verify(c); err != nil { return c, err }
	return c, nil
}
func writeTemp(path string, c Config) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil { return "", err }
	b, err := json.Marshal(c); if err != nil { return "", err }
	f, err := os.CreateTemp(dir, ".signalforge-config-*"); if err != nil { return "", err }
	name := f.Name(); ok := false
	defer func() { _ = f.Close(); if !ok { _ = os.Remove(name) } }()
	if err := f.Chmod(0600); err != nil { return "", err }
	if _, err := f.Write(b); err != nil { return "", err }
	if err := f.Sync(); err != nil { return "", err }
	if err := f.Close(); err != nil { return "", err }
	ok = true
	return name, nil
}
