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
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
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
