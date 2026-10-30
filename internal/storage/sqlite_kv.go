package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（无 CGO），注册名为 "sqlite"
)

// sqliteKV 以 modernc.org/sqlite（纯 Go）实现 KVDB。
// 数据模型：单表 kv(bucket TEXT, key BLOB, value BLOB)，主键 (bucket,key)，
// key 以 BLOB 存储 → 排序为 memcmp 字节序，与 bbolt 一致。
//
// 并发模型：
//   - journal_mode=WAL：读不阻塞写、写不阻塞读；
//   - 写事务经 writeMu 全局串行（对齐 bbolt 单写者语义），使用独立连接；
//   - 读事务从连接池取连接，BEGIN DEFERRED 提供一致快照。
type sqliteKV struct {
	path string
	sql  *sql.DB
	// writeConn 是专用写连接：保证 BEGIN/COMMIT 在同一连接上执行，
	// 并彻底避免两个写事务并发时的 SQLITE_BUSY。
	writeConn *sql.Conn
	writeMu   sync.Mutex
}

const sqliteDSNPragmas = "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)&_pragma=cache_size(-8000)"

func openSQLiteKV(path string) (KVDB, error) {
	db, err := sql.Open("sqlite", "file:"+path+sqliteDSNPragmas)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database %s: %w", path, err)
	}
	// 写事务走专用连接，读连接按需创建
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

	ctx := context.Background()
	wconn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to acquire sqlite write connection: %w", err)
	}

	kv := &sqliteKV{path: path, sql: db, writeConn: wconn}

	// 建表（幂等）：buckets 是 bucket 注册表，保证「已创建的空 bucket」
	// 与「从未创建的 bucket」可区分（对齐 bbolt 语义）。
	if _, err := wconn.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS kv (
			bucket TEXT NOT NULL,
			key    BLOB NOT NULL,
			value  BLOB,
			PRIMARY KEY (bucket, key)
		) WITHOUT ROWID`); err != nil {
		_ = wconn.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to init sqlite kv table: %w", err)
	}
	if _, err := wconn.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS buckets (
			name TEXT PRIMARY KEY
		)`); err != nil {
		_ = wconn.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to init sqlite buckets table: %w", err)
	}
	return kv, nil
}

func (s *sqliteKV) Path() string    { return s.path }
func (s *sqliteKV) Backend() string { return BackendSQLite }

func (s *sqliteKV) Close() error {
	if s.writeConn != nil {
		// 关闭前做一次 checkpoint，把 WAL 合并进主文件，保证单文件拷贝可用
		_, _ = s.writeConn.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)")
		_ = s.writeConn.Close()
	}
	return s.sql.Close()
}

func (s *sqliteKV) Sync() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.writeConn.ExecContext(context.Background(), "PRAGMA wal_checkpoint(FULL)")
	return err
}

// Compact 原地 VACUUM 重建文件（与 bbolt 不同：无需关库换文件）。
func (s *sqliteKV) Compact() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.writeConn.ExecContext(context.Background(), "VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum sqlite db: %w", err)
	}
	// 重建后把 WAL 合并回主文件，避免长期运行下 -wal 无限增长
	_, _ = s.writeConn.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)")
	return nil
}

// BackupTo 使用 VACUUM INTO 输出事务一致性快照。
func (s *sqliteKV) BackupTo(dstPath string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = os.Remove(dstPath)
	quoted := strings.ReplaceAll(dstPath, "'", "''")
	// #nosec G202 -- SQLite VACUUM INTO 不支持参数绑定，路径中的单引号已转义（' → ''），
	// 且 dstPath 为服务端生成的快照临时路径，非用户输入
	_, err := s.writeConn.ExecContext(context.Background(), "VACUUM INTO '"+quoted+"'")
	if err != nil {
		return fmt.Errorf("failed to backup sqlite db: %w", err)
	}
	return nil
}

// ---- 事务 ----

func (s *sqliteKV) Update(fn func(tx KVTx) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	ctx := context.Background()
	tx := &sqliteTx{ctx: ctx, conn: s.writeConn}
	defer tx.closeCursors()

	if _, err := s.writeConn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("sqlite begin immediate: %w", err)
	}
	if err := fn(tx); err != nil {
		_, _ = s.writeConn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	if _, err := s.writeConn.ExecContext(ctx, "COMMIT"); err != nil {
		_, _ = s.writeConn.ExecContext(ctx, "ROLLBACK")
		return fmt.Errorf("sqlite commit: %w", err)
	}
	return nil
}

