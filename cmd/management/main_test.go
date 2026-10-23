package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func managementCall(s *Server,method,path,token,body string)*httptest.ResponseRecorder{
	r:=httptest.NewRequest(method,path,bytes.NewBufferString(body))
	if token!=""{r.Header.Set("Authorization","Bearer "+token)}
	w:=httptest.NewRecorder();s.ServeHTTP(w,r);return w
}
func TestEnrollmentTokenSingleUseSurvivesServerRestart(t *testing.T){
	path:=filepath.Join(t.TempDir(),"management.db")
	admin:=strings.Repeat("a",40)
	s,err:=newServer(path,admin);if err!=nil{t.Fatal(err)}
	if w:=managementCall(s,"POST","/api/v1/admin/tenants",admin,`{"tenant_id":"t1"}`);w.Code!=201{t.Fatalf("tenant: %d %s",w.Code,w.Body)}
	w:=managementCall(s,"POST","/api/v1/admin/enrollment-tokens",admin,`{"tenant_id":"t1","ttl_seconds":300}`)
	if w.Code!=201{t.Fatalf("token: %d %s",w.Code,w.Body)}
	var tok struct{Token string `json:"token"`}
	if err=json.Unmarshal(w.Body.Bytes(),&tok);err!=nil{t.Fatal(err)}
	body,_:=json.Marshal(map[string]string{"token":tok.Token})
	if w=managementCall(s,"POST","/api/v1/agents/enroll","",string(body));w.Code!=201{t.Fatalf("enroll: %d %s",w.Code,w.Body)}
	if w=managementCall(s,"POST","/api/v1/agents/enroll","",string(body));w.Code!=410{t.Fatalf("replay accepted: %d",w.Code)}
	if err=s.store.Close();err!=nil{t.Fatal(err)}
	s2,err:=newServer(path,admin);if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = s2.store.Close()})
	if w=managementCall(s2,"POST","/api/v1/agents/enroll","",string(body));w.Code!=410{t.Fatalf("replay after restart accepted: %d %s",w.Code,w.Body)}
}
func TestMissingAdminSecretAndUnauthorizedAdminFailClosed(t *testing.T){
	if _,err:=newServer(filepath.Join(t.TempDir(),"s.db"),"short");err==nil{t.Fatal("short admin token accepted")}
	s,err:=newServer(filepath.Join(t.TempDir(),"s.db"),strings.Repeat("b",40));if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = s.store.Close()})
	w:=managementCall(s,http.MethodPost,"/api/v1/admin/tenants","",`{"tenant_id":"x"}`)
	if w.Code!=401{t.Fatalf("got %d",w.Code)}
}
func TestPolicyDigestMatchesCanonicalResponsePayload(t *testing.T){
	s,err:=newServer(filepath.Join(t.TempDir(),"state.db"),strings.Repeat("z",40));if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = s.store.Close()})
	admin:=strings.Repeat("z",40)
	if w:=managementCall(s,http.MethodPost,"/api/v1/admin/tenants",admin,`{"tenant_id":"digest-test"}`);w.Code!=http.StatusCreated{t.Fatalf("create tenant: %d %s",w.Code,w.Body)}
	w:=managementCall(s,http.MethodPost,"/api/v1/admin/policies",admin,`{"tenant_id":"digest-test","schema_version":"1","config":{ "service" : { "pipelines" : {} } }}`)
	if w.Code!=http.StatusCreated{t.Fatalf("create policy: %d %s",w.Code,w.Body)}
	var response struct{Digest string `json:"digest"`;Config json.RawMessage `json:"config"`}
	if err=json.Unmarshal(w.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
	sum:=sha256.Sum256(response.Config)
	if response.Digest!=hex.EncodeToString(sum[:]){t.Fatalf("digest %s does not match returned payload %s",response.Digest,response.Config)}
}

func TestEnvOrFileReadsSecretFileAndRejectsAmbiguousConfiguration(t *testing.T) {
	const name = "SIGNALFORGE_TEST_SECRET"
	t.Setenv(name, "")
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("secret-value\r\n"), 0600); err != nil { t.Fatal(err) }
	t.Setenv(name+"_FILE", path)
	got, err := envOrFile(name)
	if err != nil { t.Fatal(err) }
	if got != "secret-value" { t.Fatalf("secret file value = %q", got) }

	t.Setenv(name, "direct-value")
	if _, err := envOrFile(name); err == nil { t.Fatal("accepted both direct secret and secret file") }
}
