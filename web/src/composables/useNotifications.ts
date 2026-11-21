// The notification inbox state (B12): unread count polled every 30 seconds
// while the tab is visible, the list grouped Today/Earlier, read tracking,
// and one quiet toast per new arrival that needs action. The first load
// only sets the baseline, so old items never toast.
import { computed, onUnmounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { notificationClient } from '../api'
import { groupNotifications, needsAction, type NotificationTarget, notificationTarget } from '../notifications'
import type { Notification } from '../gen/athena/v1/notifications_pb'

export const POLL_MS = 30000

export function useNotifications() {
  const unread = ref(0)
  const items = ref<Notification[]>([])
  const nextToken = ref('')
  const loaded = ref(false)
  const loadingMore = ref(false)
  const loadError = ref('')
  const inboxOpen = ref(false)
  const knownIds = new Set<string>()
  const primed = ref(false)
  let timer: ReturnType<typeof setInterval> | undefined

  const groups = computed(() => groupNotifications(items.value))

  async function pollCount(): Promise<void> {
    try {
      const res = await notificationClient.unreadCount({})
      unread.value = Number(res.count)
    } catch {
      // The bell keeps its last count; the inbox shows errors on open.
    }
  }

  async function pollList(): Promise<void> {
    // New arrivals toast only when they need action, and never on the
    // first load: the baseline establishes silently.
    try {
      const res = await notificationClient.list({ pageSize: 30, pageToken: '', unreadOnly: false })
      const fresh = res.notifications
      if (primed.value) {
        for (const item of fresh) {
          if (!knownIds.has(item.id)) {
            knownIds.add(item.id)
            if (!item.readAt && needsAction(item.kind)) toast.info(item.text)
          }
        }
      } else {
        for (const item of fresh) knownIds.add(item.id)
        primed.value = true
      }
      if (inboxOpen.value) {
        items.value = fresh
        nextToken.value = res.nextPageToken
      }
    } catch {
      // Same as the count: stay quiet until the inbox opens.
    }
  }

  async function poll() {
    // Polling pauses in hidden tabs: unseen badges need no refresh, and the
    // quiet tab lets a truly idle session lapse.
    if (document.hidden) return
    await pollCount()
    await pollList()
  }

  function onVisible() {
    if (!document.hidden) void poll()
  }

  function start() {
    stop()
    document.addEventListener('visibilitychange', onVisible)
    timer = setInterval(() => void poll(), POLL_MS)
    void poll()
  }

  function stop() {
    if (timer !== undefined) clearInterval(timer)
    timer = undefined
    document.removeEventListener('visibilitychange', onVisible)
  }

  async function openInbox() {
    inboxOpen.value = true
    loaded.value = false
    loadError.value = ''
    try {
      const res = await notificationClient.list({ pageSize: 30, pageToken: '', unreadOnly: false })
      items.value = res.notifications
      nextToken.value = res.nextPageToken
      for (const item of res.notifications) knownIds.add(item.id)
      primed.value = true
    } catch {
      loadError.value = 'Could not load the notifications.'
    } finally {
      loaded.value = true
    }
  }

  function closeInbox() {
    inboxOpen.value = false
  }

  async function showOlder() {
    if (!nextToken.value || loadingMore.value) return
    loadingMore.value = true
    try {
      const res = await notificationClient.list({ pageSize: 30, pageToken: nextToken.value, unreadOnly: false })
      items.value = [...items.value, ...res.notifications]
      nextToken.value = res.nextPageToken
      for (const item of res.notifications) knownIds.add(item.id)
    } catch {
      loadError.value = 'Could not load older notifications.'
    } finally {
      loadingMore.value = false
    }
  }

  async function markRead(ids: string[]) {
    const pending = ids.filter((id) => items.value.some((item) => item.id === id && !item.readAt))
    if (pending.length === 0) return
    try {
      const res = await notificationClient.markRead({ ids: pending })
      const now = new Date().toISOString()
      items.value = items.value.map((item) => (pending.includes(item.id) ? { ...item, readAt: item.readAt || now } : item))
      unread.value = Math.max(0, unread.value - Number(res.changed))
    } catch {
      toast.error('Could not mark that read.')
    }
  }

  async function markAllRead() {
    try {
      await notificationClient.markAllRead({})
      const now = new Date().toISOString()
      items.value = items.value.map((item) => ({ ...item, readAt: item.readAt || now }))
      unread.value = 0
    } catch {
      toast.error('Could not mark everything read.')
    }
  }

  function targetFor(item: Notification): NotificationTarget {
    return notificationTarget(item)
  }

  onUnmounted(stop)

  return { unread, items, groups, nextToken, loaded, loadingMore, loadError, inboxOpen, start, stop, openInbox, closeInbox, showOlder, markRead, markAllRead, targetFor }
}

export type NotificationsState = ReturnType<typeof useNotifications>