func (s *sqliteKV) View(fn func(tx KVTx) error) error {
	ctx := context.Background()
	conn, err := s.sql.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	tx := &sqliteTx{ctx: ctx, conn: conn}
	defer tx.closeCursors()

	if _, err := conn.ExecContext(ctx, "BEGIN DEFERRED"); err != nil {
		return fmt.Errorf("sqlite begin deferred: %w", err)
	}
	if err := fn(tx); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("sqlite end read tx: %w", err)
	}
	return nil
}

// ---- Tx 实现 ----

type sqliteTx struct {
	ctx  context.Context
	conn *sql.Conn

	cursors []*sqliteCursor // 本事务内创建的游标，事务结束时统一关流（防止早退迭代泄漏连接）
}

func (t *sqliteTx) Get(bucket string, key []byte) []byte {
	var value []byte
	err := t.conn.QueryRowContext(t.ctx,
		"SELECT value FROM kv WHERE bucket = ? AND key = ?", bucket, key).Scan(&value)
	if err != nil {
		return nil
	}
	return value
}

func (t *sqliteTx) Put(bucket string, key []byte, value []byte) error {
	_, err := t.conn.ExecContext(t.ctx,
		`INSERT INTO kv (bucket, key, value) VALUES (?, ?, ?)
		 ON CONFLICT(bucket, key) DO UPDATE SET value = excluded.value`,
		bucket, key, value)
	return err
}

func (t *sqliteTx) Delete(bucket string, key []byte) error {
	_, err := t.conn.ExecContext(t.ctx,
		"DELETE FROM kv WHERE bucket = ? AND key = ?", bucket, key)
	return err
}

func (t *sqliteTx) CreateBucketIfNotExists(bucket string) error {
	_, err := t.conn.ExecContext(t.ctx,
		"INSERT OR IGNORE INTO buckets (name) VALUES (?)", bucket)
	return err
}

func (t *sqliteTx) DeleteBucket(bucket string) error {
	if _, err := t.conn.ExecContext(t.ctx, "DELETE FROM kv WHERE bucket = ?", bucket); err != nil {
		return err
	}
	_, err := t.conn.ExecContext(t.ctx, "DELETE FROM buckets WHERE name = ?", bucket)
	return err
}

func (t *sqliteTx) HasBucket(bucket string) bool {
	var one int
	err := t.conn.QueryRowContext(t.ctx,
		"SELECT 1 FROM buckets WHERE name = ? LIMIT 1", bucket).Scan(&one)
	return err == nil
}

