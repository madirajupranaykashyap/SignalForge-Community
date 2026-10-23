package managementstore

import (
 "context"
 "encoding/json"
 "os"
 "sync"
 "testing"
 "time"
)
func postgresTestStore(t *testing.T)*Store {
 t.Helper(); dsn:=os.Getenv("SIGNALFORGE_TEST_POSTGRES_URL")
 if dsn=="" { t.Skip("SIGNALFORGE_TEST_POSTGRES_URL is not configured") }
 s,err:=OpenPostgres(dsn,[]byte(`{"n":0}`)); if err!=nil { t.Fatalf("OpenPostgres: %v",err) }
 t.Cleanup(func(){_=s.Close()})
 if _,err=s.db.Exec(`UPDATE management_state SET payload=$1 WHERE id=1`,[]byte(`{"n":0}`)); err!=nil { t.Fatalf("reset state: %v",err) }
 return s
}
func TestPostgresTransactionPersistsAndReloads(t *testing.T) {
 s:=postgresTestStore(t); tx,err:=s.Begin(context.Background()); if err!=nil { t.Fatal(err) }
 if _,err=tx.Load(); err!=nil { t.Fatal(err) }
 if err=tx.Save([]byte(`{"n":7}`)); err!=nil { t.Fatal(err) }
 if err=tx.Commit(); err!=nil { t.Fatal(err) }
 tx,err=s.Begin(context.Background()); if err!=nil { t.Fatal(err) }; defer tx.Rollback()
 got,err:=tx.Load(); if err!=nil { t.Fatal(err) }
 var state struct{ N int `json:"n"` }; if err=json.Unmarshal(got,&state); err!=nil { t.Fatal(err) }
 if state.N!=7 { t.Fatalf("got n=%d, want 7",state.N) }
}
func TestPostgresConcurrentTransactionsDoNotLoseUpdates(t *testing.T) {
 s:=postgresTestStore(t); const workers=12
 var wg sync.WaitGroup; errs:=make(chan error,workers)
 for i:=0;i<workers;i++ { wg.Add(1); go func(){ defer wg.Done()
  ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second); defer cancel()
  tx,err:=s.Begin(ctx); if err!=nil { errs<-err; return }; defer tx.Rollback()
  payload,err:=tx.Load(); if err!=nil { errs<-err; return }
  var state struct{ N int `json:"n"` }; if err=json.Unmarshal(payload,&state); err!=nil { errs<-err; return }
  state.N++; payload,err=json.Marshal(state); if err!=nil { errs<-err; return }
  if err=tx.Save(payload); err!=nil { errs<-err; return }; errs<-tx.Commit()
 }() }
 wg.Wait(); close(errs); for err:=range errs { if err!=nil { t.Error(err) } }
 tx,err:=s.Begin(context.Background()); if err!=nil { t.Fatal(err) }; defer tx.Rollback()
 payload,err:=tx.Load(); if err!=nil { t.Fatal(err) }
 var got struct{ N int `json:"n"` }; if err=json.Unmarshal(payload,&got); err!=nil { t.Fatal(err) }
 if got.N!=workers { t.Fatalf("lost updates: got %d, want %d",got.N,workers) }
}
