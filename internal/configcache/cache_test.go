package configcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeFetcher struct { config Config; err error }
func (f fakeFetcher) FetchConfig(context.Context) (Config, error) { return f.config, f.err }

func makeConfig(version int, payload string) Config {
	raw := json.RawMessage(payload)
	sum := sha256.Sum256(raw)
	return Config{Version:version, SchemaVersion:"1", Digest:hex.EncodeToString(sum[:]), Config:raw}
}

func TestRefreshPersistsAndRestoresLastKnownGood(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache", "active.json")
	s, err := New(path); if err != nil { t.Fatal(err) }
	c := makeConfig(1, "{ \"processors\" : { \"batch\" : {} } }")
	changed, err := s.Refresh(context.Background(), fakeFetcher{config:c}, func(schema string, raw json.RawMessage) error {
		if schema != "1" || !json.Valid(raw) { t.Fatal("unexpected validator input") }; return nil
	}, func(_ context.Context, got Config) error {
		if got.Version != 1 { t.Fatalf("version = %d", got.Version) }; return nil
	})
	if err != nil || !changed { t.Fatalf("Refresh = (%v, %v)", changed, err) }
	info, err := os.Stat(path); if err != nil { t.Fatal(err) }
	if info.Mode().Perm() != 0600 { t.Fatalf("cache permissions = %o, want 600", info.Mode().Perm()) }
	reloaded, err := New(path); if err != nil { t.Fatal(err) }
	restored, err := reloaded.Restore(context.Background(), nil, func(_ context.Context, got Config) error {
		if got.Digest != c.Digest { t.Fatal("restored wrong digest") }; return nil
	})
	if err != nil || !restored { t.Fatalf("Restore = (%v, %v)", restored, err) }
}

func TestRejectedConfigDoesNotReplaceCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active.json")
	s, err := New(path); if err != nil { t.Fatal(err) }
	first := makeConfig(1, "{\"processors\":{\"batch\":{}}}")
	if changed, err := s.Refresh(context.Background(), fakeFetcher{config:first}, nil, nil); err != nil || !changed { t.Fatalf("initial refresh = (%v, %v)", changed, err) }
	second := makeConfig(2, "{\"processors\":{\"not-real\":{}}}")
	_, err = s.Refresh(context.Background(), fakeFetcher{config:second}, func(string, json.RawMessage) error { return errors.New("unsupported processor") }, nil)
	if !errors.Is(err, ErrInvalidConfig) { t.Fatalf("error = %v, want ErrInvalidConfig", err) }
	got, ok := s.Active(); if !ok || got.Version != 1 { t.Fatalf("active config after rejection = %#v, %v", got, ok) }
	disk, err := readConfig(path); if err != nil || disk.Version != 1 { t.Fatalf("disk config after rejection = %#v, %v", disk, err) }
}

func TestDigestStaleAndActivationFailuresFailClosed(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "active.json")); if err != nil { t.Fatal(err) }
	first := makeConfig(2, "{\"service\":\"ok\"}")
	if _, err := s.Refresh(context.Background(), fakeFetcher{config:first}, nil, nil); err != nil { t.Fatal(err) }
	if _, err := s.Refresh(context.Background(), fakeFetcher{config:makeConfig(1, "{\"service\":\"old\"}")}, nil, nil); !errors.Is(err, ErrStaleVersion) { t.Fatalf("stale error = %v", err) }
	badDigest := makeConfig(3, "{\"service\":\"bad\"}"); badDigest.Digest = "bad"
	if _, err := s.Refresh(context.Background(), fakeFetcher{config:badDigest}, nil, nil); !errors.Is(err, ErrDigestMismatch) { t.Fatalf("digest error = %v", err) }
	candidate := makeConfig(3, "{\"service\":\"new\"}")
	_, err = s.Refresh(context.Background(), fakeFetcher{config:candidate}, nil, func(context.Context, Config) error { return errors.New("apply failed") })
	if err == nil { t.Fatal("expected activation error") }
	got, _ := s.Active(); if got.Version != 2 { t.Fatalf("active version = %d, want 2", got.Version) }
}
