// Package configclient polls the SignalForge management API and feeds immutable
// configuration envelopes into the last-known-good cache.
package configclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/configcache"
)

var ErrNoAssignment = errors.New("no configuration is assigned to this agent")

const maxResponseBytes = 4 << 20

type envelope struct {
	TenantID string `json:"tenant_id,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	ConfigVersion int `json:"config_version,omitempty"`
	Version int             `json:"version"`
	SchemaVersion string   `json:"schema_version"`
	Digest string          `json:"digest"`
	Config json.RawMessage `json:"config"`
}

type Fetcher struct {
	endpoint string
	ackEndpoint string
	credential string
	httpClient *http.Client
	mu sync.Mutex
	last *configcache.Config
}

func NewFetcher(endpoint, credential string, client *http.Client) (*Fetcher, error) {
	if strings.TrimSpace(credential) == "" {
		return nil, errors.New("agent credential is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("config endpoint must be an absolute HTTP(S) URL without userinfo, query, or fragment")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return nil, errors.New("config endpoint must use HTTPS except for loopback development")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("unsupported config endpoint scheme")
	}
	if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/api/v1/agents/config") {
		return nil, errors.New("config endpoint path must end in /api/v1/agents/config")
	}
	if client == nil {
		client = &http.Client{}
	}
	clientCopy := *client
	if clientCopy.Timeout <= 0 {
		clientCopy.Timeout = 10 * time.Second
	}
	// Never forward an agent bearer credential through an HTTP redirect.
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	client = &clientCopy
	endpoint = strings.TrimRight(endpoint, "/")
	ackEndpoint := strings.TrimSuffix(endpoint, "/config") + "/config/ack"
	return &Fetcher{endpoint: endpoint, ackEndpoint: ackEndpoint, credential: credential, httpClient: client}, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (f *Fetcher) FetchConfig(ctx context.Context) (configcache.Config, error) {
	f.mu.Lock()
	f.last = nil
	f.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.endpoint, nil)
	if err != nil {
		return configcache.Config{}, err
	}
	req.Header.Set("Authorization", "Bearer "+f.credential)
	req.Header.Set("Accept", "application/json")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return configcache.Config{}, fmt.Errorf("request config endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return configcache.Config{}, fmt.Errorf("config endpoint rejected agent credentials (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return configcache.Config{}, fmt.Errorf("config endpoint returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return configcache.Config{}, fmt.Errorf("read config response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return configcache.Config{}, errors.New("config response exceeds size limit")
	}
	var e envelope
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return configcache.Config{}, fmt.Errorf("decode config response: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return configcache.Config{}, errors.New("config response must contain exactly one JSON object")
	}
	if len(e.Config) == 0 || bytes.Equal(bytes.TrimSpace(e.Config), []byte("null")) {
		return configcache.Config{}, ErrNoAssignment
	}
	c := configcache.Config{Version: e.Version, SchemaVersion: e.SchemaVersion, Digest: e.Digest, Config: append(json.RawMessage(nil), e.Config...)}
	f.mu.Lock()
	f.last = &c
	f.mu.Unlock()
	return c, nil
}

func (f *Fetcher) LastFetched() (configcache.Config, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.last == nil {
		return configcache.Config{}, false
	}
	c := *f.last
	c.Config = append(json.RawMessage(nil), c.Config...)
	return c, true
}

func (f *Fetcher) Acknowledge(ctx context.Context, version int, applied bool, message string) error {
	payload, err := json.Marshal(struct {
		Version int `json:"version"`
		Applied bool `json:"applied"`
		Message string `json:"message"`
	}{Version: version, Applied: applied, Message: truncate(message, 512)})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.ackEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+f.credential)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("acknowledge config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("config acknowledgement returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max]
	}
	return s
}

type Validator = configcache.Validator
type Activator = configcache.Activator

type Status struct {
	LastPoll time.Time `json:"last_poll,omitempty"`
	LastSuccessfulSync time.Time `json:"last_successful_sync,omitempty"`
	LastError string `json:"last_error,omitempty"`
	ActiveVersion int `json:"active_version"`
	Running bool `json:"running"`
}

type Poller struct {
	Fetcher *Fetcher
	Cache *configcache.Store
	Validate Validator
	Activate Activator
	Interval time.Duration
	MinBackoff time.Duration
	MaxBackoff time.Duration
	mu sync.RWMutex
	status Status
}

func (p *Poller) Status() Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s := p.status
	if p.Cache != nil {
		if c, ok := p.Cache.Active(); ok {
			s.ActiveVersion = c.Version
		}
	}
	return s
}

func (p *Poller) Run(ctx context.Context) error {
	if p.Fetcher == nil || p.Cache == nil {
		return errors.New("config poller requires a fetcher and cache")
	}
	if _, err := p.Cache.Restore(ctx, p.Validate, p.Activate); err != nil {
		p.mu.Lock()
		p.status.LastError = safeError(err)
		p.mu.Unlock()
		return fmt.Errorf("restore last-known-good config: %w", err)
	}
	interval := p.Interval
	if interval <= 0 { interval = 30 * time.Second }
	minBackoff := p.MinBackoff
	if minBackoff <= 0 { minBackoff = time.Second }
	maxBackoff := p.MaxBackoff
	if maxBackoff <= 0 { maxBackoff = time.Minute }
	if maxBackoff < minBackoff { maxBackoff = minBackoff }
	p.mu.Lock()
	p.status.Running = true
	p.mu.Unlock()
	defer func() { p.mu.Lock(); p.status.Running = false; p.mu.Unlock() }()

	backoff := minBackoff
	for {
		err := p.pollOnce(ctx)
		delay := interval
		if err != nil {
			delay = jitter(backoff)
			backoff *= 2
			if backoff > maxBackoff { backoff = maxBackoff }
		} else {
			backoff = minBackoff
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() { <-timer.C }
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context) error {
	now := time.Now().UTC()
	p.mu.Lock()
	p.status.LastPoll = now
	p.mu.Unlock()

	_, err := p.Cache.Refresh(ctx, p.Fetcher, p.Validate, p.Activate)
	if errors.Is(err, ErrNoAssignment) {
		p.setSuccess(now, "")
		return nil
	}
	if err != nil {
		p.mu.Lock()
		p.status.LastError = safeError(err)
		p.mu.Unlock()
		if c, ok := p.Fetcher.LastFetched(); ok {
			ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			ackErr := p.Fetcher.Acknowledge(ackCtx, c.Version, false, safeError(err))
			cancel()
			if ackErr != nil {
				p.mu.Lock()
				p.status.LastError = safeError(fmt.Errorf("%v; acknowledgement failed: %w", err, ackErr))
				p.mu.Unlock()
			}
		}
		return err
	}
	p.setSuccess(now, "")
	if c, ok := p.Fetcher.LastFetched(); ok {
		ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		ackErr := p.Fetcher.Acknowledge(ackCtx, c.Version, true, "")
		cancel()
		if ackErr != nil {
			p.mu.Lock()
			p.status.LastError = safeError(ackErr)
			p.mu.Unlock()
			return ackErr
		}
	}
	return nil
}

func (p *Poller) setSuccess(at time.Time, message string) {
	p.mu.Lock()
	p.status.LastSuccessfulSync = at
	p.status.LastError = message
	p.mu.Unlock()
}

func safeError(err error) string {
	if err == nil { return "" }
	switch {
	case errors.Is(err, ErrNoAssignment):
		return ""
	case errors.Is(err, configcache.ErrDigestMismatch):
		return "configuration digest mismatch"
	case errors.Is(err, configcache.ErrStaleVersion):
		return "stale configuration version rejected"
	case errors.Is(err, configcache.ErrInvalidConfig):
		return "configuration rejected by integrity or schema validation"
	default:
		return "control-plane synchronization failed"
	}
}

func jitter(d time.Duration) time.Duration {
	if d <= 0 { return time.Second }
	factor := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(d) * factor)
}
