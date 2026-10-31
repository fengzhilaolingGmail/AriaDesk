const dot = document.getElementById('dot')
const statusText = document.getElementById('statusText')
const toggleEnabled = document.getElementById('toggleEnabled')
const btnReconnect = document.getElementById('btnReconnect')
const btnOptions = document.getElementById('btnOptions')

function renderStatus({ connected, port }) {
  dot.className = 'dot ' + (connected ? 'ok' : 'err')
  statusText.textContent = connected ? `已连接 AriaDesk（端口 ${port}）` : '未连接，请确认 AriaDesk 已运行'
}

async function refresh() {
  dot.className = 'dot'
  statusText.textContent = '检测中…'
  const res = await chrome.runtime.sendMessage({ type: 'status' })
  renderStatus(res || { connected: false })
}

async function loadToggle() {
  const { opts } = await chrome.storage.local.get('opts')
  const enabled = !opts || opts.enabled !== false
  toggleEnabled.classList.toggle('off', !enabled)
}

toggleEnabled.addEventListener('click', async () => {
  const { opts } = await chrome.storage.local.get('opts')
  const next = { ...(opts || {}), enabled: !(!opts || opts.enabled !== false) }
  await chrome.storage.local.set({ opts: next })
  toggleEnabled.classList.toggle('off', !next.enabled)
})

btnReconnect.addEventListener('click', async () => {
  dot.className = 'dot'
  statusText.textContent = '检测中…'
  const res = await chrome.runtime.sendMessage({ type: 'reconnect' })
  renderStatus(res || { connected: false })
})

btnOptions.addEventListener('click', () => chrome.runtime.openOptionsPage())

refresh()
loadToggle()
