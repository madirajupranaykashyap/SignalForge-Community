package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/madirajupranaykashyap/SignalForge-Community/internal/collectorhost"
	"github.com/madirajupranaykashyap/SignalForge-Community/internal/configcache"
	"github.com/madirajupranaykashyap/SignalForge-Community/internal/configclient"
)

type collectorEngine struct {
	root context.Context
	binary string
	healthURL string
	httpClient *http.Client
	mu sync.Mutex
	cmd *exec.Cmd
	exited chan struct{}
	exitErr error
}

func newCollectorEngine(ctx context.Context,binary,healthURL string)*collectorEngine{
	return &collectorEngine{root:ctx,binary:binary,healthURL:healthURL,httpClient:&http.Client{Timeout:1200*time.Millisecond}}
}
func (e *collectorEngine) Validate(ctx context.Context,payload []byte)error{
	ctx,cancel:=context.WithTimeout(ctx,15*time.Second);defer cancel()
	dir,err:=os.MkdirTemp("","signalforge-validate-*");if err!=nil{return err};defer os.RemoveAll(dir)
	path:=filepath.Join(dir,"candidate.yaml")
	if err=os.WriteFile(path,payload,0600);err!=nil{return err}
	cmd:=exec.CommandContext(ctx,e.binary,"validate","--config="+path)
	if output,err:=cmd.CombinedOutput();err!=nil{
		_ = output // Collector output may include sensitive config values; don't expose it.
		return fmt.Errorf("Collector validation command failed: %w",err)
	}
	return nil
}
func (e *collectorEngine) Restart(ctx context.Context,path string)error{
	if err:=e.Stop(ctx);err!=nil{return err}
	cmd:=exec.CommandContext(e.root,e.binary,"--config="+path)
	cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr
	if err:=cmd.Start();err!=nil{return fmt.Errorf("start Collector: %w",err)}
	e.mu.Lock();e.cmd=cmd;e.exited=make(chan struct{});e.exitErr=nil;done:=e.exited;e.mu.Unlock()
	go func(){err:=cmd.Wait();e.mu.Lock();e.exitErr=err;e.mu.Unlock();close(done)}()
	return nil
}
func (e *collectorEngine) Healthy(ctx context.Context)error{
	ctx,cancel:=context.WithTimeout(ctx,20*time.Second);defer cancel()
	ticker:=time.NewTicker(250*time.Millisecond);defer ticker.Stop()
	var last error
	for{
		e.mu.Lock();done:=e.exited;cmd:=e.cmd;e.mu.Unlock()
		if cmd==nil{return errors.New("Collector process is not running")}
		select{case <-done:
			e.mu.Lock();err:=e.exitErr;e.mu.Unlock()
			if err==nil{return errors.New("Collector exited before becoming healthy")}
			return fmt.Errorf("Collector exited before becoming healthy: %w",err)
		default:}
		req,err:=http.NewRequestWithContext(ctx,http.MethodGet,e.healthURL,nil)
		if err==nil{
			resp,doErr:=e.httpClient.Do(req)
			if doErr==nil{_ = resp.Body.Close();if resp.StatusCode>=200&&resp.StatusCode<300{return nil};last=fmt.Errorf("Collector health endpoint returned HTTP %d",resp.StatusCode)}else{last=doErr}
		}else{last=err}
		select{case <-ctx.Done():if last!=nil{return fmt.Errorf("Collector health check timed out: %w",last)};return ctx.Err();case <-ticker.C:}
	}
}
func (e *collectorEngine) Stop(ctx context.Context)error{
	e.mu.Lock();cmd,done:=e.cmd,e.exited;e.mu.Unlock()
	if cmd==nil||cmd.Process==nil{return nil}
	select{case <-done:
		e.mu.Lock();e.cmd=nil;e.mu.Unlock();return nil
	default:}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	timer:=time.NewTimer(5*time.Second);defer timer.Stop()
	select{
	case <-done:
	case <-ctx.Done():
		_ = cmd.Process.Kill();<-done
	case <-timer.C:
		_ = cmd.Process.Kill();<-done
	}
	e.mu.Lock();e.cmd=nil;e.mu.Unlock()
	return nil
}

