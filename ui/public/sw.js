/* 反缓存 Service Worker
 * 根因：历史部署曾向浏览器注册过缓存型 SW，其 CacheStorage 会长期以旧 bundle
 * 响应 /assets/* 请求，导致每次发布新版本都被旧缓存遮蔽、老用户看不到更新。
 * 本 SW 负责两条：
 *   1) install 时清空本域全部缓存（caches.keys → delete），解除旧缓存对资产的劫持；
 *   2) fetch 一律不参与缓存，直接走网络，保证网关前端永远拉取最新构建产物。
 * 部署机制：本文件位于 ui/public/sw.js，vite 构建后原样复制到 dist/sw.js；
 * edgeCore 静态服务 Static("/", uiDist) 会精确命中该真实文件，从而绕过其
 * SPA 回退（Get("*")→index.html），向浏览器返回这一段合法的 worker 脚本。 */
self.addEventListener('install', function (event) {
  self.skipWaiting()
  event.waitUntil(
    caches.keys().then(function (keys) {
      return Promise.all(
        keys.map(function (key) {
          return caches.delete(key)
        })
      )
    })
  )
})

self.addEventListener('activate', function (event) {
  event.waitUntil(self.clients.claim())
})

self.addEventListener('fetch', function () {
  // 不拦截、不缓存任何请求，交由浏览器默认网络行为，保证始终获取最新版本。
})