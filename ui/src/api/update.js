import request from '@/utils/request'

// 软件更新：检查新版本、触发一键升级、查询升级进度
// 后端路由：/api/system/update/*
export default {
  // 检查 GitHub 上是否存在新版本
  checkUpdate() {
    return request({
      url: '/api/system/update/check',
      method: 'get',
      silent: true
    })
  },

  // 触发一键自动升级（version 传空则自动升级到最新稳定版）
  performUpdate(version) {
    return request({
      url: '/api/system/update/perform',
      method: 'post',
      data: { version: version || '' },
      timeout: 15000
    })
  },

  // 查询升级进度
  updateStatus() {
    return request({
      url: '/api/system/update/status',
      method: 'get',
      silent: true
    })
  },

  // 上传本地安装包并校验（.tar.gz / .deb / .rpm）
  uploadPackage(formData) {
    return request({
      url: '/api/system/update/upload',
      method: 'post',
      data: formData,
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 60000
    })
  },

  // 使用已上传校验的安装包执行本地升级/降级
  installLocal(pkgPath) {
    return request({
      url: '/api/system/update/install-local',
      method: 'post',
      data: { path: pkgPath },
      timeout: 15000
    })
  }
}