package main

import (
 "bytes"
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "log"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "strconv"
 "sync"
 "time"
)

type Agent struct { ID string `json:"id"`; TenantID string `json:"tenant_id"`; CredentialHash string `json:"-"`; Revoked bool `json:"revoked"`; LastSeen time.Time `json:"last_seen,omitempty"`; ConfigVersion int `json:"config_version"`; AppliedVersion int `json:"applied_version"` }
type Token struct { TenantID string `json:"tenant_id"`; Expires time.Time `json:"expires"`; Used bool `json:"used"` }
type Policy struct { TenantID string `json:"tenant_id"`; Version int `json:"version"`; SchemaVersion string `json:"schema_version"`; Config json.RawMessage `json:"config"`; Digest string `json:"digest"`; CreatedAt time.Time `json:"created_at"` }
type State struct { Tenants map[string]bool `json:"tenants"`; Tokens map[string]Token `json:"tokens"`; Agents map[string]Agent `json:"agents"`; Policies map[string][]Policy `json:"policies"`; Audit []map[string]string `json:"audit"` }
type Server struct { mu sync.Mutex; path, adminHash string; state State }

func hash(v string) string { h:=sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func random(n int)(string,error){ b:=make([]byte,n); if _,e:=rand.Read(b);e!=nil{return "",e}; return hex.EncodeToString(b),nil }
func newServer(path, admin string)(*Server,error){
 if len(admin)<32{return nil,errors.New("SIGNALFORGE_ADMIN_TOKEN must contain at least 32 characters")}
 s:=&Server{path:path,adminHash:hash(admin),state:State{Tenants:map[string]bool{},Tokens:map[string]Token{},Agents:map[string]Agent{},Policies:map[string][]Policy{}}}
 b,e:=os.ReadFile(path); if e==nil {if e=json.Unmarshal(b,&s.state);e!=nil{return nil,e}} else if !errors.Is(e,os.ErrNotExist){return nil,e}
 if s.state.Tenants==nil{s.state.Tenants=map[string]bool{}};if s.state.Tokens==nil{s.state.Tokens=map[string]Token{}};if s.state.Agents==nil{s.state.Agents=map[string]Agent{}};if s.state.Policies==nil{s.state.Policies=map[string][]Policy{}}
 return s,nil
}
func (s *Server) persist()error{
 b,e:=json.MarshalIndent(s.state,"","  ");if e!=nil{return e};dir:=filepath.Dir(s.path);if e=os.MkdirAll(dir,0700);e!=nil{return e}
 f,e:=os.CreateTemp(dir,".state-*");if e!=nil{return e};name:=f.Name();defer os.Remove(name)
 if e=f.Chmod(0600);e==nil{_,e=f.Write(b)};if e==nil{e=f.Sync()};ce:=f.Close();if e==nil{e=ce};if e!=nil{return e};if e=os.Rename(name,s.path);e!=nil{return e}
 d,e:=os.Open(dir);if e==nil{_ = d.Sync();_ = d.Close()};return nil
}
func (s *Server) decode(w http.ResponseWriter,r *http.Request,v any)bool{
 r.Body=http.MaxBytesReader(w,r.Body,1<<20);d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 if d.Decode(v)!=nil{http.Error(w,"invalid JSON request",400);return false};var extra any;if d.Decode(&extra)!=io.EOF{http.Error(w,"expected exactly one JSON object",400);return false};return true
}
func (s *Server) adminAuth(r *http.Request)bool{v:=strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ");if v==""||v==r.Header.Get("Authorization"){return false};return subtle.ConstantTimeCompare([]byte(hash(v)),[]byte(s.adminHash))==1}
func (s *Server) agentAuth(r *http.Request)(Agent,bool){v:=strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ");if v==""||v==r.Header.Get("Authorization"){return Agent{},false};h:=hash(v);for _,a:=range s.state.Agents{if a.CredentialHash!=""&&subtle.ConstantTimeCompare([]byte(h),[]byte(a.CredentialHash))==1&&!a.Revoked{return a,true}};return Agent{},false}
func (s *Server) ServeHTTP(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Content-Type","application/json")
 if r.URL.Path=="/healthz"&&r.Method=="GET"{_ = json.NewEncoder(w).Encode(map[string]string{"status":"healthy"});return}
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
 var compact bytes.Buffer
 if e:=json.Compact(&compact,q.Config);e!=nil{http.Error(w,"invalid config JSON",422);return}
 q.Config=append(json.RawMessage(nil),compact.Bytes()...)
 s.mu.Lock();defer s.mu.Unlock();if !s.state.Tenants[q.TenantID]{http.NotFound(w,r);return};ps:=s.state.Policies[q.TenantID];v:=1;if len(ps)>0{v=ps[len(ps)-1].Version+1};d:=sha256.Sum256(q.Config);p:=Policy{TenantID:q.TenantID,Version:v,SchemaVersion:q.SchemaVersion,Config:append(json.RawMessage(nil),q.Config...),Digest:hex.EncodeToString(d[:]),CreatedAt:time.Now().UTC()};s.state.Policies[q.TenantID]=append(ps,p);for id,a:=range s.state.Agents{if a.TenantID==q.TenantID&&!a.Revoked{a.ConfigVersion=v;s.state.Agents[id]=a}};s.state.Audit=append(s.state.Audit,map[string]string{"action":"policy.create","tenant_id":q.TenantID,"version":strconv.Itoa(v),"at":p.CreatedAt.Format(time.RFC3339Nano)});if e:=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};w.WriteHeader(201);_ = json.NewEncoder(w).Encode(p)
}
func (s *Server) revoke(w http.ResponseWriter,r *http.Request){id:=strings.TrimSuffix(strings.TrimPrefix(r.URL.Path,"/api/v1/admin/agents/"),"/revoke");s.mu.Lock();defer s.mu.Unlock();a,ok:=s.state.Agents[id];if !ok{http.NotFound(w,r);return};a.Revoked=true;a.CredentialHash="";s.state.Agents[id]=a;s.state.Audit=append(s.state.Audit,map[string]string{"action":"agent.revoke","agent_id":id,"at":time.Now().UTC().Format(time.RFC3339Nano)});if e:=s.persist();e!=nil{http.Error(w,"persistence failure",500);return};w.WriteHeader(204)}
func main(){path:=os.Getenv("SIGNALFORGE_MANAGEMENT_STATE");if path==""{path="data/management-state.json"};s,e:=newServer(path,os.Getenv("SIGNALFORGE_ADMIN_TOKEN"));if e!=nil{log.Fatal(e)};addr:=os.Getenv("SIGNALFORGE_MANAGEMENT_ADDR");if addr==""{addr="127.0.0.1:8081"};srv:=&http.Server{Addr:addr,Handler:s,ReadHeaderTimeout:5*time.Second,IdleTimeout:60*time.Second};log.Printf("[signalforge-management] listening on %s",addr);log.Fatal(srv.ListenAndServe())}
