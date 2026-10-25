// Synthetic data for local UI tests; this file never enters the production bundle.
(() => {
  const callbacks = new Map()
  window.runtime = {
    EventsOnMultiple(name, fn) { if (!callbacks.has(name)) callbacks.set(name, new Set()); callbacks.get(name).add(fn); return () => callbacks.get(name).delete(fn) },
    EventsEmit(name, payload) { for (const fn of callbacks.get(name) || []) fn(payload) },
    BrowserOpenURL() {},
  }
  const files = [
    { name: '已完成.mp4', path: 'D:\\fixture\\completed.mp4', state: 'done', dialogId: 7, messageId: 1, dialogType: 'supergroup', size: 1024 },
    { name: '重名视频.mp4', path: 'skipped://channel:7:2', state: 'skipped', reason: '来源匹配，已跳过', dialogId: 7, messageId: 2, dialogType: 'supergroup', size: 2048 },
    { name: '失败文件.zip', path: 'D:\\fixture\\failed.tmp', state: 'failed', reason: '合成测试错误', dialogId: 7, messageId: 3, dialogType: 'supergroup', size: 4096 },
  ]
  const items = [1, 2, 3].map(id => ({ dialogId: 7, messageId: id, mediaId: `document:${id}`, name: id < 3 ? '重名视频.mp4' : '文件.zip', size: id < 3 ? 2048 : 4096, date: 1700000000 + id * 86400, kind: 'file', mime: 'application/octet-stream', caption: '合成数据，无真实 Telegram 内容' }))
  let tasks = [{ id: 'fixture-task', label: '合成任务 · 成功/失败/跳过', dir: 'D:\\fixture', status: 'done', total: 3, finished: 1, failed: 1, skipped: 1, fileCount: 3, urls: [], scriptName: '', createdAt: '2026-10-25 10:00:00' }]
  tasks.push({ ...tasks[0], id: 'disk-limit-fixture', label: '磁盘保护测试', failed: 0, skipped: 0, fileCount: 0, notice: '下载目录所在磁盘已达到 90% 使用率上限，已停止下载。清理磁盘空间后，可导入清单继续。\n已下载清单：\nD:\\fixture\\tdl-media-channel-7-task.jsonl' })
  let settings = { namingPolicyVersion: 1, proxy: '', proxyMode: 'off', diskUsageLimit: 90, downloadDir: 'D:\\fixture', template: '{{ filenamify .FileName }}', conflictAction: 'skip', conflictCheck: 'origin', threads: 4, limit: 2, poolSize: 8, theme: 'light', exitAction: 'prompt', cacheDir: '', tempDir: '', loggedInUserId: 42, loggedInUsername: 'synthetic-user', enabledScripts: [], recentDirs: {}, ui: { logFontSize: 14, scrollbarSize: 10, maxLogLines: 128 }, log: { targets: 'ui', level: 'info', dir: 'D:\\fixture\\logs', format: '', maxSizeMb: 10, maxAgeDays: 7, maxBackups: 10 } }
  const info = { path: 'D:\\fixture\\manifest.json', header: { version: 2, accountId: 42, dialogId: 7, dialogType: 'supergroup', title: '合成媒体对话', createdAt: '2026-10-25' }, entries: [], total: 0, done: 0, ignored: 0 }
  const comparison = { token: 'synthetic-review', total: 3, matched: 0, unmatched: 1, ambiguous: 2, matches: [], duplicates: [{ name: '重名视频.mp4', entries: items.slice(0, 2), files: [{ path: '重名视频.mp4', size: 2048, date: 1700000000 }] }] }
  window.qaActions = []
  function call(service, name, args) {
    window.qaActions.push({ service, name, args })
    if (service === 'AuthService' && name === 'Status') return { loggedIn: true, userId: 42, username: 'synthetic-user', sessionPresent: true }
    if (service === 'SettingsService') {
      if (name === 'Get') return settings
      if (name === 'Save') { settings = args[0]; return null }
      if (name === 'DataDir') return 'D:\\fixture\\tdl-ui-data'
      if (name === 'IsPortable') return true
      if (name === 'RecentDirs' || name === 'AddRecentDir') return []
      return null
    }
    if (service === 'ChatService') {
      if (name === 'ListDialogs') return [{ id: 42, title: '收藏夹', type: 'private', isSelf: true, unreadCount: 0 }, { id: 7, title: '合成媒体对话', type: 'supergroup', unreadCount: 2 }, { id: 7, title: '同 ID 普通群组', type: 'group', unreadCount: 0 }]
      if (name === 'ListMedia') return { items, nextOffset: 0 }
      return null
    }
    if (service === 'DownloadService') {
      if (name === 'ListTasks') return tasks
      if (name === 'ListTaskFiles') return files
      if (name === 'GetTotalSpeed') return 0
      if (name === 'ListDownloadedMessages') return []
      if (name === 'SelectDirectory') return 'D:\\fixture'
      if (name === 'ReviewLocalMedia' || name === 'CompareLocalMedia') return comparison
      if (name === 'PreviewDownload') return { total: args[3].length, manifestSkipped: 0, localMatched: 0, pending: args[3].length, manifestMessageIds: [], comparison }
      if (['ChooseManifest', 'ReadManifest', 'NewDownloadManifest', 'ExportReviewedManifest'].includes(name)) return info
      if (name === 'CreateTask') { const task = { ...tasks[0], id: 'created-task', label: args[0].label, status: 'queued', failed: 0, finished: 0, skipped: 0 }; tasks = [task, ...tasks]; window.runtime.EventsEmit('task:update', task); return task.id }
      return null
    }
    if (name === 'List' || name === 'Templates' || name === 'GetRecent' || name === 'ListLogFiles') return []
    return null
  }
  window.go = { services: Object.fromEntries(['AuthService', 'ChatService', 'DownloadService', 'SettingsService', 'ScriptService', 'LogService'].map(service => [service, new Proxy({}, { get: (_, name) => (...args) => Promise.resolve(call(service, name, args)) })])) }
  document.addEventListener('DOMContentLoaded', () => {
    const button = document.createElement('button'); button.textContent = '测试关闭窗口'; button.style = 'position:fixed;bottom:8px;left:8px;z-index:9999;font-size:12px'; button.onclick = () => window.runtime.EventsEmit('app:before_close'); document.body.append(button)
  })
})()
