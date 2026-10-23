package oidcauth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Issuer string
	Audience string
	JWKSURL string
	TenantClaim string
	RolesClaim string
	HTTPClient *http.Client
}

type Principal struct {
	Subject string
	TenantID string
	Roles []string
}

type Verifier struct {
	issuer, audience, jwksURL, tenantClaim, rolesClaim string
	client *http.Client
	mu sync.Mutex
	keys map[string]*rsa.PublicKey
	keysUntil time.Time
}

type jwksDocument struct { Keys []struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N string `json:"n"`
	E string `json:"e"`
} `json:"keys"` }

func secureEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" { return false }
	if u.Scheme == "https" { return true }
	return u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

func New(cfg Config) (*Verifier, error) {
	if strings.TrimSpace(cfg.Issuer)=="" || strings.TrimSpace(cfg.Audience)=="" || strings.TrimSpace(cfg.JWKSURL)=="" {
		return nil, errors.New("OIDC issuer, audience, and JWKS URL are required")
	}
	if !secureEndpoint(cfg.Issuer) || !secureEndpoint(cfg.JWKSURL) {
		return nil, errors.New("OIDC issuer and JWKS URL must use HTTPS (HTTP is allowed only for loopback development)")
	}
	if cfg.TenantClaim=="" { cfg.TenantClaim="tenant_id" }
	if cfg.RolesClaim=="" { cfg.RolesClaim="roles" }
	client:=cfg.HTTPClient
	if client==nil { client=&http.Client{Timeout:5*time.Second, CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}} }
	return &Verifier{issuer:strings.TrimRight(cfg.Issuer,"/"),audience:cfg.Audience,jwksURL:cfg.JWKSURL,tenantClaim:cfg.TenantClaim,rolesClaim:cfg.RolesClaim,client:client,keys:map[string]*rsa.PublicKey{}},nil
}

func (v *Verifier) loadKeys(force bool) (map[string]*rsa.PublicKey,error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !force && time.Now().Before(v.keysUntil) && len(v.keys)>0 { return v.keys,nil }
	req,err:=http.NewRequest(http.MethodGet,v.jwksURL,nil); if err!=nil{return nil,err}
	resp,err:=v.client.Do(req);if err!=nil{return nil,errors.New("OIDC key service unavailable")}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK{return nil,fmt.Errorf("OIDC key service returned status %d",resp.StatusCode)}
	body,err:=io.ReadAll(io.LimitReader(resp.Body,(1<<20)+1));if err!=nil||len(body)>1<<20{return nil,errors.New("invalid OIDC key response")}
	var doc jwksDocument
	if err=json.Unmarshal(body,&doc);err!=nil{return nil,errors.New("invalid OIDC key response")}
	keys:=map[string]*rsa.PublicKey{}
	for _,k:=range doc.Keys {
		if k.Kty!="RSA" || (k.Use!=""&&k.Use!="sig") || (k.Alg!=""&&k.Alg!="RS256") || k.Kid=="" { continue }
		nb,e1:=base64.RawURLEncoding.DecodeString(k.N); eb,e2:=base64.RawURLEncoding.DecodeString(k.E)
		if e1!=nil||e2!=nil||len(eb)==0||len(eb)>4 { continue }
		e:=0;for _,b:=range eb { e=e*256+int(b) }
		if e<3||e%2==0 { continue }
		n:=new(big.Int).SetBytes(nb);if n.Sign()<=0||n.BitLen()<2048 { continue }
		keys[k.Kid]=&rsa.PublicKey{N:n,E:e}
	}
	if len(keys)==0{return nil,errors.New("OIDC key set contains no supported signing keys")}
	v.keys=keys;v.keysUntil=time.Now().Add(5*time.Minute)
	return v.keys,nil
}

func audienceMatches(raw json.RawMessage, expected string) bool {
	var one string
	if json.Unmarshal(raw,&one)==nil{return one==expected}
	var many []string
	if json.Unmarshal(raw,&many)==nil { for _,a:=range many {if a==expected{return true}} }
	return false
}

func (v *Verifier) Verify(token string) (Principal,error) {
	var empty Principal
	parts:=strings.Split(token,".");if len(parts)!=3{return empty,errors.New("invalid bearer token")}
	hb,err:=base64.RawURLEncoding.DecodeString(parts[0]);if err!=nil{return empty,errors.New("invalid bearer token")}
	var header struct { Alg string `json:"alg"`; Kid string `json:"kid"` }
	if json.Unmarshal(hb,&header)!=nil||header.Alg!="RS256"||header.Kid=="" {return empty,errors.New("unsupported bearer token")}
	keys,err:=v.loadKeys(false);if err!=nil{return empty,err}
	key:=keys[header.Kid]
	if key==nil { keys,err=v.loadKeys(true);if err!=nil{return empty,err};key=keys[header.Kid] }
	if key==nil{return empty,errors.New("unknown OIDC signing key")}
	sig,err:=base64.RawURLEncoding.DecodeString(parts[2]);if err!=nil{return empty,errors.New("invalid bearer token")}
	sum:=sha256.Sum256([]byte(parts[0]+"."+parts[1]))
	if err=rsa.VerifyPKCS1v15(key,crypto.SHA256,sum[:],sig);err!=nil{return empty,errors.New("invalid bearer token signature")}
	pb,err:=base64.RawURLEncoding.DecodeString(parts[1]);if err!=nil{return empty,errors.New("invalid bearer token")}
	var c map[string]json.RawMessage
	if err=json.Unmarshal(pb,&c);err!=nil{return empty,errors.New("invalid bearer token claims")}
	var iss,sub,tenant string
	var exp,nbf int64
	if json.Unmarshal(c["iss"],&iss)!=nil||json.Unmarshal(c["sub"],&sub)!=nil||json.Unmarshal(c["exp"],&exp)!=nil {return empty,errors.New("required OIDC claims are missing")}
	if raw:=c[v.tenantClaim];len(raw)==0||json.Unmarshal(raw,&tenant)!=nil {return empty,errors.New("tenant claim is missing")}
	if iss!=v.issuer||sub==""||tenant==""||!audienceMatches(c["aud"],v.audience) {return empty,errors.New("OIDC claims do not match configured issuer, audience, or tenant")}
	now:=time.Now().Unix()
	if exp<=now-30 {return empty,errors.New("bearer token expired")}
	if raw:=c["nbf"];len(raw)>0 {if json.Unmarshal(raw,&nbf)!=nil||nbf>now+30{return empty,errors.New("bearer token is not yet valid")}}
	roles:=[]string{}
	if raw:=c[v.rolesClaim];len(raw)>0 {if json.Unmarshal(raw,&roles)!=nil{return empty,errors.New("roles claim must be an array of strings")}}
	return Principal{Subject:sub,TenantID:tenant,Roles:roles},nil
}
