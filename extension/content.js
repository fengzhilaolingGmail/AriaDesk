// 拦截 magnet: 链接点击（downloads API 捕获不到磁力协议）
window.addEventListener(
  'click',
  (e) => {
    const a = e.target && e.target.closest ? e.target.closest('a[href^="magnet:"]') : null
    if (!a) return
    e.preventDefault()
    e.stopPropagation()
    chrome.runtime.sendMessage({ type: 'magnet', url: a.href })
  },
  true,
)

// 页面内推送结果提示：background 推送完成后发消息过来，右上角浮出 toast
chrome.runtime.onMessage.addListener((msg) => {
  if (msg && msg.type === 'ariadesk-toast') showToast(msg.text, msg.color || '#2e62f6')
})

function showToast(text, color) {
  const el = document.createElement('div')
  el.textContent = text
  Object.assign(el.style, {
    position: 'fixed',
    top: '16px',
    right: '16px',
    zIndex: '2147483647',
    padding: '10px 16px',
    borderRadius: '8px',
    fontSize: '13px',
    fontFamily: 'system-ui, "Microsoft YaHei", sans-serif',
    color: '#fff',
    background: color,
    boxShadow: '0 4px 12px rgba(0,0,0,.25)',
    maxWidth: '360px',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    pointerEvents: 'none',
    opacity: '0',
    transition: 'opacity .2s',
  })
  ;(document.body || document.documentElement).appendChild(el)
  requestAnimationFrame(() => {
    el.style.opacity = '1'
  })
  setTimeout(() => {
    el.style.opacity = '0'
    setTimeout(() => el.remove(), 300)
  }, 3000)
}
