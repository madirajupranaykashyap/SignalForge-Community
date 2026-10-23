package collectorhost

import (
	"context"
	"encoding/json"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/configcache"
)

type fakeEngine struct {
	path string
	current string
	failRestart bool
	failHealth bool
	restarts int
}
func (f *fakeEngine) Validate(_ context.Context,b []byte)error{
	if !json.Valid(b){return errors.New("invalid JSON")}
	if strings.Contains(string(b),"invalid-schema"){return errors.New("schema invalid")}
	return nil
}
func (f *fakeEngine) Restart(_ context.Context,path string)error{
	f.restarts++;f.path=path;b,err:=os.ReadFile(path);if err!=nil{return err};f.current=string(b)
	if strings.Contains(f.current,"restart-fails")||f.failRestart{return errors.New("simulated restart failure")}
	return nil
}
func (f *fakeEngine) Healthy(context.Context)error{if f.failHealth||strings.Contains(f.current,"health-fails"){return errors.New("simulated health failure")};return nil}
func (f *fakeEngine) Stop(context.Context)error{return nil}
func candidate(version int,payload string)configcache.Config{return configcache.Config{Version:version,SchemaVersion:"1",Digest:digest([]byte(payload)),Config:json.RawMessage(payload)}}
func digest(b []byte)string{c:=sha256.Sum256(b);return hex.EncodeToString(c[:])}

func TestActivationAndFailedRestartRestorePreviousConfig(t *testing.T){
	path:=filepath.Join(t.TempDir(),"active.yml")
	old:=`{"service":{"pipelines":{"logs/default":{"receivers":["otlp"],"exporters":["debug"]}}}}`
	if err:=os.WriteFile(path,[]byte(old),0600);err!=nil{t.Fatal(err)}
	engine:=&fakeEngine{}
	r,err:=New(path,engine);if err!=nil{t.Fatal(err)}
	ctx:=context.Background()
	next:=`{"service":{"pipelines":{"logs/default":{"receivers":["otlp"],"exporters":["debug"]}}},"extensions":{"health_check":{}}}`
	if err=r.Validate(ctx,"1",json.RawMessage(next));err!=nil{t.Fatal(err)}
	if err=r.Activate(ctx,candidate(2,next));err!=nil{t.Fatal(err)}
	b,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)};if string(b)!=next{t.Fatalf("active config mismatch: %s",b)}
	bad:=`{"service":"restart-fails"}`
	if err=r.Activate(ctx,candidate(3,bad));err==nil{t.Fatal("expected simulated restart failure")}
	b,err=os.ReadFile(path);if err!=nil{t.Fatal(err)}
	if string(b)!=next{t.Fatalf("failed activation did not restore previous config: %s",b)}
	if engine.current!=next{t.Fatalf("previous pipeline was not restored: %s",engine.current)}
	if engine.restarts<3{t.Fatalf("expected candidate restart and rollback, got %d restarts",engine.restarts)}
}
func TestInvalidSchemaNeverChangesActiveConfig(t *testing.T){
	path:=filepath.Join(t.TempDir(),"active.yml")
	old:=`{"service":{"pipelines":{}}}`
	if err:=os.WriteFile(path,[]byte(old),0600);err!=nil{t.Fatal(err)}
	engine:=&fakeEngine{}
	r,_:=New(path,engine)
	bad:=`{"invalid-schema":true}`
	if err:=r.Validate(context.Background(),"1",json.RawMessage(bad));err==nil{t.Fatal("invalid config accepted")}
	b,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
	if string(b)!=old{t.Fatal("validation failure changed active config")}
	if engine.restarts!=0{t.Fatal("validation failure restarted Collector")}
}
func TestUnsupportedSchemaVersionRejected(t *testing.T){
	r,_:=New(filepath.Join(t.TempDir(),"active.yml"),&fakeEngine{})
	if err:=r.Validate(context.Background(),"999",json.RawMessage(`{}`));err==nil{t.Fatal("unsupported schema accepted")}
}


func TestHealthFailureRestoresPreviousConfig(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"active.yml")
	old:=`{"service":{"pipelines":{}}}`
	if err:=os.WriteFile(path,[]byte(old),0600);err!=nil{t.Fatal(err)}
	engine:=&fakeEngine{}
	r,err:=New(path,engine);if err!=nil{t.Fatal(err)}
	candidateConfig:=`{"service":"health-fails"}`
	err=r.Activate(context.Background(),candidate(2,candidateConfig))
	if err==nil{t.Fatal("expected health-check failure")}
	b,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
	if string(b)!=old{t.Fatalf("health failure did not restore config: %s",b)}
	if engine.current!=old{t.Fatalf("previous process was not restored: %s",engine.current)}
}
