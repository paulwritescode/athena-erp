// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { POLL_MS, useNotifications } from './composables/useNotifications'
import { notificationClient } from './api'
import { toast } from 'vue-sonner'

vi.mock('./api', () => ({
  notificationClient: { unreadCount: vi.fn(), list: vi.fn(), markRead: vi.fn(), markAllRead: vi.fn() },
}))
vi.mock('vue-sonner', () => ({ toast: { info: vi.fn(), error: vi.fn(), success: vi.fn() } }))

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

function item(id: string, kind: string, readAt = '') {
  return { id, kind, subjectType: 'note', subjectId: 'r1', subjectLabel: 'N', actorLabel: '', text: `${kind} happened`, createdAt: new Date().toISOString(), readAt }
}

describe('useNotifications', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.mocked(notificationClient.unreadCount).mockResolvedValue({ count: BigInt(2) } as never)
    vi.mocked(notificationClient.list).mockResolvedValue({ notifications: [], nextPageToken: '' } as never)
  })

  it('polls the count every 30 s while visible and pauses when hidden', async () => {
    const inbox = useNotifications()
    inbox.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(notificationClient.unreadCount).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(notificationClient.unreadCount).toHaveBeenCalledTimes(2)
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    await vi.advanceTimersByTimeAsync(POLL_MS * 3)
    expect(notificationClient.unreadCount).toHaveBeenCalledTimes(2)
    Object.defineProperty(document, 'hidden', { configurable: true, value: false })
    inbox.stop()
  })

  it('toasts only new arrivals that need action, never the baseline', async () => {
    vi.mocked(notificationClient.list)
      .mockResolvedValueOnce({ notifications: [item('old', 'save_stopped')], nextPageToken: '' } as never)
      .mockResolvedValue({ notifications: [item('old', 'save_stopped'), item('new-info', 'invite_accepted'), item('new-act', 'save_stopped')], nextPageToken: '' } as never)
    const inbox = useNotifications()
    inbox.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(toast.info).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(toast.info).toHaveBeenCalledTimes(1)
    expect(toast.info).toHaveBeenCalledWith('save_stopped happened')
    inbox.stop()
  })

  it('opens the inbox, pages, and tracks read state locally', async () => {
    vi.mocked(notificationClient.list).mockResolvedValue({ notifications: [item('a', 'period_closed'), item('b', 'access_changed', '2026-01-01T00:00:00Z')], nextPageToken: 't' } as never)
    vi.mocked(notificationClient.markRead).mockResolvedValue({ changed: BigInt(1) } as never)
    const inbox = useNotifications()
    await inbox.openInbox()
    expect(inbox.items.value).toHaveLength(2)
    expect(inbox.nextToken.value).toBe('t')
    await inbox.markRead(['a'])
    expect(notificationClient.markRead).toHaveBeenCalledWith({ ids: ['a'] })
    expect(inbox.items.value.find((i) => i.id === 'a')?.readAt).not.toBe('')
    // Already-read ids never reach the server.
    await inbox.markRead(['b'])
    expect(notificationClient.markRead).toHaveBeenCalledTimes(1)
    inbox.stop()
  })

  it('marks everything read and zeroes the badge', async () => {
    vi.mocked(notificationClient.markAllRead).mockResolvedValue({ changed: BigInt(2) } as never)
    const inbox = useNotifications()
    await inbox.markAllRead()
    expect(inbox.unread.value).toBe(0)
    inbox.stop()
  })
})
