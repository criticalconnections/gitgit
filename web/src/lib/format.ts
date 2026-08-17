export function timeAgo(ts?: number): string {
  if (!ts) return ""
  const s = Math.floor(Date.now() / 1000 - ts)
  if (s < 60) return "just now"
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} minute${m === 1 ? "" : "s"} ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} hour${h === 1 ? "" : "s"} ago`
  const d = Math.floor(h / 24)
  if (d === 1) return "yesterday"
  if (d < 30) return `${d} days ago`
  const mo = Math.floor(d / 30)
  if (mo < 12) return `${mo} month${mo === 1 ? "" : "s"} ago`
  return new Date(ts * 1000).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })
}

export function formatDate(ts?: number): string {
  if (!ts) return ""
  return new Date(ts * 1000).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  })
}

export function formatDateTime(ts?: number): string {
  if (!ts) return ""
  return new Date(ts * 1000).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export function formatBytes(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`
  return `${n} B`
}

export function shortSha(sha: string): string {
  return sha.length > 7 ? sha.slice(0, 7) : sha
}

export function duration(start?: number, end?: number): string {
  if (!start || !end || end < start) return ""
  const s = end - start
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}
