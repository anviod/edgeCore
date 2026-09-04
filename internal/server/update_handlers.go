package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/update"

	"github.com/gofiber/fiber/v2"
)

// handleUpdateCheck 检查 GitHub 上是否存在更新的稳定版本，供前端自动检测。
func (s *Server) handleUpdateCheck(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()
	data := fiber.Map{
		"current":    model.Version,
		"hasUpdate":  false,
		"checkError": "",
		"latest":     nil,
	}
	rel, err := s.updater.Checker().FetchLatest(ctx)
	if err != nil {
		data["checkError"] = err.Error()
		return c.JSON(fiber.Map{"code": "0", "data": data})
	}
	data["hasUpdate"] = update.CompareVersions(model.Version, rel.TagName) == update.CmpLT
	data["latest"] = fiber.Map{
		"tag":         rel.TagName,
		"name":        rel.Name,
		"body":        rel.Body,
		"htmlUrl":     rel.HTMLURL,
		"publishedAt": rel.PublishedAt,
		"prerelease":  rel.Prerelease,
	}
	return c.JSON(fiber.Map{"code": "0", "data": data})
}

// handlePerformUpdate 一键自动升级：触发后端下载→校验→替换→重启。
func (s *Server) handlePerformUpdate(c *fiber.Ctx) error {
	var req struct {
		Version string `json:"version"` // 传空则自动取最新稳定版本
	}
	_ = c.BodyParser(&req)

	if s.updater.IsRunning() {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "升级已在进行中，请稍候"})
	}

	target := req.Version
	if target == "" {
		ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
		defer cancel()
		rel, err := s.updater.Checker().FetchLatest(ctx)
		if err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "获取最新版本失败: " + err.Error()})
		}
		target = rel.TagName
	}
	if update.CompareVersions(model.Version, target) != update.CmpLT {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("当前版本 %s 无需升级到 %s", model.Version, target),
		})
	}
	if err := s.updater.Start(context.Background(), target); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"code": "0",
		"data": fiber.Map{"status": "started", "target_version": target},
	})
}

// handleUpdateStatus 返回升级进度，供前端轮询。
func (s *Server) handleUpdateStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"code": "0", "data": s.updater.Status()})
}

// handleUploadPackage 接收本地安装包并校验，不安装。支持 .tar.gz / .deb / .rpm。
func (s *Server) handleUploadPackage(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "缺少上传文件 (file)"})
	}
	// 仅取安全的基名，防止路径注入
	fileName := filepath.Base(fileHeader.Filename)
	if fileName == "." || fileName == "/" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "非法文件名"})
	}
	dst := filepath.Join(os.TempDir(), "edgeCore-upload-"+fileName)
	if err := c.SaveFile(fileHeader, dst); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "保存上传文件失败: " + err.Error()})
	}
	pkg, verr := s.updater.ValidateLocal(dst)
	if verr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "校验安装包失败: " + verr.Error()})
	}
	if !pkg.Compatible {
		return c.JSON(fiber.Map{
			"code": "0",
			"data": fiber.Map{"valid": false, "reason": pkg.Reason, "info": pkg},
		})
	}
	return c.JSON(fiber.Map{
		"code": "0",
		"data": fiber.Map{
			"valid":   true,
			"version": pkg.Version,
			"reason":  "",
			"info":    pkg,
		},
	})
}

// handleInstallLocal 使用已上传校验过的本地安装包执行升级/降级。
func (s *Server) handleInstallLocal(c *fiber.Ctx) error {
	var req struct {
		Path string `json:"path"`
	}
	_ = c.BodyParser(&req)
	if req.Path == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "缺少安装包路径"})
	}
	if s.updater.IsRunning() {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "升级已在进行中，请稍候"})
	}
	// 重新校验，确保路径对应真实可用的安装包
	pkg, verr := s.updater.ValidateLocal(req.Path)
	if verr != nil || !pkg.Compatible {
		reason := "安装包校验失败"
		if verr != nil {
			reason = verr.Error()
		} else if pkg.Reason != "" {
			reason = pkg.Reason
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": reason})
	}
	if err := s.updater.StartLocal(c.Context(), req.Path); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"code": "0",
		"data": fiber.Map{
			"status":         "started",
			"target_version": pkg.Version,
			"format":         pkg.Format,
			"arch":           pkg.Arch,
		},
	})
}