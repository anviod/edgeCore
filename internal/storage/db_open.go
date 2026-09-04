package storage

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

const boltOpenTimeout = 30 * time.Second

func openBoltDB(path string, noGrowSync bool) (*bbolt.DB, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{
		Timeout:    boltOpenTimeout,
		NoGrowSync: noGrowSync,
	})
	if err != nil {
		return nil, formatBoltOpenError(path, err)
	}
	return db, nil
}

func formatBoltOpenError(path string, err error) error {
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "timeout") {
		return fmt.Errorf("failed to open database %s: %w", path, err)
	}
	hint := boltLockTimeoutHint(path)
	return fmt.Errorf("failed to open database %s: %w; %s", path, err, hint)
}

func boltLockTimeoutHint(dbPath string) string {
	if holders := boltDBLockHolders(dbPath); holders != "" {
		return holders
	}
	return fmt.Sprintf(
		"database lock not acquired within %s — another edgeCore/go process may already be using %s; check with: lsof %q",
		boltOpenTimeout,
		dbPath,
		dbPath,
	)
}

// boltDBLockHolders reports PIDs from lsof when the DB file is open elsewhere (Unix).
func boltDBLockHolders(dbPath string) string {
	cmd := exec.Command("lsof", "-t", dbPath)
	out, err := cmd.Output()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	seen := make(map[string]struct{}, len(lines))
	var pids []string
	for _, line := range lines {
		pid := strings.TrimSpace(line)
		if pid == "" {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	if len(pids) == 0 {
		return ""
	}
	if len(pids) == 1 {
		return fmt.Sprintf(
			"process PID %s holds %s (stop it: kill %s, or killall main if it is a stale go run)",
			pids[0],
			dbPath,
			pids[0],
		)
	}
	return fmt.Sprintf(
		"processes PIDs %s hold %s (stop them before starting another instance)",
		strings.Join(pids, ", "),
		dbPath,
	)
}
