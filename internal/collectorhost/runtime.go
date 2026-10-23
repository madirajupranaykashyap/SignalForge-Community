package collectorhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/configcache"
)

// Engine adapts a Collector process to configuration validation and activation.
type Engine interface {
	Validate(context.Context, []byte) error
	Restart(context.Context, string) error
	Healthy(context.Context) error
	Stop(context.Context) error
}

type Runtime struct {
	mu sync.Mutex
	path string
	engine Engine
	activeDigest string
}

func New(path string, engine Engine) (*Runtime,error) {
	if strings.TrimSpace(path)=="" { return nil,errors.New("active Collector config path is required") }
	if engine==nil { return nil,errors.New("Collector engine is required") }
	return &Runtime{path:path,engine:engine},nil
}

// Validate performs syntax and distribution-specific validation without
// changing the active pipeline.
func (r *Runtime) Validate(ctx context.Context, schemaVersion string, payload json.RawMessage) error {
	if schemaVersion!="1" && schemaVersion!="otelcol-v1" {
		return fmt.Errorf("unsupported Collector config schema version %q",schemaVersion)
	}
	if len(payload)==0 || !json.Valid(payload) {
		return errors.New("Collector config must be a JSON/YAML-compatible object")
	}
	return r.engine.Validate(ctx,append([]byte(nil),payload...))
}

// Activate writes a candidate beside the active config, restarts the Collector,
// and waits for its health endpoint. Any failure restores the previous file and
// restarts the previous pipeline before returning an error.
func (r *Runtime) Activate(ctx context.Context, c configcache.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.Version<1 || c.Digest=="" || len(c.Config)==0 || !json.Valid(c.Config) {
		return errors.New("invalid configuration envelope")
	}
	sum:=sha256.Sum256(c.Config)
	if hex.EncodeToString(sum[:])!=c.Digest { return configcache.ErrDigestMismatch }
	if r.activeDigest==c.Digest {
		if err:=r.engine.Healthy(ctx);err==nil{return nil}
	}
	old,oldErr:=os.ReadFile(r.path)
	hadOld:=oldErr==nil
	if oldErr!=nil && !errors.Is(oldErr,os.ErrNotExist) { return fmt.Errorf("read current Collector config: %w",oldErr) }
	if err:=atomicWrite(r.path,c.Config);err!=nil{return fmt.Errorf("stage active Collector config: %w",err)}
	activationErr:=r.engine.Restart(ctx,r.path)
	if activationErr==nil { activationErr=r.engine.Healthy(ctx) }
	if activationErr!=nil {
		var rollbackErr error
		if hadOld {
			if e:=atomicWrite(r.path,old);e!=nil { rollbackErr=fmt.Errorf("restore previous config: %w",e) } else if e=r.engine.Restart(ctx,r.path);e!=nil { rollbackErr=fmt.Errorf("restart previous config: %w",e) } else if e=r.engine.Healthy(ctx);e!=nil { rollbackErr=fmt.Errorf("previous config failed health check: %w",e) }
		} else {
			_ = os.Remove(r.path)
			rollbackErr=r.engine.Stop(ctx)
		}
		if rollbackErr!=nil{return fmt.Errorf("activate candidate: %v; rollback also failed: %w",activationErr,rollbackErr)}
		return fmt.Errorf("candidate Collector config failed activation; previous config restored: %w",activationErr)
	}
	r.activeDigest=c.Digest
	return nil
}

func (r *Runtime) StartDefault(ctx context.Context) error {
	r.mu.Lock();defer r.mu.Unlock()
	if _,err:=os.Stat(r.path);err!=nil{return fmt.Errorf("default Collector config is unavailable: %w",err)}
	if err:=r.engine.Restart(ctx,r.path);err!=nil{return err}
	if err:=r.engine.Healthy(ctx);err!=nil{_ = r.engine.Stop(ctx);return err}
	return nil
}
func (r *Runtime) Stop(ctx context.Context)error{return r.engine.Stop(ctx)}

func atomicWrite(path string,payload []byte)error{
	dir:=filepath.Dir(path)
	if err:=os.MkdirAll(dir,0700);err!=nil{return err}
	f,err:=os.CreateTemp(dir,".signalforge-config-*.yaml");if err!=nil{return err}
	name:=f.Name();defer os.Remove(name)
	if err=f.Chmod(0600);err==nil{_,err=f.Write(payload)}
	if err==nil{err=f.Sync()}
	ce:=f.Close();if err==nil{err=ce};if err!=nil{return err}
	if err=os.Rename(name,path);err!=nil{return err}
	if d,e:=os.Open(dir);e==nil{_ = d.Sync();_ = d.Close()}
	return nil
}
