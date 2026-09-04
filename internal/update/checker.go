package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	defaultOwner = "anviod"      // 仓库属主
	defaultRepo  = "edgeCore"    // 仓库名
	defaultAPI   = "https://api.github.com"
	checkTimeout = 5 * time.Second // 超时上限受制于 5s API 约束
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Asset 描述一个 Release 附件。
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release 描述一个 GitHub Release（取自 /releases/latest）。
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	HTMLURL     string  `json:"html_url"`
	PublishedAt string  `json:"published_at"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	Assets      []Asset `json:"assets"`
}

// Checker 负责查询 GitHub Releases 并挑选当前平台可用的升级归档。
type Checker struct {
	apiBase string
	owner   string
	repo    string
	client  httpDoer
}

// NewChecker 构造 GitHub 更新检查器。apiBase 为空时使用官方 API。
func NewChecker(apiBase string) *Checker {
	if apiBase == "" {
		apiBase = defaultAPI
	}
	return &Checker{
		apiBase: strings.TrimRight(apiBase, "/"),
		owner:   defaultOwner,
		repo:    defaultRepo,
		client:  &http.Client{Timeout: checkTimeout},
	}
}

// SetClient 允许注入自定义 HTTP 客户端（测试用）。
func (c *Checker) SetClient(cl httpDoer) { c.client = cl }

// FetchLatest 获取仓库最新（非 Draft/Pre-release）Release。
func (c *Checker) FetchLatest(ctx context.Context) (*Release, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.apiBase, c.owner, c.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "edgeCore-update-check") // GitHub API 要求 UA
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api status %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// ArchiveAsset 依据当前平台挑选可升级的 tar.gz 归档。
// 归档命名见 .goreleaser.yml archives.name_template：
//
//	edgeCore-{{.RawVersion}}-{{.Os}}-{{.Arch}}[{{.Arm}}].tar.gz
func (c *Checker) ArchiveAsset(rel *Release, targetVersion string) (*Asset, error) {
	suffix := fmt.Sprintf("-linux-%s.tar.gz", runtime.GOARCH)
	for i := range rel.Assets {
		if strings.HasSuffix(rel.Assets[i].Name, suffix) {
			return &rel.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("无匹配当前平台 linux/%s 的升级包（期望后缀 %q）", runtime.GOARCH, suffix)
}

// FindAsset 按文件名查找附件（用于定位 SHA256SUMS）。
func FindAsset(rel *Release, name string) *Asset {
	for i := range rel.Assets {
		if rel.Assets[i].Name == name {
			return &rel.Assets[i]
		}
	}
	return nil
}