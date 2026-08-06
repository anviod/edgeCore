# edgeCore 文档 GitHub Pages 站点

## 简介

这是 edgeCore 项目的文档 GitHub Pages 站点（Jekyll + Cayman 主题），用于展示产品说明、用户手册、驱动文档与架构设计。站点首页内容见 [README_CN.md](README_CN.md)（与仓库根目录 README 结构对齐，路径为 docs 相对路径）。

## 站点结构

- `index.md` - 站点首页
- `_config.yml` - 站点配置文件
- `.gitignore` - Git 忽略文件
- `API/` - API 文档目录
- `guide/` - 产品指南（用户手册权威目录）
- `man/` - 历史索引（已合并至 guide/ 与 edge/，保留跳转）
- `img/` - 图片资源目录

## 本地预览

要在本地预览 GitHub Pages 站点，需要安装 Jekyll：

1. 安装 Ruby 3.0+ 和 RubyGems
2. 在 `docs` 目录中运行：
   ```bash
   bundle install
   bundle exec jekyll serve --baseurl /edgeCore
   ```
3. 打开浏览器访问 `http://localhost:4000/edgeCore/`

### 链接与资源校验（无需 Jekyll）

```bash
cd docs
bash scripts/verify_site.sh
```

脚本会审计全部内部 Markdown/HTML 链接，并检查首页索引与 `assets/` 静态资源是否齐全。

## 部署

GitHub Pages 从仓库指定分支的 `docs/` 目录构建（Jekyll + Cayman 主题，`baseurl: /edgeCore`）。部署步骤：

1. 将文档更改合并并推送到 Pages 源分支（通常为 `main` 或 `dev`，以仓库 Settings → Pages 为准）
2. 在仓库 Settings → Pages 中确认 Source 为 **Branch + `/docs` folder**
3. 等待 GitHub Pages 构建完成
4. 访问 [https://anviod.github.io/edgeCore/](https://anviod.github.io/edgeCore/)

### 发布范围

| 包含 | 排除（不发布） |
|------|----------------|
| 全部 `.md` 文档（含 `TODO/`、Q2/Q3 方案） | `done/` 历史归档副本 |
| `assets/`、`img/` 静态资源 | `man/` 已合并的历史索引 |
| `testing/_run_logs/` 回归日志 `.txt` | `scripts/` 构建/审计脚本 |

## 贡献

如果您想贡献文档，请：

1. Fork 仓库
2. 进行更改
3. 提交 Pull Request

## 联系我们

如有任何问题或建议，请通过 GitHub Issues 与我们联系。
