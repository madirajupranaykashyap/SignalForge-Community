package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
		"strconv"
	"strings"
	"syscall"
	"sync"
	"time"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/oidcauth"
	"github.com/madirajupranaykashyap/SignalForge-Community/internal/managementstore"
)

type Agent struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	CredentialHash string `json:"-"`
	Revoked bool `json:"revoked"`
	LastSeen time.Time `json:"last_seen,omitempty"`
	ConfigVersion int `json:"config_version"`
	AppliedVersion int `json:"applied_version"`
}
type Token struct { TenantID string `json:"tenant_id"`; Expires time.Time `json:"expires"`; Used bool `json:"used"` }
type Policy struct {
	TenantID string `json:"tenant_id"`
	Version int `json:"version"`
	SchemaVersion string `json:"schema_version"`
	Config json.RawMessage `json:"config"`
	Digest string `json:"digest"`
	CreatedAt time.Time `json:"created_at"`
}
type State struct {
	Tenants map[string]bool `json:"tenants"`
	Tokens map[string]Token `json:"tokens"`
	Agents map[string]Agent `json:"agents"`
	Policies map[string][]Policy `json:"policies"`
	Audit []map[string]string `json:"audit"`
}
type Server struct {
	mu sync.Mutex
	requestMu sync.Mutex
	adminHash string
	state State
	store managementstore.Persistence
	tx managementstore.Transaction
	txErr error
	oidc *oidcauth.Verifier
	limiter *rateLimiter
}

