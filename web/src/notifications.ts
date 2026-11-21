// Notification helpers (B12): deep-link targets, the kinds that interrupt
// with a quiet toast, and Today/Earlier grouping. The server owns the text;
// the desk decides where each item leads and how loudly it arrives.
import type { Notification } from './gen/athena/v1/notifications_pb'

export type NotificationTarget = { kind: 'record'; recordType: string; recordId: string } | { kind: 'page'; page: 'access' | 'ledger' }

// Only these interrupt with a toast; the rest wait silently in the inbox.
const ACTION_KINDS = new Set(['save_stopped', 'account_locked', 'sign_in_failed'])

export function needsAction(kind: string): boolean {
  return ACTION_KINDS.has(kind)
}

// Kind-first mapping, so a Record Type that happens to be named user,
// session or period can never steal a system link.
export function notificationTarget(item: Pick<Notification, 'kind' | 'subjectType' | 'subjectId'>): NotificationTarget {
  switch (item.kind) {
    case 'save_stopped':
      return { kind: 'record', recordType: item.subjectType, recordId: item.subjectId }
    case 'period_closed':
      return { kind: 'page', page: 'ledger' }
    default:
      return { kind: 'page', page: 'access' }
  }
}

function startOfToday(): number {
  const now = new Date()
  now.setHours(0, 0, 0, 0)
  return now.getTime()
}

export function groupNotifications<T extends { createdAt: string }>(items: T[]): { today: T[]; earlier: T[] } {
  const boundary = startOfToday()
  const today: T[] = []
  const earlier: T[] = []
  for (const item of items) {
    const at = new Date(item.createdAt).getTime()
    if (!Number.isNaN(at) && at >= boundary) today.push(item)
    else earlier.push(item)
  }
  return { today, earlier }
}
