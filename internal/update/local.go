package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// 本地安装包支持三类格式：tar.gz（解压替换）、deb / rpm（包管理器安装）。
// 文件名需符合 .goreleaser.yml 产物命名：
//
//	edgeCore-{{.RawVersion}}-linux-{{.Arch}}.tar.gz   （Arch: amd64/arm64/arm7）
//	edgeCore-v{{.RawVersion}}-{{.Arch}}.{deb,rpm}     （Arch: amd64/arm64/arm）
var (
	reTar = regexp.MustCompile(`^edgeCore-(.+)-linux-(amd64|arm64|arm7)\.tar\.gz$`)
	reDeb = regexp.MustCompile(`^edgeCore-v(.+)-(amd64|arm64|arm)\.deb$`)
	reRpm = regexp.MustCompile(`^edgeCore-v(.+)-(amd64|arm64|arm)\.rpm$`)
)

// LocalPackage 描述一个上传的本地安装包。
type LocalPackage struct {
	FileName   string `json:"file_name"` // 原文件名
	Format     string `json:"format"`    // tar.gz | deb | rpm
	Version    string `json:"version"`
	Arch       string `json:"arch"` // 安装包声明的架构（amd64/arm64/arm7/arm）
	Size       int64  `json:"size"`
	Compatible bool   `json:"compatible"` // 是否与当前运行平台匹配
	Reason     string `json:"reason,omitempty"`
	Path       string `json:"path"` // 服务端暂存路径（仅校验通过时返回）
}

// allowedArch 判断安装包声明的架构是否兼容当前运行时。
func packageArchCompat(arch string) bool {
	// 将归档/包声明的架构归一化到 runtime.GOARCH，arm7 视为 GOARCH=arm（GOARM=7）。
	goArch := arch
	if arch == "arm7" {
		goArch = "arm"
	}
	return goArch == runtime.GOARCH && runtime.GOOS == "linux"
}

// ParseLocalFileName 仅依据文件名解析版本与架构，不读取内容。
func parseLocalName(fileName, format string) (version, arch string, ok bool) {
	switch format {
	case "tar.gz":
		m := reTar.FindStringSubmatch(fileName)
		if m == nil {
			return "", "", false
		}
		return strings.TrimPrefix(m[1], "v"), m[2], true
	case "deb":
		m := reDeb.FindStringSubmatch(fileName)
		if m == nil {
			return "", "", false
		}
		return strings.TrimPrefix(m[1], "v"), m[2], true
	case "rpm":
		m := reRpm.FindStringSubmatch(fileName)
		if m == nil {
			return "", "", false
		}
		return strings.TrimPrefix(m[1], "v"), m[2], true
	}
	return "", "", false
}

// validateContent 按格式深度校验文件内容是否可解析（归档完整性 / 包格式魔数）。
func validateContent(path, format string) error {
	switch format {
	case "tar.gz":
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("不是有效的 gzip 压缩文件: %w", err)
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		foundBin := false
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("tar 归档已损坏: %w", err)
			}
			// Windows 包内的可执行文件带 .exe；Linux 下为 edgeCore
			if hdr.Typeflag == tar.TypeReg && (hdr.Name == "edgeCore" || hdr.Name == "edgeCore.exe") {
				foundBin = true
			}
		}
		if !foundBin {
			return fmt.Errorf("升级包缺少可执行文件 (edgeCore)")
		}
	case "deb":
		buf, err := readHead(path, 8)
		if err != nil {
			return err
		}
		if string(buf) != "!<arch>\n" {
			return fmt.Errorf("不是有效的 deb 安装包（缺少 ar 归档头）")
		}
	case "rpm":
		buf, err := readHead(path, 4)
		if err != nil {
			return err
		}
		if buf[0] != 0xED || buf[1] != 0xAB || buf[2] != 0xEE || buf[3] != 0xDB {
			return fmt.Errorf("不是有效的 rpm 安装包（魔数校验失败）")
		}
	default:
		return fmt.Errorf("不支持的安装包格式: %s", format)
	}
	return nil
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, fmt.Errorf("读取文件头部失败: %w", err)
	}
	return buf, nil
}

// ValidateLocal 校验本地安装包：解析文件名 → 架构兼容 → 深度校验内容。
// 校验通过且平台兼容时返回 Compatible=true，Path 为该文件的暂存路径。
func (m *Manager) ValidateLocal(localPath string) (LocalPackage, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return LocalPackage{}, err
	}
	fileName := filepath.Base(localPath)
	format := detectFormat(fileName)
	if format == "" {
		return LocalPackage{FileName: fileName, Compatible: false, Reason: "不支持的文件格式（支持 .tar.gz / .deb / .rpm）"}, nil
	}
	version, arch, ok := parseLocalName(fileName, format)
	if !ok {
		return LocalPackage{FileName: fileName, Format: format, Compatible: false, Reason: "文件名不符合规范，无法识别版本号"}, nil
	}
	pkg := LocalPackage{
		FileName: fileName,
		Format:   format,
		Version:  version,
		Arch:     arch,
		Size:     info.Size(),
	}
	if !packageArchCompat(arch) {
		pkg.Reason = fmt.Sprintf("安装包架构 %s 与当前平台 %s/%s 不匹配", arch, runtime.GOOS, runtime.GOARCH)
		return pkg, nil
	}
	if err := validateContent(localPath, format); err != nil {
		pkg.Reason = "校验失败: " + err.Error()
		return pkg, nil
	}
	pkg.Compatible = true
	pkg.Path = localPath
	return pkg, nil
}

