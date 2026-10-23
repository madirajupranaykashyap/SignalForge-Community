package main

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultRateLimitRPM = 120
const maxRateLimitClients = 4096

type rateLimitBucket struct {
	tokens float64
	last   time.Time
	seen   time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	rpm     int
	clients map[string]rateLimitBucket
	calls   uint64
}

func newRateLimiter(rpm int) *rateLimiter {
	if rpm < 1 {
		rpm = defaultRateLimitRPM
	}
	return &rateLimiter{rpm: rpm, clients: make(map[string]rateLimitBucket)}
}

func clientAddress(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	// Do not trust X-Forwarded-For or other caller-controlled forwarding headers.
	return remoteAddr
}

func (l *rateLimiter) allow(key string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	capacity := float64(l.rpm)
	refillPerSecond := capacity / 60
	bucket, exists := l.clients[key]
	if !exists {
		if len(l.clients) >= maxRateLimitClients {
			for address, old := range l.clients {
				if now.Sub(old.seen) > 2*time.Minute {
					delete(l.clients, address)
				}
			}
		}
		if len(l.clients) >= maxRateLimitClients {
			return false, 60
		}
		bucket = rateLimitBucket{tokens: capacity, last: now, seen: now}
	} else {
		elapsed := now.Sub(bucket.last).Seconds()
		if elapsed > 0 {
			bucket.tokens = math.Min(capacity, bucket.tokens+elapsed*refillPerSecond)
			bucket.last = now
		}
		bucket.seen = now
	}

	if bucket.tokens < 1 {
		wait := (1 - bucket.tokens) / refillPerSecond
		l.clients[key] = bucket
		retry := int(math.Ceil(wait))
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}

	bucket.tokens--
	l.clients[key] = bucket
	l.calls++
	if l.calls%128 == 0 {
		for address, old := range l.clients {
			if now.Sub(old.seen) > 2*time.Minute {
				delete(l.clients, address)
			}
		}
	}
	return true, 0
}

// ServeHTTP applies defensive response headers and a per-source token bucket.
// Health probes are exempt so a burst of API traffic does not make liveness checks fail.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
	}

	if r.URL.Path != "/healthz" && s.limiter != nil {
		allowed, retryAfter := s.limiter.allow(clientAddress(r.RemoteAddr), time.Now())
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
	}
	s.serveHTTPCore(w, r)
}