func hash(v string) string { h:=sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func random(n int)(string,error){ b:=make([]byte,n); if _,e:=rand.Read(b);e!=nil{return "",e}; return hex.EncodeToString(b),nil }
func envOrFile(name string) (string, error) {
	value := os.Getenv(name)
	file := strings.TrimSpace(os.Getenv(name + "_FILE"))
	if value != "" && file != "" {
		return "", fmt.Errorf("configure only one of %s or %s_FILE", name, name)
	}
	if file == "" {
		return value, nil
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	value = strings.TrimRight(string(raw), "\r\n")
	if value == "" {
		return "", fmt.Errorf("%s_FILE is empty", name)
	}
	return value, nil
}

func emptyState() State {
 return State{Tenants:map[string]bool{},Tokens:map[string]Token{},Agents:map[string]Agent{},Policies:map[string][]Policy{},Audit:[]map[string]string{}}
}
func newServer(path,admin string)(*Server,error) {
 if len(admin)<32 { return nil,errors.New("SIGNALFORGE_ADMIN_TOKEN must contain at least 32 characters") }
 initial,err:=json.Marshal(emptyState()); if err!=nil { return nil,err }
 store,err:=managementstore.Open(path,initial); if err!=nil { return nil,err }
 return newServerWithStore(store,admin)
}
func newServerWithStore(store managementstore.Persistence,admin string)(*Server,error) {
 if len(admin)<32 { _=store.Close(); return nil,errors.New("SIGNALFORGE_ADMIN_TOKEN must contain at least 32 characters") }
 return &Server{adminHash:hash(admin),store:store,state:emptyState(),limiter:newRateLimiter(defaultRateLimitRPM)},nil
}
func (s *Server) persist()error{
	if s.tx==nil{return errors.New("management transaction is not active")}
	b,e:=json.Marshal(s.state);if e!=nil{return e}
	if e=s.tx.Save(b);e!=nil{s.txErr=e;return e}
	return nil
}
func (s *Server) decode(w http.ResponseWriter,r *http.Request,v any)bool{
	r.Body=http.MaxBytesReader(w,r.Body,1<<20);d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
	if d.Decode(v)!=nil{http.Error(w,"invalid JSON request",400);return false};var extra any;if d.Decode(&extra)!=io.EOF{http.Error(w,"expected exactly one JSON object",400);return false};return true
}
func (s *Server) adminAuth(r *http.Request)bool{v:=strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ");if v==""||v==r.Header.Get("Authorization"){return false};return subtle.ConstantTimeCompare([]byte(hash(v)),[]byte(s.adminHash))==1}
func (s *Server) agentAuth(r *http.Request)(Agent,bool){v:=strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ");if v==""||v==r.Header.Get("Authorization"){return Agent{},false};h:=hash(v);for _,a:=range s.state.Agents{if a.CredentialHash!=""&&subtle.ConstantTimeCompare([]byte(h),[]byte(a.CredentialHash))==1&&!a.Revoked{return a,true}};return Agent{},false}
func (s *Server) serveHTTPCore(w http.ResponseWriter,r *http.Request){
	if r.URL.Path=="/healthz"&&r.Method=="GET"{w.Header().Set("Content-Type","application/json");_ = json.NewEncoder(w).Encode(map[string]string{"status":"healthy"});return}
	// A write-reserving SQLite transaction spans load -> handler -> save, so
	// concurrent processes cannot overwrite state from a stale in-memory copy.
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	tx,err:=s.store.Begin(r.Context())
	if err!=nil{http.Error(w,"management storage unavailable",http.StatusServiceUnavailable);return}
	payload,err:=tx.Load()
	if err!=nil{_ = tx.Rollback();http.Error(w,"management storage unavailable",http.StatusServiceUnavailable);return}
	var fresh State
	if err=json.Unmarshal(payload,&fresh);err!=nil{_ = tx.Rollback();http.Error(w,"management storage is corrupt",http.StatusInternalServerError);return}
	if fresh.Tenants==nil{fresh.Tenants=map[string]bool{}}
	if fresh.Tokens==nil{fresh.Tokens=map[string]Token{}}
	if fresh.Agents==nil{fresh.Agents=map[string]Agent{}}
	if fresh.Policies==nil{fresh.Policies=map[string][]Policy{}}
	if fresh.Audit==nil{fresh.Audit=[]map[string]string{}}
	s.mu.Lock();s.state=fresh;s.mu.Unlock()
	s.tx=tx;s.txErr=nil
	recorder:=httptest.NewRecorder()
	s.dispatchHTTP(recorder,r)
	if s.txErr!=nil{_ = tx.Rollback();s.tx=nil;http.Error(w,"management persistence failure",http.StatusInternalServerError);return}
	if err=tx.Commit();err!=nil{_ = tx.Rollback();s.tx=nil;http.Error(w,"management persistence failure",http.StatusInternalServerError);return}
	s.tx=nil
	for k,values:=range recorder.Header(){w.Header().Del(k);for _,v:=range values{w.Header().Add(k,v)}}
	w.WriteHeader(recorder.Code)
	_,_=w.Write(recorder.Body.Bytes())
}
func (s *Server) dispatchHTTP(w http.ResponseWriter,r *http.Request){
	if strings.HasPrefix(r.URL.Path,"/api/v1/tenants/"){s.tenantAPI(w,r);return}
	if r.Method=="POST"&&r.URL.Path=="/api/v1/agents/enroll"{s.enroll(w,r);return}
	if r.Method=="POST"&&r.URL.Path=="/api/v1/agents/heartbeat"{s.heartbeat(w,r);return}
	if r.Method=="GET"&&r.URL.Path=="/api/v1/agents/config"{s.getConfig(w,r);return}
	if r.Method=="POST"&&r.URL.Path=="/api/v1/agents/config/ack"{s.ack(w,r);return}
	if !s.adminAuth(r){http.Error(w,"unauthorized",401);return}
	switch {
	case r.Method=="POST"&&r.URL.Path=="/api/v1/admin/tenants":s.createTenant(w,r)
	case r.Method=="POST"&&r.URL.Path=="/api/v1/admin/enrollment-tokens":s.createToken(w,r)
	case r.Method=="POST"&&r.URL.Path=="/api/v1/admin/policies":s.createPolicy(w,r)
	case r.Method=="POST"&&strings.HasPrefix(r.URL.Path,"/api/v1/admin/agents/")&&strings.HasSuffix(r.URL.Path,"/revoke"):s.revoke(w,r)
	default:http.NotFound(w,r)
	}
}
func (s *Server) createTenant(w http.ResponseWriter,r *http.Request){var q struct{ID string `json:"tenant_id"`};if !s.decode(w,r,&q){return};if q.ID==""||len(q.ID)>100||strings.ContainsAny(q.ID,"/\\ "){http.Error(w,"invalid tenant_id",400);return}
	s.mu.Lock();defer s.mu.Unlock();if s.state.Tenants[q.ID]{http.Error(w,"tenant exists",409);return};s.state.Tenants[q.ID]=true;s.state.Audit=append(s.state.Audit,map[string]string{"action":"tenant.create","tenant_id":q.ID,"at":time.Now().UTC().Format(time.RFC3339Nano)});if e:=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};w.WriteHeader(201);_ = json.NewEncoder(w).Encode(map[string]string{"tenant_id":q.ID})
}
func (s *Server) createToken(w http.ResponseWriter,r *http.Request){var q struct{TenantID string `json:"tenant_id"`;TTL int `json:"ttl_seconds"`};if !s.decode(w,r,&q){return};if q.TTL<60||q.TTL>86400{http.Error(w,"ttl_seconds must be between 60 and 86400",400);return}
	s.mu.Lock();defer s.mu.Unlock();if !s.state.Tenants[q.TenantID]{http.NotFound(w,r);return};v,e:=random(32);if e!=nil{http.Error(w,"random generation failed",500);return};expires:=time.Now().UTC().Add(time.Duration(q.TTL)*time.Second);s.state.Tokens[hash(v)]=Token{TenantID:q.TenantID,Expires:expires};if e=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};w.WriteHeader(201);_ = json.NewEncoder(w).Encode(map[string]any{"token":v,"expires_at":expires})
}
func (s *Server) enroll(w http.ResponseWriter,r *http.Request){var q struct{Token string `json:"token"`};if !s.decode(w,r,&q){return};s.mu.Lock();defer s.mu.Unlock();k:=hash(q.Token);t,ok:=s.state.Tokens[k];if !ok||t.Used||!time.Now().Before(t.Expires){http.Error(w,"enrollment token invalid, expired, or used",410);return}
	id,e:=random(16);if e!=nil{http.Error(w,"random generation failed",500);return};cred,e:=random(32);if e!=nil{http.Error(w,"random generation failed",500);return};t.Used=true;s.state.Tokens[k]=t;s.state.Agents[id]=Agent{ID:id,TenantID:t.TenantID,CredentialHash:hash(cred)};s.state.Audit=append(s.state.Audit,map[string]string{"action":"agent.enroll","tenant_id":t.TenantID,"agent_id":id,"at":time.Now().UTC().Format(time.RFC3339Nano)});if e=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};w.WriteHeader(201);_ = json.NewEncoder(w).Encode(map[string]string{"agent_id":id,"credential":cred})
}
func (s *Server) heartbeat(w http.ResponseWriter,r *http.Request){s.mu.Lock();defer s.mu.Unlock();a,ok:=s.agentAuth(r);if !ok{http.Error(w,"unauthorized",401);return};var q struct{Version string `json:"version"`;AppliedVersion int `json:"applied_version"`};if !s.decode(w,r,&q){return};a.LastSeen=time.Now().UTC();if q.AppliedVersion>=0&&q.AppliedVersion<=a.ConfigVersion{a.AppliedVersion=q.AppliedVersion};s.state.Agents[a.ID]=a;if e:=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};_ = json.NewEncoder(w).Encode(map[string]any{"agent_id":a.ID,"config_version":a.ConfigVersion,"server_time":a.LastSeen})
}
func (s *Server) getConfig(w http.ResponseWriter,r *http.Request){s.mu.Lock();defer s.mu.Unlock();a,ok:=s.agentAuth(r);if !ok{http.Error(w,"unauthorized",401);return};ps:=s.state.Policies[a.TenantID];for i:=len(ps)-1;i>=0;i--{if ps[i].Version==a.ConfigVersion{_ = json.NewEncoder(w).Encode(ps[i]);return}};_ = json.NewEncoder(w).Encode(map[string]any{"config_version":a.ConfigVersion,"config":nil})
}
func (s *Server) ack(w http.ResponseWriter,r *http.Request){s.mu.Lock();defer s.mu.Unlock();a,ok:=s.agentAuth(r);if !ok{http.Error(w,"unauthorized",401);return};var q struct{Version int `json:"version"`;Applied bool `json:"applied"`;Message string `json:"message"`};if !s.decode(w,r,&q){return};if q.Version!=a.ConfigVersion{http.Error(w,"version conflict",409);return};if q.Applied{a.AppliedVersion=q.Version};s.state.Agents[a.ID]=a;s.state.Audit=append(s.state.Audit,map[string]string{"action":"config.ack","agent_id":a.ID,"version":strconv.Itoa(q.Version),"applied":map[bool]string{true:"true",false:"false"}[q.Applied],"message":q.Message});if e:=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};w.WriteHeader(204)
}
func (s *Server) createPolicy(w http.ResponseWriter,r *http.Request){var q struct{TenantID string `json:"tenant_id"`;SchemaVersion string `json:"schema_version"`;Config json.RawMessage `json:"config"`};if !s.decode(w,r,&q){return};if q.SchemaVersion==""||len(q.Config)==0||!json.Valid(q.Config){http.Error(w,"invalid schema_version or config",422);return}
	var compact bytes.Buffer;if e:=json.Compact(&compact,q.Config);e!=nil{http.Error(w,"invalid config JSON",422);return};q.Config=append(json.RawMessage(nil),compact.Bytes()...)
	s.mu.Lock();defer s.mu.Unlock();if !s.state.Tenants[q.TenantID]{http.NotFound(w,r);return};ps:=s.state.Policies[q.TenantID];v:=1;if len(ps)>0{v=ps[len(ps)-1].Version+1};d:=sha256.Sum256(q.Config);p:=Policy{TenantID:q.TenantID,Version:v,SchemaVersion:q.SchemaVersion,Config:append(json.RawMessage(nil),q.Config...),Digest:hex.EncodeToString(d[:]),CreatedAt:time.Now().UTC()};s.state.Policies[q.TenantID]=append(ps,p);for id,a:=range s.state.Agents{if a.TenantID==q.TenantID&&!a.Revoked{a.ConfigVersion=v;s.state.Agents[id]=a}};s.state.Audit=append(s.state.Audit,map[string]string{"action":"policy.create","tenant_id":q.TenantID,"version":strconv.Itoa(v),"at":p.CreatedAt.Format(time.RFC3339Nano)});if e:=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};w.WriteHeader(201);_ = json.NewEncoder(w).Encode(p)
}
func (s *Server) revoke(w http.ResponseWriter,r *http.Request){id:=strings.TrimSuffix(strings.TrimPrefix(r.URL.Path,"/api/v1/admin/agents/"),"/revoke");s.mu.Lock();defer s.mu.Unlock();a,ok:=s.state.Agents[id];if !ok{http.NotFound(w,r);return};a.Revoked=true;a.CredentialHash="";s.state.Agents[id]=a;s.state.Audit=append(s.state.Audit,map[string]string{"action":"agent.revoke","agent_id":id,"at":time.Now().UTC().Format(time.RFC3339Nano)});if e:=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};w.WriteHeader(204)
}

