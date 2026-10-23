package configclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/configcache"
)

func testEnvelope() envelope {
	raw := json.RawMessage(`{"receivers":{"otlp":{}}}`)
	sum := sha256.Sum256(raw)
	return envelope{Version: 3, SchemaVersion: "1", Digest: hex.EncodeToString(sum[:]), Config: raw}
}

func TestPollerFetchesValidatesActivatesAndAcknowledges(t *testing.T) {
	var ackApplied bool
	var ackVersion int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agent-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agents/config":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testEnvelope())
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/config/ack":
			var q struct {
				Version int `json:"version"`
				Applied bool `json:"applied"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				t.Errorf("decode ack: %v", err)
			}
			ackVersion, ackApplied = q.Version, q.Applied
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher, err := NewFetcher(server.URL+"/api/v1/agents/config", "agent-secret", server.Client())
	if err != nil { t.Fatal(err) }
	cache, err := configcache.New(filepath.Join(t.TempDir(), "active.json"))
	if err != nil { t.Fatal(err) }
	activated := 0
	p := &Poller{
		Fetcher: fetcher,
		Cache: cache,
		Validate: func(schema string, raw json.RawMessage) error {
			if schema != "1" || !json.Valid(raw) { t.Fatal("invalid validator input") }
			return nil
		},
		Activate: func(_ context.Context, c configcache.Config) error {
			if c.Version != 3 { t.Fatalf("version = %d", c.Version) }
			activated++
			return nil
		},
	}
	if err := p.pollOnce(context.Background()); err != nil { t.Fatal(err) }
	active, ok := cache.Active()
	if !ok || active.Version != 3 || activated != 1 { t.Fatalf("active=%#v ok=%v activated=%d", active, ok, activated) }
	if ackVersion != 3 || !ackApplied { t.Fatalf("ack version=%d applied=%v", ackVersion, ackApplied) }
	if p.Status().LastSuccessfulSync.IsZero() { t.Fatal("successful sync timestamp not recorded") }
}

func TestRejectedConfigIsNotCachedAndIsAcknowledged(t *testing.T) {
	var ackApplied bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/config":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testEnvelope())
		case "/api/v1/agents/config/ack":
			var q struct { Applied bool `json:"applied"` }
			_ = json.NewDecoder(r.Body).Decode(&q)
			ackApplied = q.Applied
			w.WriteHeader(http.StatusNoContent)
		default: http.NotFound(w, r)
		}
	}))
	defer server.Close()
	fetcher, err := NewFetcher(server.URL+"/api/v1/agents/config", "agent-secret", server.Client())
	if err != nil { t.Fatal(err) }
	cache, err := configcache.New(filepath.Join(t.TempDir(), "active.json"))
	if err != nil { t.Fatal(err) }
	p := &Poller{Fetcher:fetcher, Cache:cache, Validate:func(string, json.RawMessage) error { return context.DeadlineExceeded }}
	if err := p.pollOnce(context.Background()); err == nil { t.Fatal("expected validation failure") }
	if _, ok := cache.Active(); ok { t.Fatal("rejected config was cached") }
	if ackApplied { t.Fatal("rejected config acknowledged as applied") }
	if !strings.Contains(p.Status().LastError, "invalid") { t.Fatalf("unexpected status error: %q", p.Status().LastError) }
}

func TestNoAssignmentIsSuccessfulPollWithoutAcknowledgement(t *testing.T) {
	acks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agents/config/ack" { acks++; w.WriteHeader(http.StatusNoContent); return }
		_, _ = w.Write([]byte(`{"config_version":0,"config":null}`))
	}))
	defer server.Close()
	fetcher, err := NewFetcher(server.URL+"/api/v1/agents/config", "agent-secret", server.Client())
	if err != nil { t.Fatal(err) }
	cache, err := configcache.New(filepath.Join(t.TempDir(), "active.json"))
	if err != nil { t.Fatal(err) }
	p := &Poller{Fetcher:fetcher, Cache:cache}
	if err := p.pollOnce(context.Background()); err != nil { t.Fatal(err) }
	if acks != 0 { t.Fatalf("unexpected acknowledgements: %d", acks) }
	if p.Status().LastSuccessfulSync.IsZero() { t.Fatal("no-assignment poll should be healthy") }
}

func TestFetcherRequiresSecureRemoteEndpoint(t *testing.T) {
	if _, err := NewFetcher("http://control.example.com/api/v1/agents/config", "secret", nil); err == nil {
		t.Fatal("non-loopback HTTP endpoint accepted")
	}
	if _, err := NewFetcher("https://user:pass@control.example.com/api/v1/agents/config", "secret", nil); err == nil {
		t.Fatal("endpoint with userinfo accepted")
	}
	if _, err := NewFetcher("http://127.0.0.1:8081/api/v1/agents/config", "secret", nil); err != nil {
		t.Fatalf("loopback HTTP should be allowed for local development: %v", err)
	}
}

func TestBackoffJitterStaysWithinBounds(t *testing.T) {
	for i := 0; i < 100; i++ {
		got := jitter(10 * time.Second)
		if got < 8*time.Second || got > 12*time.Second { t.Fatalf("jitter = %v", got) }
	}
}
