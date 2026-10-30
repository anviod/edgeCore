package storage

import (
	"fmt"
	"os"

	"go.etcd.io/bbolt"
)

const compactTxMaxSize = 65536 // matches bbolt CLI default; avoids OOM during large compacts

// bboltKV 将 *bbolt.DB 适配为 KVDB。行为即 bbolt 原生语义。
type bboltKV struct {
	db *bbolt.DB
}

// openBoltKV 打开（或创建）bbolt 数据库并适配为 KVDB。
// allowCleanup 对应 NoGrowSync 选项（runtime 库允许清理/compact 重建）。
func openBoltKV(path string, allowCleanup bool) (KVDB, error) {
	db, err := openBoltDB(path, allowCleanup)
	if err != nil {
		return nil, err
	}
	return &bboltKV{db: db}, nil
}

func (b *bboltKV) View(fn func(tx KVTx) error) error {
	return b.db.View(func(tx *bbolt.Tx) error { return fn(&bboltTx{tx: tx}) })
}

func (b *bboltKV) Update(fn func(tx KVTx) error) error {
	return b.db.Update(func(tx *bbolt.Tx) error { return fn(&bboltTx{tx: tx}) })
}

func (b *bboltKV) Close() error { return b.db.Close() }

func (b *bboltKV) Path() string { return b.db.Path() }

func (b *bboltKV) Backend() string { return BackendBbolt }

func (b *bboltKV) Sync() error { return b.db.Sync() }

// Compact 整文件重写并原子换绑：关闭当前库 → 旧文件改名 .pre-compact.bak →
// 压缩文件改名回原名 → 重新打开（句柄仍是同一 bboltKV 实例）。
// Storage 层的 runtimeMu 依旧负责让换绑窗口内的写入排队。
func (b *bboltKV) Compact() error {
	srcPath := b.db.Path()
	tmpPath := srcPath + ".compact.tmp"
	_ = os.Remove(tmpPath)

	dst, err := openBoltDB(tmpPath, true)
	if err != nil {
		return fmt.Errorf("failed to create compact temp db: %w", err)
	}
	if err := bbolt.Compact(dst, b.db, compactTxMaxSize); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to compact runtime db: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close compact temp db: %w", err)
	}

	tmpInfo, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to stat compacted file: %w", err)
	}
	if tmpInfo.Size() == 0 {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("compacted file is empty, aborting")
	}

	backupPath := srcPath + ".pre-compact.bak"
	if err := b.db.Close(); err != nil {
		return fmt.Errorf("failed to close runtime db: %w", err)
	}
	if err := os.Rename(srcPath, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		if reopenErr := b.reopen(srcPath); reopenErr != nil {
			return fmt.Errorf("failed to backup original runtime db: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to backup original runtime db: %w", err)
	}
	if err := os.Rename(tmpPath, srcPath); err != nil {
		_ = os.Rename(backupPath, srcPath)
		if reopenErr := b.reopen(srcPath); reopenErr != nil {
			return fmt.Errorf("failed to replace runtime db with compacted file: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to replace runtime db with compacted file: %w", err)
	}
	if err := b.reopen(srcPath); err != nil {
		return fmt.Errorf("failed to reopen compacted runtime db: %w", err)
	}
	_ = os.Remove(backupPath)
	return nil
}

func (b *bboltKV) reopen(path string) error {
	db, err := openBoltDB(path, true)
	if err != nil {
		return err
	}
	b.db = db
	return nil
}

// BackupTo 通过只读事务快照（WriteTo）输出一致性热备。
func (b *bboltKV) BackupTo(dstPath string) error {
	if _, err := os.Stat(b.db.Path()); err != nil {
		return fmt.Errorf("database file does not exist: %s", b.db.Path())
	}
	// #nosec G304 -- dstPath 由导出/备份流程在服务端生成（os.CreateTemp 或固定备份目录），非外部输入
	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer f.Close()
	return b.db.View(func(tx *bbolt.Tx) error {
		_, err := tx.WriteTo(f)
		return err
	})
}

// ---- Tx / Cursor 适配 ----

type bboltTx struct{ tx *bbolt.Tx }

func (t *bboltTx) bucket(name string) *bbolt.Bucket {
	return t.tx.Bucket([]byte(name))
}

func (t *bboltTx) Get(bucket string, key []byte) []byte {
	b := t.bucket(bucket)
	if b == nil {
		return nil
	}
	v := b.Get(key)
	if v == nil {
		return nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out
}

func (t *bboltTx) Put(bucket string, key []byte, value []byte) error {
	b := t.bucket(bucket)
	if b == nil {
		return fmt.Errorf("bucket %s not found", bucket)
	}
	return b.Put(key, value)
}

func (t *bboltTx) Delete(bucket string, key []byte) error {
	b := t.bucket(bucket)
	if b == nil {
		return nil
	}
	return b.Delete(key)
}

func (t *bboltTx) CreateBucketIfNotExists(bucket string) error {
	_, err := t.tx.CreateBucketIfNotExists([]byte(bucket))
	return err
}

func (t *bboltTx) DeleteBucket(bucket string) error {
	if t.bucket(bucket) == nil {
		return nil // 幂等：与 sqlite 实现语义一致
	}
	return t.tx.DeleteBucket([]byte(bucket))
}

func (t *bboltTx) Buckets() ([]string, error) {
	var names []string
	err := t.tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
		names = append(names, string(name))
		return nil
	})
	return names, err
}

