package configclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/configcache"
)

type fixedFetcher struct { config configcache.Config }
func (f fixedFetcher) FetchConfig(context.Context) (configcache.Config, error) { return f.config, nil }

func testEnvelope() envelope {
	raw := json.RawMessage(`{"receivers":{"otlp":{}}}`)
	sum := sha256.Sum256(raw)
	return envelope{TenantID: "tenant-1", CreatedAt: time.Now().UTC(), Version: 3, SchemaVersion: "1", Digest: hex.EncodeToString(sum[:]), Config: raw}
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
	var ackMessage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/config":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testEnvelope())
		case "/api/v1/agents/config/ack":
			var q struct { Applied bool `json:"applied"`; Message string `json:"message"` }
			_ = json.NewDecoder(r.Body).Decode(&q)
			ackApplied, ackMessage = q.Applied, q.Message
			w.WriteHeader(http.StatusNoContent)
		default: http.NotFound(w, r)
		}
	}))
	defer server.Close()
	fetcher, err := NewFetcher(server.URL+"/api/v1/agents/config", "agent-secret", server.Client())
	if err != nil { t.Fatal(err) }
	cache, err := configcache.New(filepath.Join(t.TempDir(), "active.json"))
	if err != nil { t.Fatal(err) }
	p := &Poller{Fetcher:fetcher, Cache:cache, Validate:func(string, json.RawMessage) error { return errors.New("secret config value should never be exposed") }}
	if err := p.pollOnce(context.Background()); err == nil { t.Fatal("expected validation failure") }
	if _, ok := cache.Active(); ok { t.Fatal("rejected config was cached") }
	if ackApplied { t.Fatal("rejected config acknowledged as applied") }
	if !strings.Contains(p.Status().LastError, "rejected") { t.Fatalf("unexpected status error: %q", p.Status().LastError) }
	if strings.Contains(p.Status().LastError, "secret") || strings.Contains(ackMessage, "secret") { t.Fatal("configuration details leaked through status or acknowledgement") }
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

func TestRunRestoresCachedConfigBeforePolling(t *testing.T) {
	envelope := testEnvelope()
	config := configcache.Config{Version: envelope.Version, SchemaVersion: envelope.SchemaVersion, Digest: envelope.Digest, Config: envelope.Config}
	cache, err := configcache.New(filepath.Join(t.TempDir(), "active.json"))
	if err != nil { t.Fatal(err) }
	if _, err := cache.Refresh(context.Background(), fixedFetcher{config: config}, nil, nil); err != nil { t.Fatal(err) }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/config":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(envelope)
		case "/api/v1/agents/config/ack":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	fetcher, err := NewFetcher(server.URL+"/api/v1/agents/config", "agent-secret", server.Client())
	if err != nil { t.Fatal(err) }
	activations := 0
	p := &Poller{Fetcher: fetcher, Cache: cache, Interval: time.Hour, Activate: func(context.Context, configcache.Config) error { activations++; return nil }}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = p.Run(ctx)
	if err != context.DeadlineExceeded { t.Fatalf("Run error = %v", err) }
	if activations != 1 { t.Fatalf("activation count = %d, want startup restore only", activations) }
	if p.Status().Running { t.Fatal("poller still marked running after exit") }
}

func TestFetcherDoesNotFollowRedirects(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits++; w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/v1/agents/config", http.StatusFound)
	}))
	defer source.Close()
	fetcher, err := NewFetcher(source.URL+"/api/v1/agents/config", "agent-secret", source.Client())
	if err != nil { t.Fatal(err) }
	if _, err := fetcher.FetchConfig(context.Background()); err == nil { t.Fatal("redirect response unexpectedly accepted") }
	if targetHits != 0 { t.Fatalf("redirect target received %d requests", targetHits) }
}
