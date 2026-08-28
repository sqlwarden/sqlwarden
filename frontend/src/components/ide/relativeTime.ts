const shortDateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

const exactTimeFormatter = new Intl.DateTimeFormat(undefined, {
  dateStyle: 'medium',
  timeStyle: 'medium',
})

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR
const WEEK = 7 * DAY

/** Formats an ISO timestamp as "Just now" / "5m ago" / "3h ago" / "2d ago", falling back to a short date once it's a week or older. */
export function formatRelativeTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return 'Unknown'

  const diff = Date.now() - date.getTime()
  if (diff < MINUTE) return 'Just now'
  if (diff < HOUR) return `${Math.floor(diff / MINUTE)}m ago`
  if (diff < DAY) return `${Math.floor(diff / HOUR)}h ago`
  if (diff < WEEK) return `${Math.floor(diff / DAY)}d ago`
  return shortDateFormatter.format(date)
}

/** Formats an ISO timestamp as a full local date and time, e.g. "Aug 20, 2026, 4:12:33 PM". */
export function formatExactTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return 'Unknown'
  return exactTimeFormatter.format(date)
}

function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
}

/** Groups an ISO timestamp into "Today" / "Yesterday" / a short date, for sectioning lists by day. */
export function formatDateGroup(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return 'Unknown'

  const dayDiff = Math.round((startOfDay(new Date()) - startOfDay(date)) / DAY)
  if (dayDiff === 0) return 'Today'
  if (dayDiff === 1) return 'Yesterday'
  return shortDateFormatter.format(date)
}
