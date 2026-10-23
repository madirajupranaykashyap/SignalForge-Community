package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagementRateLimitAndSecurityHeaders(t *testing.T) {
	s, err := newServer(filepath.Join(t.TempDir(), "management.db"), strings.Repeat("x", 40))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.store.Close() })
	s.limiter = newRateLimiter(2)

	for i := 0; i < 2; i++ {
		w := managementCall(s, http.MethodGet, "/api/v1/admin/unknown", "", "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected 401, got %d", i+1, w.Code)
		}
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("missing nosniff header: %q", got)
		}
		if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
			t.Fatalf("missing frame protection header: %q", got)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("API response should not be cacheable: %q", got)
		}
	}

	w := managementCall(s, http.MethodGet, "/api/v1/admin/unknown", "", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected rate limit 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("rate limited response missing Retry-After")
	}
}

func TestManagementRateLimitDoesNotTrustForwardedAddress(t *testing.T) {
	l := newRateLimiter(1)
	now := time.Now()
	first, _ := l.allow(clientAddress("192.0.2.10:1234"), now)
	second, _ := l.allow(clientAddress("192.0.2.10:1235"), now.Add(time.Millisecond))
	if !first || second {
		t.Fatal("same source address should share a rate limit bucket")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "192.0.2.11:1234"
	req.Header.Set("X-Forwarded-For", "192.0.2.10")
	if got := clientAddress(req.RemoteAddr); got != "192.0.2.11" {
		t.Fatalf("unexpected client identity derived from forwarded header: %q", got)
	}
}

func TestHealthCheckIsExemptFromRateLimitButGetsSecurityHeaders(t *testing.T) {
	s, err := newServer(filepath.Join(t.TempDir(), "management.db"), strings.Repeat("h", 40))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.store.Close() })
	s.limiter = newRateLimiter(1)

	for i := 0; i < 5; i++ {
		w := managementCall(s, http.MethodGet, "/healthz", "", "")
		if w.Code != http.StatusOK {
			t.Fatalf("health check %d was rate limited: %d", i, w.Code)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("health check missing security headers")
		}
	}
}
