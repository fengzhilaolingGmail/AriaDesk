export function toNum(v) {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}

export function formatSize(bytes) {
  const n = toNum(bytes)
  if (n < 1024) return `${Math.round(n)} B`
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`
  return `${(n / 1024 ** 3).toFixed(1)} GB`
}

export function formatSpeed(bytesPerSec) {
  return `${formatSize(bytesPerSec)}/s`
}

export function formatPercent(completed, total) {
  const t = toNum(total)
  if (t <= 0) return 0
  return Math.min(100, Math.round((toNum(completed) / t) * 100))
}

export function formatTime(v) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function taskName(task) {
  if (task?.name) return task.name
  const bt = task?.bittorrent?.info?.name
  if (bt) return bt
  const path = task?.files?.[0]?.path
  if (path) {
    const parts = path.split(/[\\/]/)
    const base = parts[parts.length - 1]
    if (base) return base
  }
  const uri = task?.files?.[0]?.uris?.[0]?.uri
  if (uri) return uri
  return task?.gid || ''
}
