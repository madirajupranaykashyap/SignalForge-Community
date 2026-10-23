package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/oidcauth"
)

func hasRole(p oidcauth.Principal, required string) bool {
	for _, role := range p.Roles {
		if role == required || role == "platform_admin" { return true }
	}
	return false
}

func (s *Server) tenantAPI(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.Error(w, "tenant OIDC authentication is not configured", http.StatusServiceUnavailable)
		return
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	principal, err := s.oidc.Verify(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/tenants/"), "/")
	if len(parts) != 2 || parts[0] == "" || (parts[1] != "agents" && parts[1] != "policies") {
		http.NotFound(w, r)
		return
	}
	tenantID := parts[0]
	// Never trust a tenant ID from a request body or URL alone. The identity
	// provider's signed tenant claim is the authorization boundary.
	if principal.TenantID != tenantID {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.Method == http.MethodGet && parts[1] == "agents":
		if !hasRole(principal, "tenant_viewer") && !hasRole(principal, "tenant_operator") && !hasRole(principal, "tenant_admin") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.state.Tenants[tenantID] {
			http.NotFound(w, r)
			return
		}
		agents := make([]Agent, 0)
		for _, a := range s.state.Agents {
			if a.TenantID == tenantID {
				a.CredentialHash = ""
				agents = append(agents, a)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tenant_id": tenantID, "agents": agents})
	case r.Method == http.MethodPost && parts[1] == "policies":
		if !hasRole(principal, "tenant_admin") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var q struct {
			SchemaVersion string          `json:"schema_version"`
			Config        json.RawMessage `json:"config"`
		}
		if !s.decode(w, r, &q) {
			return
		}
		if q.SchemaVersion == "" || len(q.Config) == 0 || !json.Valid(q.Config) {
			http.Error(w, "invalid schema_version or config", http.StatusUnprocessableEntity)
			return
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, q.Config); err != nil {
			http.Error(w, "invalid config JSON", http.StatusUnprocessableEntity)
			return
		}
		config := append(json.RawMessage(nil), compact.Bytes()...)
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.state.Tenants[tenantID] {
			http.NotFound(w, r)
			return
		}
		policies := s.state.Policies[tenantID]
		version := 1
		if len(policies) > 0 {
			version = policies[len(policies)-1].Version + 1
		}
		digest := sha256.Sum256(config)
		now := time.Now().UTC()
		p := Policy{TenantID: tenantID, Version: version, SchemaVersion: q.SchemaVersion, Config: config, Digest: hex.EncodeToString(digest[:]), CreatedAt: now}
		s.state.Policies[tenantID] = append(policies, p)
		for id, a := range s.state.Agents {
			if a.TenantID == tenantID && !a.Revoked {
				a.ConfigVersion = version
				s.state.Agents[id] = a
			}
		}
		s.state.Audit = append(s.state.Audit, map[string]string{
			"action": "policy.create", "tenant_id": tenantID, "actor": principal.Subject,
			"version": strconv.Itoa(version), "at": now.Format(time.RFC3339Nano),
		})
		if err := s.persist(); err != nil {
			http.Error(w, "persistence failure", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
	default:
		w.Header().Set("Allow", map[string]string{"agents": "GET", "policies": "POST"}[parts[1]])
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
