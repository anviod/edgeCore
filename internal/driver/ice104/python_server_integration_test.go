//go:build integration

package ice104

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("cannot locate repo root (go.mod)")
		}
		dir = parent
	}
}

func pythonExecutable(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("PYTHON"); v != "" {
		return v
	}
	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "python3"}
	}
	for _, name := range candidates {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if err := exec.Command(path, "-c", "import sys").Run(); err != nil {
			continue
		}
		return path
	}
	t.Skip("python not found in PATH (set PYTHON to override)")
	return ""
}

func c104Available(t *testing.T, python string) {
	t.Helper()
	out, err := exec.Command(python, "-c", "import c104").CombinedOutput()
	if err != nil {
		t.Skipf("c104 not installed for %s: %v (%s); run: pip install -r test/ice104-python-server/requirements.txt", python, err, out)
	}
}

func startPythonServer(t *testing.T) (host string, port int, stop func()) {
	t.Helper()
	python := pythonExecutable(t)
	c104Available(t, python)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	port, err = strconv.Atoi(portStr)
	require.NoError(t, err)
	require.NoError(t, ln.Close())

	root := findRepoRoot(t)
	script := filepath.Join(root, "test", "ice104-python-server", "server.py")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("python server script not found at %s: %v", script, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, python, script, "--bind", host, "--port", strconv.Itoa(port))
	cmd.Dir = filepath.Join(root, "test", "ice104-python-server")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())

	addr := fmt.Sprintf("%s:%d", host, port)
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 15*time.Second, 100*time.Millisecond)

	return host, port, func() {
		cancel()
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
	}
}

// TestIntegrationWithPythonServer exercises the driver against Fraunhofer c104
// (iec104-python / lib60870-C, GPLv3) over real TCP. Open-source Python alternative
// to Freyr pyiec104 proprietary DLL.
func TestIntegrationWithPythonServer(t *testing.T) {
	host, port, stop := startPythonServer(t)
	defer stop()

	cfg := map[string]any{
		"ip":            host,
		"port":          port,
		"commonAddress": 1,
		"t0":            5,
		"t1":            15,
	}
	transport := NewICE104Transport(cfg)
	ctx := context.Background()
	require.NoError(t, transport.Connect(ctx))
	defer transport.Disconnect()

	assert.True(t, transport.IsConnected())

	scheduler := NewICE104Scheduler(transport, NewICE104Decoder(), cfg)
	results, err := scheduler.ReadPoints(ctx, []model.Point{
		{ID: "ai-1", Address: "1", Group: "M_ME_NA_1", DataType: "FLOAT", ReportMode: "poll"},
	})
	require.NoError(t, err)
	require.Contains(t, results, "ai-1")
	require.Equal(t, "Good", results["ai-1"].Quality, "c104 server should return M_ME_NA_1 IOA=1 after GI")
	require.NotNil(t, results["ai-1"].Value)
	assert.InDelta(t, 0.5, results["ai-1"].Value.(float64), 0.01)
	assert.True(t, transport.IsConnected(), "link should stay up after GI")
}
