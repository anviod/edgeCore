package network

import "os/exec"

// netCmd 是本包所有网络管理命令（netsh / ip / nmcli / dhclient / ping / systemctl）
// 的统一构造入口。命令名均为固定字面量，参数以参数数组传递（不经 shell，无注入面）；
// iface.Name 来自系统接口枚举、IP/网关/超时等来自管理界面配置，均为受信输入。
// 网络配置功能本身必须携带可变参数，gosec G204 在此统一豁免并说明。
func netCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...) // #nosec G204 -- 见函数注释：命令名固定、参数数组传递、输入受信
}
