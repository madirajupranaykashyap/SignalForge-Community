package managementstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Store persists the management snapshot transactionally in SQLite. Requests
// hold an IMMEDIATE transaction across load/mutate/save, serializing writers
// across processes that share this host-local database.
type Store struct{ db *sql.DB }
type Tx struct{ tx *sql.Tx }

func Open(path string, initial []byte) (*Store,error) {
	if strings.TrimSpace(path)=="" { return nil,errors.New("management database path is required") }
	if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return nil,err}
	var legacy []byte
	if b,err:=os.ReadFile(path);err==nil&&len(b)>0&&string(b[:min(len(b),16)])!="SQLite format 3\x00" {
		if !json.Valid(b){return nil,errors.New("existing management state is neither SQLite nor valid legacy JSON")}
		backup:=path+".legacy-json.bak"
		if _,e:=os.Stat(backup);e==nil{return nil,fmt.Errorf("legacy JSON migration backup already exists: %s",backup)}
		if e:=os.Rename(path,backup);e!=nil{return nil,fmt.Errorf("preserve legacy state before migration: %w",e)}
		legacy=b
	} else if err!=nil&&!errors.Is(err,os.ErrNotExist){return nil,err}

	dsn:="file:"+filepath.ToSlash(path)+"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_txlock=immediate"
	db,err:=sql.Open("sqlite",dsn);if err!=nil{return nil,err}
	db.SetMaxOpenConns(1);db.SetMaxIdleConns(1)
	fail:=func(e error)(*Store,error){_ = db.Close();return nil,e}
	if err=db.Ping();err!=nil{return fail(err)}
	if err=os.Chmod(path,0600);err!=nil{return fail(err)}
	if _,err=db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`);err!=nil{return fail(err)}
	if _,err=db.Exec(`CREATE TABLE IF NOT EXISTS management_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		payload BLOB NOT NULL,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`);err!=nil{return fail(err)}
	if _,err=db.Exec(`INSERT OR IGNORE INTO schema_migrations(version,name) VALUES(1,'initial_management_state')`);err!=nil{return fail(err)}
	payload:=initial;if len(legacy)>0{payload=legacy}
	if len(payload)==0||!json.Valid(payload){return fail(errors.New("initial management state must be valid JSON"))}
	if _,err=db.Exec(`INSERT INTO management_state(id,payload) VALUES(1,?) ON CONFLICT(id) DO NOTHING`,payload);err!=nil{return fail(err)}
	return &Store{db:db},nil
}
func (s *Store) Begin(ctx context.Context)(*Tx,error){
	tx,err:=s.db.BeginTx(ctx,nil)
	if err!=nil{return nil,err};return &Tx{tx:tx},nil
}
func (t *Tx) Load()([]byte,error){
	var payload []byte
	if err:=t.tx.QueryRow(`SELECT payload FROM management_state WHERE id=1`).Scan(&payload);err!=nil{return nil,err}
	if !json.Valid(payload){return nil,errors.New("stored management state is corrupt")}
	return payload,nil
}
func (t *Tx) Save(payload []byte)error{
	if !json.Valid(payload){return errors.New("refusing to persist invalid management JSON")}
	_,err:=t.tx.Exec(`UPDATE management_state SET payload=?,updated_at=CURRENT_TIMESTAMP WHERE id=1`,payload);return err
}
func (t *Tx) Commit()error{return t.tx.Commit()}
func (t *Tx) Rollback()error{return t.tx.Rollback()}
func (s *Store) Close()error{return s.db.Close()}