func detectFormat(name string) string {
	switch {
	case strings.HasSuffix(name, ".tar.gz"):
		return "tar.gz"
	case strings.HasSuffix(name, ".deb"):
		return "deb"
	case strings.HasSuffix(name, ".rpm"):
		return "rpm"
	}
	return ""
}

// StartLocal 以异步方式执行本地安装包升级/降级（指定文件，不限版本高低）。
func (m *Manager) StartLocal(ctx context.Context, localPath string) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("升级已在进行中")
	}
	pkg, perr := m.ValidateLocal(localPath)
	if perr != nil || !pkg.Compatible {
		m.mu.Unlock()
		if perr != nil {
			return perr
		}
		return fmt.Errorf("安装包校验失败: %s", pkg.Reason)
	}
	m.running = true
	m.state = State{Stage: StageVerifying, TargetVersion: pkg.Version, StartedAt: time.Now()}
	m.mu.Unlock()
	go m.runLocal(ctx, pkg)
	return nil
}

func (m *Manager) runLocal(ctx context.Context, pkg LocalPackage) {
	fail := func(stageMsg string) {
		m.setState(StageFailed, pkg.Version, stageMsg)
	}
	m.setState(StageInstalling, pkg.Version, "")
	var installErr error
	if pkg.Format == "deb" || pkg.Format == "rpm" {
		installErr = m.installPkg(pkg.Path, pkg.Format)
	} else {
		installErr = m.installTarPath(pkg.Path)
	}
	if installErr != nil {
		m.rollbackPkg(pkg.Format)
		fail("安装失败，已尝试回滚: " + installErr.Error())
		return
	}
	m.setState(StageRestarting, pkg.Version, "")
	if err := m.restartAndHealth(); err != nil {
		m.rollbackPkg(pkg.Format)
		fail("服务重启失败，已回滚到原版本: " + err.Error())
		return
	}
	m.setState(StageUpstaging, pkg.Version, "")
}

// installTarPath 解压本地 tar.gz 后复用二进制替换逻辑安装。
func (m *Manager) installTarPath(path string) error {
	tmp, err := os.MkdirTemp("", upgradeTmpPrefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	extractDir := filepath.Join(tmp, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	if err := untar(path, extractDir); err != nil {
		return err
	}
	return m.install(extractDir) // 复用备份→替换逻辑
}

// installPkg 通过系统包管理器安装 deb/rpm。安装前备份当前二进制与前端供回滚。
func (m *Manager) installPkg(path, format string) error {
	if err := os.MkdirAll(m.installDir, 0o755); err != nil {
		return err
	}
	curBin := filepath.Join(m.installDir, "edgeCore")
	prevBin := curBin + ".prev"
	if _, err := os.Stat(curBin); err == nil {
		if err := copyFile(curBin, prevBin, 0o755); err != nil {
			return fmt.Errorf("备份当前二进制失败: %w", err)
		}
	}
	curUI := filepath.Join(m.installDir, "ui", "dist")
	prevUI := curUI + ".prev"
	if st, err := os.Stat(curUI); err == nil && st.IsDir() {
		_ = os.RemoveAll(prevUI)
		if err := os.Rename(curUI, prevUI); err != nil {
			return fmt.Errorf("备份前端目录失败: %w", err)
		}
	}

	var out []byte
	var err error
	switch format {
	case "deb":
		out, err = m.execFn("dpkg", "--force-confnew", "--force-confmiss", "-i", path).CombinedOutput()
	case "rpm":
		out, err = m.execFn("rpm", "-Uvh", path).CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("%s 安装失败: %s: %s", format, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// rollbackPkg 依据格式恢复备份：tar.gz 走通用 rollback，deb/rpm 恢复备份文件并重启。
func (m *Manager) rollbackPkg(format string) {
	if format == "tar.gz" {
		m.rollback()
		return
	}
	curBin := filepath.Join(m.installDir, "edgeCore")
	if _, err := os.Stat(curBin + ".prev"); err == nil {
		_ = copyFileAtomic(curBin+".prev", curBin, 0o755)
	}
	curUI := filepath.Join(m.installDir, "ui", "dist")
	prevUI := curUI + ".prev"
	if _, err := os.Stat(prevUI); err == nil {
		_ = os.RemoveAll(curUI)
		_ = os.Rename(prevUI, curUI)
	}
	_ = m.execFn("systemctl", "restart", m.serviceName).Run()
}