package managementstore

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "time"

 _ "github.com/jackc/pgx/v5/stdlib"
 _ "modernc.org/sqlite"
)

type Persistence interface {
 Begin(context.Context) (Transaction, error)
 Close() error
}
type Transaction interface {
 Load() ([]byte, error)
 Save([]byte) error
 Commit() error
 Rollback() error
}
type Store struct { db *sql.DB; dialect string }
type Tx struct { tx *sql.Tx; dialect string }

// Open opens SQLite for single-host installations. SQLite WAL requires local storage.
func Open(path string, initial []byte) (*Store, error) {
 if strings.TrimSpace(path)=="" { return nil, errors.New("management database path is required") }
 if err:=os.MkdirAll(filepath.Dir(path),0700); err!=nil { return nil,err }
 var legacy []byte
 if b,err:=os.ReadFile(path); err==nil && len(b)>0 && string(b[:min(len(b),16)])!="SQLite format 3\x00" {
  if !json.Valid(b) { return nil,errors.New("existing management state is neither SQLite nor valid legacy JSON") }
  backup:=path+".legacy-json.bak"
  if _,e:=os.Stat(backup); e==nil { return nil,fmt.Errorf("legacy JSON migration backup already exists: %s",backup) }
  if e:=os.Rename(path,backup); e!=nil { return nil,fmt.Errorf("preserve legacy state before migration: %w",e) }
  legacy=b
 } else if err!=nil && !errors.Is(err,os.ErrNotExist) { return nil,err }
 dsn:="file:"+filepath.ToSlash(path)+"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_txlock=immediate"
 db,err:=sql.Open("sqlite",dsn); if err!=nil { return nil,err }
 db.SetMaxOpenConns(1); db.SetMaxIdleConns(1)
 s:=&Store{db:db,dialect:"sqlite"}
 if err=s.initialize(initial,legacy); err!=nil { _=db.Close(); return nil,err }
 if err=os.Chmod(path,0600); err!=nil { _=db.Close(); return nil,err }
 return s,nil
}

// OpenPostgres opens a server database for multi-instance deployments. Production URLs
// must enable TLS certificate verification (sslmode=verify-full where supported).
func OpenPostgres(dsn string, initial []byte) (*Store,error) {
 u,err:=url.Parse(dsn)
 if err!=nil || (u.Scheme!="postgres" && u.Scheme!="postgresql") || u.Host=="" { return nil,errors.New("management database URL must be a valid postgres:// or postgresql:// URL") }
 if !json.Valid(initial) { return nil,errors.New("initial management state must be valid JSON") }
 db,err:=sql.Open("pgx",dsn); if err!=nil { return nil,err }
 db.SetMaxOpenConns(20); db.SetMaxIdleConns(5); db.SetConnMaxLifetime(30*time.Minute)
 s:=&Store{db:db,dialect:"postgres"}
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second); defer cancel()
 if err=db.PingContext(ctx); err!=nil { _=db.Close(); return nil,fmt.Errorf("connect to management PostgreSQL: %w",err) }
 if err=s.initialize(initial,nil); err!=nil { _=db.Close(); return nil,err }
 return s,nil
}
func (s *Store) initialize(initial,legacy []byte) error {
 if !json.Valid(initial) { return errors.New("initial management state must be valid JSON") }
 if s.dialect=="postgres" {
  stmts:=[]string{
   `CREATE TABLE IF NOT EXISTS schema_migrations(version BIGINT PRIMARY KEY,name TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
   `CREATE TABLE IF NOT EXISTS management_state(id INTEGER PRIMARY KEY CHECK(id=1),payload BYTEA NOT NULL,updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
   `INSERT INTO schema_migrations(version,name) VALUES(1,'initial_management_state') ON CONFLICT(version) DO NOTHING`,
  }
  for _,q:=range stmts { if _,err:=s.db.Exec(q); err!=nil { return fmt.Errorf("initialize PostgreSQL management schema: %w",err) } }
  payload:=initial; if len(legacy)>0 { payload=legacy }
  _,err:=s.db.Exec(`INSERT INTO management_state(id,payload) VALUES(1,$1) ON CONFLICT(id) DO NOTHING`,payload); return err
 }
 stmts:=[]string{
  `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
  `CREATE TABLE IF NOT EXISTS management_state(id INTEGER PRIMARY KEY CHECK(id=1),payload BLOB NOT NULL,updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
  `INSERT OR IGNORE INTO schema_migrations(version,name) VALUES(1,'initial_management_state')`,
 }
 for _,q:=range stmts { if _,err:=s.db.Exec(q); err!=nil { return err } }
 payload:=initial; if len(legacy)>0 { payload=legacy }
 _,err:=s.db.Exec(`INSERT INTO management_state(id,payload) VALUES(1,?) ON CONFLICT(id) DO NOTHING`,payload); return err
}
func (s *Store) Begin(ctx context.Context)(Transaction,error) {
 tx,err:=s.db.BeginTx(ctx,nil); if err!=nil { return nil,err }; return &Tx{tx:tx,dialect:s.dialect},nil
}
func (t *Tx) Load()([]byte,error) {
 var payload []byte
 q:=`SELECT payload FROM management_state WHERE id=1`
 if t.dialect=="postgres" { q+=" FOR UPDATE" }
 if err:=t.tx.QueryRow(q).Scan(&payload); err!=nil { return nil,err }
 if !json.Valid(payload) { return nil,errors.New("stored management state is corrupt") }
 return payload,nil
}
func (t *Tx) Save(payload []byte)error {
 if !json.Valid(payload) { return errors.New("refusing to persist invalid management JSON") }
 q:=`UPDATE management_state SET payload=?,updated_at=CURRENT_TIMESTAMP WHERE id=1`
 if t.dialect=="postgres" { q=`UPDATE management_state SET payload=$1,updated_at=CURRENT_TIMESTAMP WHERE id=1` }
 _,err:=t.tx.Exec(q,payload); return err
}
func (t *Tx) Commit()error { return t.tx.Commit() }
func (t *Tx) Rollback()error { return t.tx.Rollback() }
func (s *Store) Close()error { return s.db.Close() }
