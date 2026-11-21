// One clock for the desk: point-in-time facts read "Today 1:36 PM",
// "Yesterday" or "21 Nov 2026". Audit logs keep full stamps; facts carry the
// full stamp with seconds in a tooltip.
export function formatWhen(iso: string, now: Date = new Date()): string {
  if (!iso) return ''
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''
  const day = (value: Date): number => new Date(value.getFullYear(), value.getMonth(), value.getDate()).getTime()
  const diffDays = Math.round((day(now) - day(at)) / 86400000)
  if (diffDays === 0) return `Today ${at.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}`
  if (diffDays === 1) return 'Yesterday'
  return `${at.getDate()} ${at.toLocaleDateString(undefined, { month: 'short' })} ${at.getFullYear()}`
}

export function formatWhenFull(iso: string): string {
  if (!iso) return ''
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''
  return at.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'medium' })
}