func (t *bboltTx) HasBucket(bucket string) bool {
	return t.bucket(bucket) != nil
}

func (t *bboltTx) ForEach(bucket string, fn func(k, v []byte) error) error {
	b := t.bucket(bucket)
	if b == nil {
		return nil
	}
	return b.ForEach(func(k, v []byte) error {
		if v == nil {
			// bbolt 中嵌套 bucket 的 ForEach 会给 v=nil；本层 KV 语义无嵌套 bucket，
			// 正常情况下不会出现，防御性跳过。
			return nil
		}
		return fn(k, v)
	})
}

func (t *bboltTx) Cursor(bucket string) KVCursor {
	b := t.bucket(bucket)
	if b == nil {
		// 缺失 bucket 视为空集合（对齐「遍历缺失 bucket 为空」约定），防 nil panic
		return emptyCursor{}
	}
	return &bboltCursor{c: b.Cursor()}
}

func (t *bboltTx) CountKeys(bucket string) (int, error) {
	b := t.bucket(bucket)
	if b == nil {
		return 0, nil
	}
	return b.Stats().KeyN, nil
}

type bboltCursor struct{ c *bbolt.Cursor }

func (c *bboltCursor) First() ([]byte, []byte)             { return c.c.First() }
func (c *bboltCursor) Last() ([]byte, []byte)              { return c.c.Last() }
func (c *bboltCursor) Next() ([]byte, []byte)              { return c.c.Next() }
func (c *bboltCursor) Prev() ([]byte, []byte)              { return c.c.Prev() }
func (c *bboltCursor) Seek(prefix []byte) ([]byte, []byte) { return c.c.Seek(prefix) }
func (c *bboltCursor) Delete() error                       { return c.c.Delete() }

// emptyCursor 是缺失 bucket 的空游标（全部操作返回 nil，Delete 返回错误）。
type emptyCursor struct{}

func (emptyCursor) First() ([]byte, []byte)             { return nil, nil }
func (emptyCursor) Last() ([]byte, []byte)              { return nil, nil }
func (emptyCursor) Next() ([]byte, []byte)              { return nil, nil }
func (emptyCursor) Prev() ([]byte, []byte)              { return nil, nil }
func (emptyCursor) Seek([]byte) ([]byte, []byte)        { return nil, nil }
func (emptyCursor) Delete() error                       { return fmt.Errorf("no current key") }
