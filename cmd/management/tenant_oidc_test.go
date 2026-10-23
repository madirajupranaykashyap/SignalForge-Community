package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/oidcauth"
)

func tenantTestToken(t *testing.T, key *rsa.PrivateKey, tenant string, roles []string) string {
	t.Helper()
	encode := func(v any) string { b, err := json.Marshal(v); if err != nil { t.Fatal(err) }; return base64.RawURLEncoding.EncodeToString(b) }
	head := encode(map[string]any{"alg":"RS256","kid":"tenant-test-key"})
	payload := encode(map[string]any{"iss":"https://id.example.test","aud":"signalforge","sub":"user-1","tenant_id":tenant,"roles":roles,"exp":time.Now().Unix()+300})
	input:=head+"."+payload
	sum:=sha256.Sum256([]byte(input))
	sig,err:=rsa.SignPKCS1v15(rand.Reader,key,crypto.SHA256,sum[:]);if err!=nil{t.Fatal(err)}
	return input+"."+base64.RawURLEncoding.EncodeToString(sig)
}

func newTenantTestServer(t *testing.T) (*Server,*rsa.PrivateKey) {
	t.Helper()
	key,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
	jwk:=map[string]string{"kid":"tenant-test-key","kty":"RSA","use":"sig","alg":"RS256","n":base64.RawURLEncoding.EncodeToString(key.N.Bytes()),"e":base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}
	jwks:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){_ = json.NewEncoder(w).Encode(map[string]any{"keys":[]any{jwk}})}))
	t.Cleanup(jwks.Close)
	v,err:=oidcauth.New(oidcauth.Config{Issuer:"https://id.example.test",Audience:"signalforge",JWKSURL:jwks.URL});if err!=nil{t.Fatal(err)}
	admin:=strings.Repeat("x",40)
	s,err:=newServer(filepath.Join(t.TempDir(),"state.db"),admin);if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = s.store.Close()})
	s.oidc=v
	for _,tenant:=range []string{"tenant-a","tenant-b"} {
		req:=httptest.NewRequest(http.MethodPost,"/api/v1/admin/tenants",strings.NewReader(`{"tenant_id":"`+tenant+`"}`))
		req.Header.Set("Authorization","Bearer "+admin)
		w:=httptest.NewRecorder();s.ServeHTTP(w,req)
		if w.Code!=http.StatusCreated { t.Fatalf("seed tenant %s: %d %s",tenant,w.Code,w.Body) }
	}
	return s,key
}
func tenantCall(s *Server,method,path,token,body string)*httptest.ResponseRecorder{
	r:=httptest.NewRequest(method,path,strings.NewReader(body));if token!=""{r.Header.Set("Authorization","Bearer "+token)}
	w:=httptest.NewRecorder();s.ServeHTTP(w,r);return w
}
func TestTenantRBACAndIsolation(t *testing.T){
	s,key:=newTenantTestServer(t)
	viewer:=tenantTestToken(t,key,"tenant-a",[]string{"tenant_viewer"})
	admin:=tenantTestToken(t,key,"tenant-a",[]string{"tenant_admin"})
	other:=tenantTestToken(t,key,"tenant-b",[]string{"tenant_admin"})
	if w:=tenantCall(s,http.MethodGet,"/api/v1/tenants/tenant-a/agents","", "");w.Code!=http.StatusUnauthorized{t.Fatalf("missing token status=%d",w.Code)}
	if w:=tenantCall(s,http.MethodGet,"/api/v1/tenants/tenant-a/agents",other,"");w.Code!=http.StatusNotFound{t.Fatalf("cross-tenant read status=%d",w.Code)}
	if w:=tenantCall(s,http.MethodGet,"/api/v1/tenants/tenant-a/agents",viewer,"");w.Code!=http.StatusOK{t.Fatalf("viewer read status=%d body=%s",w.Code,w.Body)}
	if w:=tenantCall(s,http.MethodPost,"/api/v1/tenants/tenant-a/policies",viewer,`{"schema_version":"1","config":{"service":{}}}`);w.Code!=http.StatusForbidden{t.Fatalf("viewer wrote policy: status=%d body=%s",w.Code,w.Body)}
	if w:=tenantCall(s,http.MethodPost,"/api/v1/tenants/tenant-a/policies",admin,`{"schema_version":"1","config":{"service":{}}}`);w.Code!=http.StatusCreated{t.Fatalf("admin policy create status=%d body=%s",w.Code,w.Body)}
	if len(s.state.Policies["tenant-b"])!=0{t.Fatal("tenant A policy was written into tenant B")}
	if len(s.state.Audit)==0||s.state.Audit[len(s.state.Audit)-1]["actor"]!="user-1"{t.Fatal("tenant policy audit did not record authenticated actor")}
}
func TestTenantAPIUnavailableWhenOIDCIsNotConfigured(t *testing.T){
	s,err:=newServer(filepath.Join(t.TempDir(),"state.json"),strings.Repeat("x",40));if err!=nil{t.Fatal(err)}
	w:=tenantCall(s,http.MethodGet,"/api/v1/tenants/t/agents","","")
	if w.Code!=http.StatusServiceUnavailable{t.Fatalf("status=%d body=%s",w.Code,w.Body)}
}