func (t *sqliteTx) Buckets() ([]string, error) {
	rows, err := t.conn.QueryContext(t.ctx, "SELECT name FROM buckets ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (t *sqliteTx) ForEach(bucket string, fn func(k, v []byte) error) error {
	rows, err := t.conn.QueryContext(t.ctx,
		"SELECT key, value FROM kv WHERE bucket = ? ORDER BY key", bucket)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (t *sqliteTx) CountKeys(bucket string) (int, error) {
	var n int
	err := t.conn.QueryRowContext(t.ctx,
		"SELECT COUNT(*) FROM kv WHERE bucket = ?", bucket).Scan(&n)
	return n, err
}

func (t *sqliteTx) Cursor(bucket string) KVCursor {
	c := &sqliteCursor{tx: t, bucket: bucket}
	t.cursors = append(t.cursors, c)
	return c
}

// closeCursors 关闭本事务内所有游标的流式结果集。
// 必须在事务结束（COMMIT/ROLLBACK）前调用：提前退出的迭代（如 LoadRange
// 达到 maxKey）会留下未读完的 *sql.Rows，占住连接导致池死锁。
func (t *sqliteTx) closeCursors() {
	for _, c := range t.cursors {
		c.closeStreams()
	}
	t.cursors = nil
}

// ---- Cursor 实现 ----
// 策略：正向迭代（First/Next/Seek）与反向迭代（Last/Prev）各自维护一个
// 惰性打开的流式结果集；方向切换时关闭旧流重开。Delete 对当前键直接执行
// DELETE（当前行的数据已在内存中，流继续有效）。

type sqliteCursor struct {
	tx     *sqliteTx
	bucket string

	curKey []byte
	curVal []byte
	valid  bool

	fwdRows *sql.Rows // 正向流（key >= seek 起点）
	revRows *sql.Rows // 反向流（key <= 起点，DESC）
}

func (c *sqliteCursor) closeStreams() {
	if c.fwdRows != nil {
		c.fwdRows.Close()
		c.fwdRows = nil
	}
	if c.revRows != nil {
		c.revRows.Close()
		c.revRows = nil
	}
}

func (c *sqliteCursor) openForward(from []byte) error {
	c.closeStreams()
	var err error
	if from == nil {
		c.fwdRows, err = c.tx.conn.QueryContext(c.tx.ctx,
			"SELECT key, value FROM kv WHERE bucket = ? ORDER BY key ASC", c.bucket)
	} else {
		c.fwdRows, err = c.tx.conn.QueryContext(c.tx.ctx,
			"SELECT key, value FROM kv WHERE bucket = ? AND key >= ? ORDER BY key ASC", c.bucket, from)
	}
	return err
}

func (c *sqliteCursor) openBackward(from []byte) error {
	c.closeStreams()
	var err error
	if from == nil {
		c.revRows, err = c.tx.conn.QueryContext(c.tx.ctx,
			"SELECT key, value FROM kv WHERE bucket = ? ORDER BY key DESC", c.bucket)
	} else {
		c.revRows, err = c.tx.conn.QueryContext(c.tx.ctx,
			"SELECT key, value FROM kv WHERE bucket = ? AND key <= ? ORDER BY key DESC", c.bucket, from)
	}
	return err
}

func (c *sqliteCursor) invalidate() {
	c.valid = false
	c.curKey, c.curVal = nil, nil
}

func (c *sqliteCursor) First() ([]byte, []byte) {
	if err := c.openForward(nil); err != nil {
		c.invalidate()
		return nil, nil
	}
	return c.nextFromForward()
}

func (c *sqliteCursor) nextFromForward() ([]byte, []byte) {
	if c.fwdRows == nil {
		c.invalidate()
		return nil, nil
	}
	if c.fwdRows.Next() {
		var k, v []byte
		if err := c.fwdRows.Scan(&k, &v); err != nil {
			c.invalidate()
			return nil, nil
		}
		c.curKey, c.curVal, c.valid = k, v, true
		return c.curKey, c.curVal
	}
	c.invalidate()
	return nil, nil
}

func (c *sqliteCursor) Next() ([]byte, []byte) {
	if !c.valid {
		return nil, nil
	}
	if c.fwdRows == nil {
		// 从当前键继续正向迭代
		if err := c.openForwardAfter(c.curKey); err != nil {
			c.invalidate()
			return nil, nil
		}
		return c.nextFromForward()
	}
	return c.nextFromForward()
}

func (c *sqliteCursor) openForwardAfter(key []byte) error {
	c.closeStreams()
	var err error
	c.fwdRows, err = c.tx.conn.QueryContext(c.tx.ctx,
		"SELECT key, value FROM kv WHERE bucket = ? AND key > ? ORDER BY key ASC", c.bucket, key)
	return err
}

func (c *sqliteCursor) Last() ([]byte, []byte) {
	if err := c.openBackward(nil); err != nil {
		c.invalidate()
		return nil, nil
	}
	return c.prevFromBackward()
}

func (c *sqliteCursor) prevFromBackward() ([]byte, []byte) {
	if c.revRows == nil {
		c.invalidate()
		return nil, nil
	}
	if c.revRows.Next() {
		var k, v []byte
		if err := c.revRows.Scan(&k, &v); err != nil {
			c.invalidate()
			return nil, nil
		}
		c.curKey, c.curVal, c.valid = k, v, true
		return c.curKey, c.curVal
	}
	c.invalidate()
	return nil, nil
}

func (c *sqliteCursor) Prev() ([]byte, []byte) {
	if !c.valid {
		return nil, nil
	}
	if c.revRows == nil {
		if err := c.openBackwardBefore(c.curKey); err != nil {
			c.invalidate()
			return nil, nil
		}
		return c.prevFromBackward()
	}
	return c.prevFromBackward()
}

func (c *sqliteCursor) openBackwardBefore(key []byte) error {
	c.closeStreams()
	var err error
	c.revRows, err = c.tx.conn.QueryContext(c.tx.ctx,
		"SELECT key, value FROM kv WHERE bucket = ? AND key < ? ORDER BY key DESC", c.bucket, key)
	return err
}

func (c *sqliteCursor) Seek(prefix []byte) ([]byte, []byte) {
	if err := c.openForward(prefix); err != nil {
		c.invalidate()
		return nil, nil
	}
	return c.nextFromForward()
}

func (c *sqliteCursor) Delete() error {
	if !c.valid {
		return fmt.Errorf("sqlite cursor: no current key")
	}
	_, err := c.tx.conn.ExecContext(c.tx.ctx,
		"DELETE FROM kv WHERE bucket = ? AND key = ?", c.bucket, c.curKey)
	return err
}
