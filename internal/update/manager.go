package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultInstallDir = "/usr/local/bin/edgeCore"
	defaultService    = "edgeCore"
	upgradeTmpPrefix  = "edgeCore-upgrade-"
	healthTimeout     = 30 * time.Second

	// 升级阶段
	StageIdle        = "idle"
	StageDownloading = "downloading"
	StageVerifying   = "verifying"
	StageInstalling  = "installing"
	StageRestarting  = "restarting"
	StageUpstaging   = "upstaging"
	StageFailed      = "failed"
)

// State 返回给前端的升级进度。
type State struct {
	Stage         string    `json:"stage"`
	TargetVersion string    `json:"target_version,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	Error         string    `json:"error,omitempty"`
}

// Manager 编排一键自动升级：下载→校验→备份→替换→重启→健康检查/失败回滚。
type Manager struct {
	mu          sync.Mutex
	checker     *Checker
	installDir  string
	serviceName string
	httpClient  *http.Client
	execFn      func(name string, arg ...string) *exec.Cmd
	state       State
	running     bool
}

// NewManager 构造升级管理器。installDir 默认 /usr/local/bin/edgeCore。
func NewManager(installDir, serviceName string, checker *Checker) *Manager {
	if installDir == "" {
		installDir = defaultInstallDir
	}
	if serviceName == "" {
		serviceName = defaultService
	}
	return &Manager{
		checker:     checker,
		installDir:  installDir,
		serviceName: serviceName,
		httpClient:  &http.Client{Timeout: 5 * time.Minute},
		execFn:      exec.Command,
		state:       State{Stage: StageIdle},
	}
}

// Status 返回当前升级状态（线程安全）。
func (m *Manager) Status() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Checker 暴露底层的版本检查器（供 handler 查询）。
func (m *Manager) Checker() *Checker { return m.checker }

func (m *Manager) setState(stage, target, errMsg string) {
	m.mu.Lock()
	m.state.Stage = stage
	if target != "" {
		m.state.TargetVersion = target
	}
	m.state.Error = errMsg
	m.running = !(stage == StageFailed || stage == StageUpstaging)
	m.mu.Unlock()
}

// IsRunning 报告是否已有升级在推进中。
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Start 以异步方式启动升级到 targetVersion（由调用方先经 check/compare 确认）。
func (m *Manager) Start(ctx context.Context, targetVersion string) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("升级已在进行中")
	}
	m.running = true
	m.state = State{Stage: StageDownloading, TargetVersion: targetVersion, StartedAt: time.Now()}
	m.mu.Unlock()
	go m.run(ctx, targetVersion)
	return nil
}

func (m *Manager) run(ctx context.Context, targetVersion string) {
	fail := func(stageMsg string) {
		m.setState(StageFailed, targetVersion, stageMsg)
	}
	tmp, err := os.MkdirTemp("", upgradeTmpPrefix)
	if err != nil {
		fail("创建临时目录失败: " + err.Error())
		return
	}
	defer os.RemoveAll(tmp)

	rel, err := m.checker.FetchLatest(ctx)
	if err != nil {
		fail("获取发布信息失败: " + err.Error())
		return
	}
	asset, err := m.checker.ArchiveAsset(rel, targetVersion)
	if err != nil {
		fail(err.Error())
		return
	}

	m.setState(StageDownloading, targetVersion, "")
	archPath := filepath.Join(tmp, asset.Name)
	if err := m.download(ctx, asset.BrowserDownloadURL, archPath); err != nil {
		fail("下载升级包失败: " + err.Error())
		return
	}

	m.setState(StageVerifying, targetVersion, "")
	if sums := FindAsset(rel, "SHA256SUMS"); sums != nil {
		sumPath := filepath.Join(tmp, "SHA256SUMS")
		if err := m.download(ctx, sums.BrowserDownloadURL, sumPath); err != nil {
			fail("下载校验文件失败: " + err.Error())
			return
		}
		ok, verr := verifySHA256FromFile(sumPath, archPath, asset.Name)
		if verr != nil {
			fail("校验失败: " + verr.Error())
			return
		}
		if !ok {
			fail(fmt.Sprintf("SHA256 校验不匹配，已中止。%s 内容不可信", asset.Name))
			return
		}
	}

	extractDir := filepath.Join(tmp, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		fail("创建解压目录失败: " + err.Error())
		return
	}
	if err := untar(archPath, extractDir); err != nil {
		fail("解压升级包失败: " + err.Error())
		return
	}

	m.setState(StageInstalling, targetVersion, "")
	if err := m.install(extractDir); err != nil {
		fail("安装失败: " + err.Error())
		m.rollback()
		return
	}

	m.setState(StageRestarting, targetVersion, "")
	if err := m.restartAndHealth(); err != nil {
		m.rollback()
		fail("服务重启失败，已回滚到原版本: " + err.Error())
		return
	}

	m.setState(StageUpstaging, targetVersion, "")
}

// install 备份当前二进制与前端，再原子替换归档内容。
func (m *Manager) install(extractDir string) error {
	if err := os.MkdirAll(m.installDir, 0o755); err != nil {
		return err
	}
	binSrc := filepath.Join(extractDir, "edgeCore")
	if st, err := os.Stat(binSrc); err != nil || st.IsDir() {
		return fmt.Errorf("升级包缺少 edgeCore 二进制")
	}

	// 备份当前二进制，供失败回滚
	prevBin := filepath.Join(m.installDir, "edgeCore.prev")
	if err := copyFile(filepath.Join(m.installDir, "edgeCore"), prevBin, 0o755); err != nil {
		return fmt.Errorf("备份当前二进制失败: %w", err)
	}
	if err := copyFileAtomic(binSrc, filepath.Join(m.installDir, "edgeCore"), 0o755); err != nil {
		return err
	}

	// 替换前端静态资源（若归档含 ui/dist）
	uiSrc := filepath.Join(extractDir, "ui", "dist")
	uiDst := filepath.Join(m.installDir, "ui", "dist")
	if st, err := os.Stat(uiSrc); err == nil && st.IsDir() {
		prevUI := uiDst + ".prev"
		_ = os.RemoveAll(prevUI)
		if _, err := os.Stat(uiDst); err == nil {
			if err := os.Rename(uiDst, prevUI); err != nil {
				return fmt.Errorf("备份前端目录失败: %w", err)
			}
		}
		if err := copyDir(uiSrc, uiDst); err != nil {
			return fmt.Errorf("替换前端资源失败: %w", err)
		}
	}
	return nil
}

// rollback 恢复备份的二进制与前端，并重启服务。
func (m *Manager) rollback() {
	prevBin := filepath.Join(m.installDir, "edgeCore.prev")
	if _, err := os.Stat(prevBin); err == nil {
		_ = copyFileAtomic(prevBin, filepath.Join(m.installDir, "edgeCore"), 0o755)
	}
	prevUI := filepath.Join(m.installDir, "ui", "dist.prev")
	if _, err := os.Stat(prevUI); err == nil {
		uiDst := filepath.Join(m.installDir, "ui", "dist")
		_ = os.RemoveAll(uiDst)
		_ = os.Rename(prevUI, uiDst)
	}
	_ = m.execFn("systemctl", "restart", m.serviceName).Run()
}

// restartAndHealth 重启服务并轮询其进入 active 状态。
func (m *Manager) restartAndHealth() error {
	if out, err := m.execFn("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s", strings.TrimSpace(string(out)))
	}
	if out, err := m.execFn("systemctl", "restart", m.serviceName).CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart %s: %s", m.serviceName, strings.TrimSpace(string(out)))
	}
	deadline := time.Now().Add(healthTimeout)
	for time.Now().Before(deadline) {
		out, _ := m.execFn("systemctl", "is-active", m.serviceName).Output()
		if strings.TrimSpace(string(out)) == "active" {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("升级后服务在 %s 内未进入 active", healthTimeout)
}

// download 以流式方式下载 URL 到目标文件。
func (m *Manager) download(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "edgeCore-upgrade")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// verifySHA256FromFile 读取 SHA256SUMS，校验 archive 的哈希是否匹配。
func verifySHA256FromFile(sumPath, archPath, assetName string) (bool, error) {
	data, err := os.ReadFile(sumPath)
	if err != nil {
		return false, err
	}
	want := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return false, nil // 校验文件中未找到该归档条目 → 保守中止
	}
	h := sha256.New()
	f, err := os.Open(archPath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want), nil
}

// untar 解压 tar.gz 归档到 dest。
func untar(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// 防止路径穿越
		target := filepath.Join(dest, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			return fmt.Errorf("归档含非法路径: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// copyFile 复制源文件到目标并设置权限。
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// copyFileAtomic 复制文件并 chmod（替换 installDir 内二进制时保证可执行）。
func copyFileAtomic(src, dst string, mode os.FileMode) error {
	tmp := dst + ".inflight"
	if err := copyFile(src, tmp, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// copyDir 递归复制目录。
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		fMode := info.Mode()
		if fMode&os.ModeSymlink != 0 {
			link, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			return os.Symlink(link, target)
		}
		return copyFile(path, target, info.Mode())
	})
}