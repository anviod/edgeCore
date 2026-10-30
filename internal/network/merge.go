package network

import (
	"os"
	"path/filepath"

	"github.com/anviod/edgeCore/internal/model"
)

func writeSystemConfigFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".edgeCore.tmp"
	// #nosec G304 -- tmp 由本函数基于服务端网络配置文件路径派生（原子写临时文件），非外部输入
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	// P2 加固（2026-10-13 审查）：netplan/ifupdown/systemd 是网关现场唯一生效的
	// 网络配置，断电时 rename 已生效而数据仍在 page cache 会导致设备以错误
	// （或无）IP 启动、远程失联。tmp 落盘 fsync 后再 rename。
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// MergeConfiguredInterfaces overlays saved settings onto live interface data.
func MergeConfiguredInterfaces(live, configured []model.NetworkInterface) []model.NetworkInterface {
	cfgByName := make(map[string]model.NetworkInterface, len(configured))
	for _, iface := range configured {
		cfgByName[iface.Name] = iface
	}

	for i, liveIface := range live {
		cfg, ok := cfgByName[liveIface.Name]
		if !ok {
			continue
		}
		if cfg.InterfaceMetric > 0 {
			live[i].InterfaceMetric = cfg.InterfaceMetric
		}
		live[i].Enabled = cfg.Enabled
		if len(cfg.IPConfigs) > 0 {
			live[i].IPConfigs = cfg.IPConfigs
		}
		if len(cfg.Gateways) > 0 {
			live[i].Gateways = cfg.Gateways
		}
	}
	return live
}