func main(){
	path:=os.Getenv("SIGNALFORGE_MANAGEMENT_DB");if path==""{path="data/management.db"}
	admin,secretErr:=envOrFile("SIGNALFORGE_ADMIN_TOKEN"); if secretErr!=nil { log.Fatal(secretErr) }
	databaseURL,secretErr:=envOrFile("SIGNALFORGE_MANAGEMENT_DATABASE_URL"); if secretErr!=nil { log.Fatal(secretErr) }
	var s *Server
	var e error
	if databaseURL=strings.TrimSpace(databaseURL);databaseURL!="" {
		initial,marshalErr:=json.Marshal(emptyState()); if marshalErr!=nil { log.Fatal(marshalErr) }
		store,openErr:=managementstore.OpenPostgres(databaseURL,initial); if openErr!=nil { log.Fatal(openErr) }
		s,e=newServerWithStore(store,admin)
	} else { s,e=newServer(path,admin) }
	if e!=nil{log.Fatal(e)}
	issuer,audience,jwks:=os.Getenv("SIGNALFORGE_OIDC_ISSUER"),os.Getenv("SIGNALFORGE_OIDC_AUDIENCE"),os.Getenv("SIGNALFORGE_OIDC_JWKS_URL")
	if issuer!=""||audience!=""||jwks!="" {
		s.oidc,e=oidcauth.New(oidcauth.Config{Issuer:issuer,Audience:audience,JWKSURL:jwks,TenantClaim:os.Getenv("SIGNALFORGE_OIDC_TENANT_CLAIM"),RolesClaim:os.Getenv("SIGNALFORGE_OIDC_ROLES_CLAIM")})
		if e!=nil{_ = s.store.Close();log.Fatal(e)}
	}
	if raw:=os.Getenv("SIGNALFORGE_RATE_LIMIT_RPM");raw!=""{
		rpm,parseErr:=strconv.Atoi(raw)
		if parseErr!=nil||rpm<1||rpm>10000{_ = s.store.Close();log.Fatal("SIGNALFORGE_RATE_LIMIT_RPM must be an integer between 1 and 10000")}
		s.limiter=newRateLimiter(rpm)
	}
	addr:=os.Getenv("SIGNALFORGE_MANAGEMENT_ADDR");if addr==""{addr="127.0.0.1:8081"}
	srv:=&http.Server{Addr:addr,Handler:s,ReadHeaderTimeout:5*time.Second,IdleTimeout:60*time.Second}
	ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
	defer stop()
	go func(){<-ctx.Done();shutdownCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel();_ = srv.Shutdown(shutdownCtx)}()
	log.Printf("[signalforge-management] listening on %s",addr)
	err:=srv.ListenAndServe()
	stop()
	_ = s.store.Close()
	if err!=nil&&!errors.Is(err,http.ErrServerClosed){log.Printf("[signalforge-management] server stopped: %v",err);os.Exit(1)}
}
