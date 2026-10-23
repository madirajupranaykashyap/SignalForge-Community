package oidcauth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func enc(v any) string { b,_:=json.Marshal(v);return base64.RawURLEncoding.EncodeToString(b) }
func signedToken(t *testing.T,key *rsa.PrivateKey,header,claims map[string]any) string {
	t.Helper()
	a:=enc(header)+"."+enc(claims);sum:=sha256.Sum256([]byte(a));sig,err:=rsa.SignPKCS1v15(rand.Reader,key,crypto.SHA256,sum[:]);if err!=nil{t.Fatal(err)}
	return a+"."+base64.RawURLEncoding.EncodeToString(sig)
}
func jwk(key *rsa.PublicKey) map[string]string {
	return map[string]string{"kid":"key-1","kty":"RSA","use":"sig","alg":"RS256","n":base64.RawURLEncoding.EncodeToString(key.N.Bytes()),"e":base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}
}
func TestVerifyRS256AndTenantClaims(t *testing.T) {
	key,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){_ = json.NewEncoder(w).Encode(map[string]any{"keys":[]any{jwk(&key.PublicKey)}})}));defer server.Close()
	v,err:=New(Config{Issuer:"https://identity.example/",Audience:"signalforge-api",JWKSURL:server.URL,TenantClaim:"tenant_id",RolesClaim:"roles"});if err!=nil{t.Fatal(err)}
	now:=time.Now().Unix()
	token:=signedToken(t,key,map[string]any{"alg":"RS256","kid":"key-1"},map[string]any{"iss":"https://identity.example","aud":[]string{"other","signalforge-api"},"sub":"user-7","tenant_id":"tenant-a","roles":[]string{"tenant_admin"},"exp":now+300})
	p,err:=v.Verify(token);if err!=nil{t.Fatal(err)}
	if p.Subject!="user-7"||p.TenantID!="tenant-a"||len(p.Roles)!=1||p.Roles[0]!="tenant_admin"{t.Fatalf("unexpected principal: %+v",p)}
}
func TestRejectsWrongIssuerAudienceExpiryAndAlgorithm(t *testing.T) {
	key,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){_ = json.NewEncoder(w).Encode(map[string]any{"keys":[]any{jwk(&key.PublicKey)}})}));defer server.Close()
	v,err:=New(Config{Issuer:"https://identity.example",Audience:"api",JWKSURL:server.URL});if err!=nil{t.Fatal(err)}
	base:=map[string]any{"iss":"https://identity.example","aud":"api","sub":"u","tenant_id":"t","exp":time.Now().Unix()+60}
	for _,tc:=range []struct{name string;mutate func(map[string]any);alg string}{
		{"wrong issuer",func(c map[string]any){c["iss"]="https://evil.example"},"RS256"},
		{"wrong audience",func(c map[string]any){c["aud"]="other"},"RS256"},
		{"expired",func(c map[string]any){c["exp"]=time.Now().Unix()-100},"RS256"},
		{"algorithm confusion",func(c map[string]any){}, "HS256"},
	} {
		t.Run(tc.name,func(t *testing.T){c:=map[string]any{};for k,val:=range base{c[k]=val};tc.mutate(c);tok:=signedToken(t,key,map[string]any{"alg":tc.alg,"kid":"key-1"},c);if _,err:=v.Verify(tok);err==nil{t.Fatal("invalid token accepted")}})
	}
}
func TestNewRequiresSecureJWKSURL(t *testing.T) {
	for _,raw:=range []string{"http://identity.example/jwks","not a url",""} {
		if _,err:=New(Config{Issuer:"https://issuer.example",Audience:"api",JWKSURL:raw});err==nil{t.Fatalf("accepted unsafe URL %q",raw)}
	}
	if _,err:=New(Config{Issuer:"https://issuer.example",Audience:"api",JWKSURL:"http://localhost:9000/jwks"});err!=nil{t.Fatal(err)}
}
func TestRejectsMalformedTokenAndMissingTenant(t *testing.T) {
	key,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){_ = json.NewEncoder(w).Encode(map[string]any{"keys":[]any{jwk(&key.PublicKey)}})}));defer server.Close()
	v,err:=New(Config{Issuer:"https://issuer.example",Audience:"api",JWKSURL:server.URL});if err!=nil{t.Fatal(err)}
	if _,err=v.Verify("one.two");err==nil{t.Fatal("malformed token accepted")}
	tok:=signedToken(t,key,map[string]any{"alg":"RS256","kid":"key-1"},map[string]any{"iss":"https://issuer.example","aud":"api","sub":"u","exp":strconv.FormatInt(time.Now().Unix()+60,10)})
	if _,err=v.Verify(tok);err==nil||!strings.Contains(err.Error(),"required OIDC claims"){t.Fatalf("unexpected error: %v",err)}
}
