const DEFAULT_OPTS = {
  token: '',
  enabled: true,
  minSizeMB: 0,
  sendCookies: true,
  fallback: true,
  blacklist: '',
}

const $ = (id) => document.getElementById(id)
const msg = $('msg')

function showMsg(text, ok) {
  msg.textContent = text
  msg.className = ok ? 'ok' : 'err'
}

async function load() {
  const { opts } = await chrome.storage.local.get('opts')
  const o = { ...DEFAULT_OPTS, ...(opts || {}) }
  $('token').value = o.token
  $('enabled').checked = o.enabled
  $('sendCookies').checked = o.sendCookies
  $('fallback').checked = o.fallback
  $('minSizeMB').value = o.minSizeMB
  $('blacklist').value = o.blacklist
}

async function save(patch) {
  const { opts } = await chrome.storage.local.get('opts')
  await chrome.storage.local.set({ opts: { ...DEFAULT_OPTS, ...(opts || {}), ...patch } })
}

$('saveToken').addEventListener('click', async () => {
  await save({ token: $('token').value.trim() })
  showMsg('token 已保存', true)
})

$('saveOpts').addEventListener('click', async () => {
  await save({
    enabled: $('enabled').checked,
    sendCookies: $('sendCookies').checked,
    fallback: $('fallback').checked,
    minSizeMB: Math.max(0, Number($('minSizeMB').value) || 0),
    blacklist: $('blacklist').value,
  })
  showMsg('设置已保存', true)
})

$('test').addEventListener('click', async () => {
  await save({ token: $('token').value.trim() })
  showMsg('检测中…', true)
  const res = await chrome.runtime.sendMessage({ type: 'test' })
  if (res && res.ok) {
    showMsg(`连接成功（端口 ${res.port}），token 有效`, true)
  } else {
    showMsg(`连接失败：${(res && res.error) || '未知错误'}`, false)
  }
})

load()