func main(){
	ctx,stop:=signal.NotifyContext(context.Background(),syscall.SIGINT,syscall.SIGTERM);defer stop()
	binary:=env("SIGNALFORGE_COLLECTOR_BIN","/usr/local/bin/signalforge-collector")
	defaultConfig:=env("SIGNALFORGE_DEFAULT_CONFIG","/etc/signalforge/collector.yml")
	activeConfig:=env("SIGNALFORGE_ACTIVE_CONFIG","/var/lib/signalforge/config/active.yml")
	if err:=ensureActiveConfig(defaultConfig,activeConfig);err!=nil{fatal(err)}
	engine:=newCollectorEngine(ctx,binary,env("SIGNALFORGE_COLLECTOR_HEALTH_URL","http://127.0.0.1:13133/"))
	runtime,err:=collectorhost.New(activeConfig,engine);if err!=nil{fatal(err)}
	endpoint,credential:=os.Getenv("SIGNALFORGE_CONFIG_ENDPOINT"),os.Getenv("SIGNALFORGE_AGENT_CREDENTIAL")
	if endpoint==""&&credential==""{
		if err=runtime.StartDefault(ctx);err!=nil{fatal(err)}
		<-ctx.Done();shutdown(engine);return
	}
	if endpoint==""||credential==""{fatal(errors.New("SIGNALFORGE_CONFIG_ENDPOINT and SIGNALFORGE_AGENT_CREDENTIAL must be configured together"))}
	cachePath:=env("SIGNALFORGE_CONFIG_CACHE","/var/lib/signalforge/config/last-known-good.json")
	cache,err:=configcache.New(cachePath);if err!=nil{fatal(err)}
	fetcher,err:=configclient.NewFetcher(endpoint,credential,nil);if err!=nil{fatal(err)}
	if _,ok:=cache.Active();!ok{
		if err=runtime.StartDefault(ctx);err!=nil{fatal(err)}
	}
	poller:=&configclient.Poller{Fetcher:fetcher,Cache:cache,Validate:func(schema string,payload json.RawMessage)error{validationCtx,cancel:=context.WithTimeout(ctx,20*time.Second);defer cancel();return runtime.Validate(validationCtx,schema,payload)},Activate:runtime.Activate,Interval:durationEnv("SIGNALFORGE_POLL_INTERVAL",30*time.Second),MinBackoff:time.Second,MaxBackoff:time.Minute}
	if err=poller.Run(ctx);err!=nil&&!errors.Is(err,context.Canceled){fatal(err)}
	shutdown(engine)
}
func ensureActiveConfig(source,destination string)error{
	if _,err:=os.Stat(destination);err==nil{return nil}else if !errors.Is(err,os.ErrNotExist){return err}
	b,err:=os.ReadFile(source);if err!=nil{return fmt.Errorf("read default Collector config: %w",err)}
	if err=os.MkdirAll(filepath.Dir(destination),0700);err!=nil{return err}
	tmp,err:=os.CreateTemp(filepath.Dir(destination),".initial-config-*");if err!=nil{return err}
	name:=tmp.Name();defer os.Remove(name)
	if err=tmp.Chmod(0600);err==nil{_,err=tmp.Write(b)};if err==nil{err=tmp.Sync()};ce:=tmp.Close();if err==nil{err=ce};if err!=nil{return err}
	if err=os.Rename(name,destination);err!=nil{return err};return nil
}
func shutdown(e *collectorEngine){ctx,cancel:=context.WithTimeout(context.Background(),6*time.Second);defer cancel();_ = e.Stop(ctx)}
func env(key,fallback string)string{if v:=strings.TrimSpace(os.Getenv(key));v!=""{return v};return fallback}
func durationEnv(key string,fallback time.Duration)time.Duration{raw:=strings.TrimSpace(os.Getenv(key));if raw==""{return fallback};d,err:=time.ParseDuration(raw);if err!=nil||d<time.Second{return fallback};return d}
func fatal(err error){fmt.Fprintln(os.Stderr,"[signalforge-agent]",err);os.Exit(1)}
