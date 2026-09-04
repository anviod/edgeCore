package update

import (
	"strconv"
	"strings"
)

// 语义化版本比较结果
const (
	CmpLT = -1 // cur < latest（存在新版本）
	CmpEQ = 0  // 相等
	CmpGT = 1  // cur > latest
)

// normalizeVersion 规整版本串：去 v 前缀，拆分为主版本号段与预发布/提交后缀。
// 例如 "v0.1.0" -> ("0.1.0","")；"0.0.10-SNAPSHOT-b3a" -> ("0.0.10","SNAPSHOT-b3a")。
func normalizeVersion(v string) (main, pre string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v = strings.TrimPrefix(v, "V")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

// parseMain 将主版本号转成 3 段数字，缺失位补 0。
func parseMain(main string) [3]int {
	var out [3]int
	raw := strings.Split(main, ".")
	for i := 0; i < 3 && i < len(raw); i++ {
		if n, err := strconv.Atoi(raw[i]); err == nil {
			out[i] = n
		}
	}
	return out
}

// isPrerelease 判定该后缀是否属于预发布（SNAPSHOT/beta/alpha/rc 或带提交号）。
func isPrerelease(pre string) bool {
	lower := strings.ToLower(pre)
	return strings.Contains(lower, "snapshot") ||
		strings.Contains(lower, "beta") ||
		strings.Contains(lower, "alpha") ||
		strings.Contains(lower, "rc") ||
		strings.Contains(lower, "dev")
}

// CompareVersions 比较当前运行版本与目标（latest）版本。
// 规则：主版本号逐段比较；主版本相同时，稳定版(无预发布后缀)高于预发布版(SNAPSHOT/beta 等)。
// 返回 CmpLT 表示 latest 比 cur 新（存在更新）。
func CompareVersions(cur, latest string) int {
	curM, curP := normalizeVersion(cur)
	latM, latP := normalizeVersion(latest)
	c, l := parseMain(curM), parseMain(latM)
	for i := 0; i < 3; i++ {
		if c[i] != l[i] {
			if c[i] < l[i] {
				return CmpLT
			}
			return CmpGT
		}
	}
	curPre, latPre := isPrerelease(curP), isPrerelease(latP)
	switch {
	case !curPre && !latPre:
		return CmpEQ
	case curPre && !latPre:
		return CmpLT // 当前为预发布，latest 为稳定版 → 有更新
	case !curPre && latPre:
		return CmpGT
	default: // 两者都是预发布，按后缀字典序近似比较
		if curP != latP {
			if curP < latP {
				return CmpLT
			}
			return CmpGT
		}
		return CmpEQ
	}
}