package managementstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestTransactionPersistsAndReloadsState(t *testing.T){
	path:=filepath.Join(t.TempDir(),"management.db")
	initial:=[]byte(`{"version":1}`)
	s,err:=Open(path,initial);if err!=nil{t.Fatal(err)};defer s.Close()
	tx,err:=s.Begin(context.Background());if err!=nil{t.Fatal(err)}
	got,err:=tx.Load();if err!=nil{t.Fatal(err)}
	if string(got)!=string(initial){t.Fatalf("initial state = %s",got)}
	next:=[]byte(`{"version":2}`)
	if err=tx.Save(next);err!=nil{t.Fatal(err)}
	if err=tx.Commit();err!=nil{t.Fatal(err)}
	tx,err=s.Begin(context.Background());if err!=nil{t.Fatal(err)};defer tx.Rollback()
	got,err=tx.Load();if err!=nil{t.Fatal(err)}
	if string(got)!=string(next){t.Fatalf("persisted state = %s",got)}
}
func TestLegacyJSONIsBackedUpAndMigrated(t *testing.T){
	path:=filepath.Join(t.TempDir(),"management.json")
	legacy:=[]byte(`{"tenants":{"acme":true},"tokens":{},"agents":{},"policies":{},"audit":[]}`)
	if err:=os.WriteFile(path,legacy,0600);err!=nil{t.Fatal(err)}
	s,err:=Open(path,[]byte(`{}`));if err!=nil{t.Fatal(err)};defer s.Close()
	if _,err=os.Stat(path+".legacy-json.bak");err!=nil{t.Fatal("legacy backup missing",err)}
	tx,err:=s.Begin(context.Background());if err!=nil{t.Fatal(err)};defer tx.Rollback()
	got,err:=tx.Load();if err!=nil{t.Fatal(err)}
	var state map[string]any;if err=json.Unmarshal(got,&state);err!=nil{t.Fatal(err)}
	if state["tenants"]==nil{t.Fatalf("legacy state not imported: %s",got)}
}
func TestConcurrentTransactionsDoNotLoseUpdates(t *testing.T){
	path:=filepath.Join(t.TempDir(),"management.db")
	s1,err:=Open(path,[]byte(`{"n":0}`));if err!=nil{t.Fatal(err)};defer s1.Close()
	s2,err:=Open(path,[]byte(`{"n":0}`));if err!=nil{t.Fatal(err)};defer s2.Close()
	var wg sync.WaitGroup
	for i:=0;i<12;i++{wg.Add(1);go func(i int){defer wg.Done();s:=s1;if i%2==1{s=s2};tx,e:=s.Begin(context.Background());if e!=nil{t.Error(e);return};defer tx.Rollback();b,e:=tx.Load();if e!=nil{t.Error(e);return};var v struct{N int `json:"n"`};if e=json.Unmarshal(b,&v);e!=nil{t.Error(e);return};v.N++;b,e=json.Marshal(v);if e!=nil{t.Error(e);return};if e=tx.Save(b);e!=nil{t.Error(e);return};if e=tx.Commit();e!=nil{t.Error(e)}}(i)}
	wg.Wait()
	tx,err:=s1.Begin(context.Background());if err!=nil{t.Fatal(err)};defer tx.Rollback()
	b,err:=tx.Load();if err!=nil{t.Fatal(err)}
	var got struct{N int `json:"n"`};if err=json.Unmarshal(b,&got);err!=nil{t.Fatal(err)}
	if got.N!=12{t.Fatalf("lost concurrent updates: got %d want 12",got.N)}
}
