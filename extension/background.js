// AriaDesk 下载助手 —— background service worker
// 职责：端口扫描与缓存、下载拦截、磁力/种子推送、右键菜单

const PORT_MIN = 20000
const PORT_MAX = 20005

const DEFAULT_OPTS = {
  token: '',
  enabled: true,      // 接管浏览器下载总开关
  minSizeMB: 0,       // 小于该体积（MB）不接管，0 表示不限制
  sendCookies: true,  // 推送时附带站点 Cookie（PT/网盘等需登录资源必需）
  fallback: true,     // 推送失败时回退为浏览器自身下载
  blacklist: '',      // 域名黑名单，每行一个
}

// ---------- 设置 ----------

async function getOpts() {
  const stored = await chrome.storage.local.get('opts')
  return { ...DEFAULT_OPTS, ...(stored.opts || {}) }
}

// ---------- 端口发现 ----------

async function pingPort(port) {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), 800)
  try {
    const resp = await fetch(`http://127.0.0.1:${port}/api/ping`, { signal: ctrl.signal })
    if (!resp.ok) return false
    const data = await resp.json()
    return data && data.app === 'AriaDesk'
  } catch {
    return false
  } finally {
    clearTimeout(timer)
  }
}

async function discoverPort() {
  for (let p = PORT_MIN; p <= PORT_MAX; p++) {
    if (await pingPort(p)) {
      await chrome.storage.local.set({ port: p })
      return p
    }
  }
  await chrome.storage.local.remove('port')
  return null
}

async function getPort() {
  const { port } = await chrome.storage.local.get('port')
  return port || null
}

// ---------- API ----------

