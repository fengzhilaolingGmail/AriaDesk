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
