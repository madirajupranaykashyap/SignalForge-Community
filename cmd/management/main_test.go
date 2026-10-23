package main

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
)
func TestEnrollmentTokenSingleUseAndPersistence(t *testing.T){
 s,e:=newServer(filepath.Join(t.TempDir(),"state.json"),strings.Repeat("a",40));if e!=nil{t.Fatal(e)}
 call:=func(method,path,token,body string)*httptest.ResponseRecorder{r:=httptest.NewRequest(method,path,bytes.NewBufferString(body));if token!=""{r.Header.Set("Authorization","Bearer "+token)};w:=httptest.NewRecorder();s.ServeHTTP(w,r);return w}
 admin:=strings.Repeat("a",40)
 if w:=call("POST","/api/v1/admin/tenants",admin,`{"tenant_id":"t1"}`);w.Code!=201{t.Fatalf("tenant: %d %s",w.Code,w.Body)}
 w:=call("POST","/api/v1/admin/enrollment-tokens",admin,`{"tenant_id":"t1","ttl_seconds":300}`);if w.Code!=201{t.Fatalf("token: %d %s",w.Code,w.Body)}
 var tok struct{Token string `json:"token"`};if e=json.Unmarshal(w.Body.Bytes(),&tok);e!=nil{t.Fatal(e)}
 body,_:=json.Marshal(map[string]string{"token":tok.Token})
 if w=call("POST","/api/v1/agents/enroll","",string(body));w.Code!=201{t.Fatalf("enroll: %d %s",w.Code,w.Body)}
 if w=call("POST","/api/v1/agents/enroll","",string(body));w.Code!=410{t.Fatalf("replay accepted: %d",w.Code)}
}
func TestMissingAdminSecretAndUnauthorizedAdminFailClosed(t *testing.T){
 if _,e:=newServer(filepath.Join(t.TempDir(),"s.json"),"short");e==nil{t.Fatal("short admin token accepted")}
 s,e:=newServer(filepath.Join(t.TempDir(),"s.json"),strings.Repeat("b",40));if e!=nil{t.Fatal(e)}
 r:=httptest.NewRequest(http.MethodPost,"/api/v1/admin/tenants",bytes.NewBufferString(`{"tenant_id":"x"}`));w:=httptest.NewRecorder();s.ServeHTTP(w,r);if w.Code!=401{t.Fatalf("got %d",w.Code)}
}