async function apiPost(path, body) {
  const opts = await getOpts()
  let port = await getPort()
  if (!port) port = await discoverPort()
  if (!port) throw new Error('未连接：未找到 AriaDesk，请确认软件已运行')

  const doPost = (p) => fetch(`http://127.0.0.1:${p}${path}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-AriaDesk-Token': opts.token,
    },
    body: JSON.stringify(body),
  })

  let resp
  try {
    resp = await doPost(port)
  } catch {
    // 端口可能已切换（软件重启后重新协商），重新扫描一次再试
    port = await discoverPort()
    if (!port) throw new Error('未连接：未找到 AriaDesk，请确认软件已运行')
    resp = await doPost(port)
  }

  if (resp.status === 401) throw new Error('配对 token 无效，请在插件设置中检查')
  const data = await resp.json().catch(() => ({}))
  if (!resp.ok) {
    if (data.code === 'ARIA2_NOT_RUNNING') throw new Error('aria2 未运行，请在 AriaDesk 中启动下载服务')
    throw new Error(data.error || `推送失败（HTTP ${resp.status}）`)
  }
  return data
}

// ---------- 推送 ----------

async function getCookieHeader(url) {
  try {
    const cookies = await chrome.cookies.getAll({ url })
    return cookies.map((c) => `${c.name}=${c.value}`).join('; ')
  } catch {
    return ''
  }
}

function notify(title, message) {
  chrome.notifications.create({
    type: 'basic',
    iconUrl: 'icons/icon.png',
    title,
    message: String(message).slice(0, 200),
  })
}

// 页面右上角浮动提示；tabId 缺省时投向当前活动标签页，
// 目标页无法注入 content script（如 chrome:// 页面）时静默失败
async function toastToTab(tabId, text, ok) {
  if (tabId == null) {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
    tabId = tab && tab.id
  }
  if (tabId == null) return
  try {
    await chrome.tabs.sendMessage(tabId, { type: 'ariadesk-toast', text: String(text).slice(0, 120), ok })
  } catch { /* 页面不可注入时忽略，失败场景还有系统通知兜底 */ }
}

function taskName(url) {
  if (url.startsWith('magnet:')) return '磁力链接任务'
  try {
    const base = decodeURIComponent(new URL(url).pathname.split('/').filter(Boolean).pop() || '')
    return base || url
  } catch {
    return url
  }
}

async function reportSuccess(tabId, text) {
  await toastToTab(tabId, text, true)
}

async function reportFailure(tabId, err) {
  const text = err && err.message ? err.message : String(err)
  notify('AriaDesk 推送失败', text)
  await toastToTab(tabId, `推送失败：${text}`, false)
}

async function pushDownload({ url, filename = '', referer = '', tabId = null }) {
  const opts = await getOpts()
  const cookies = opts.sendCookies ? await getCookieHeader(url) : ''
  await apiPost('/api/download', {
    url,
    filename,
    referer,
    cookies,
    userAgent: navigator.userAgent,
  })
  await reportSuccess(tabId, `已推送到 AriaDesk：${filename || taskName(url)}`)
}

function bufToBase64(buf) {
  const bytes = new Uint8Array(buf)
  let bin = ''
  const CHUNK = 0x8000
  for (let i = 0; i < bytes.length; i += CHUNK) {
    bin += String.fromCharCode.apply(null, bytes.subarray(i, i + CHUNK))
  }
  return btoa(bin)
}

// 种子必须由插件带 Cookie 抓取字节再推送，直接把 URL 给 aria2 会在需登录站点 403
async function pushTorrent(url, referer = '', tabId = null) {
  const resp = await fetch(url, { credentials: 'include' })
  if (!resp.ok) throw new Error(`获取种子失败（HTTP ${resp.status}）`)
  const buf = await resp.arrayBuffer()
  const name = decodeURIComponent(url.split('/').pop().split('?')[0] || '') || 'download.torrent'
  await apiPost('/api/torrent', { name, dataBase64: bufToBase64(buf) })
  await reportSuccess(tabId, `已推送种子任务：${name}`)
}

function isTorrentUrl(url) {
  try {
    return new URL(url).pathname.toLowerCase().endsWith('.torrent')
  } catch {
    return false
  }
}

// ---------- 下载拦截 ----------

function hostInBlacklist(url, blacklist) {
  let host
  try {
    host = new URL(url).hostname
  } catch {
    return false
  }
  return blacklist
    .split('\n')
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean)
    .some((d) => host === d || host.endsWith('.' + d))
}

chrome.downloads.onCreated.addListener(async (item) => {
  // 自己回退触发的下载不再拦截，避免死循环
  if (item.byExtensionId === chrome.runtime.id) return

  const url = item.finalUrl || item.url || ''
  if (!/^https?:/i.test(url)) return

  const opts = await getOpts()
  if (!opts.enabled) return
  if (hostInBlacklist(url, opts.blacklist)) return
  if (opts.minSizeMB > 0 && item.fileSize > 0 && item.fileSize < opts.minSizeMB * 1048576) return

  const torrent = isTorrentUrl(url)
  try {
    await chrome.downloads.cancel(item.id)
    await chrome.downloads.erase({ id: item.id })
  } catch {
    return // 取消失败说明下载已结束或被接管，不再处理
  }

  try {
    if (torrent) {
      await pushTorrent(url, item.referrer || '')
    } else {
      await pushDownload({ url, referer: item.referrer || '' })
    }
  } catch (e) {
    await reportFailure(null, e)
    if (opts.fallback && !torrent) {
      chrome.downloads.download({ url })
    }
  }
})

// ---------- 右键菜单 ----------

function setupContextMenu() {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({
      id: 'ariadesk-download',
      title: '用 AriaDesk 下载',
      contexts: ['link'],
    })
  })
}

chrome.runtime.onInstalled.addListener(setupContextMenu)
chrome.runtime.onStartup.addListener(setupContextMenu)

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId !== 'ariadesk-download' || !info.linkUrl) return
  const url = info.linkUrl
  const referer = (tab && tab.url) || ''
  const tabId = (tab && tab.id) || null
  try {
    if (url.startsWith('magnet:')) {
      await pushDownload({ url, referer, tabId })
    } else if (isTorrentUrl(url)) {
      await pushTorrent(url, referer, tabId)
    } else {
      await pushDownload({ url, referer, tabId })
    }
  } catch (e) {
    await reportFailure(tabId, e)
  }
})

// ---------- 消息（磁力拦截 / popup 状态） ----------

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg && msg.type === 'magnet' && typeof msg.url === 'string') {
    const tabId = sender && sender.tab && sender.tab.id
    pushDownload({ url: msg.url, referer: (sender && sender.url) || '', tabId })
      .catch((e) => reportFailure(tabId, e))
    return false
  }
  if (msg && msg.type === 'status') {
    (async () => {
      let port = await getPort()
      if (!port || !(await pingPort(port))) port = await discoverPort()
      sendResponse({ connected: !!port, port })
    })()
    return true
  }
  if (msg && msg.type === 'reconnect') {
    discoverPort().then((port) => sendResponse({ connected: !!port, port }))
    return true
  }
  if (msg && msg.type === 'test') {
    // 测试连接：ping 验证服务存在，再以空 body 请求鉴权接口验证 token
    // （400 = token 正确但参数错误，401 = token 错误）
    (async () => {
      const port = await discoverPort()
      if (!port) {
        sendResponse({ ok: false, error: '未找到 AriaDesk 服务（20000~20005）' })
        return
      }
      const opts = await getOpts()
      try {
        const resp = await fetch(`http://127.0.0.1:${port}/api/download`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-AriaDesk-Token': opts.token },
          body: '{}',
        })
        if (resp.status === 401) {
          sendResponse({ ok: false, error: 'token 无效' })
        } else {
          sendResponse({ ok: true, port })
        }
      } catch (e) {
        sendResponse({ ok: false, error: String(e) })
      }
    })()
    return true
  }
  return false
})
