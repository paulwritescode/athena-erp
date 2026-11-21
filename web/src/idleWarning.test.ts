// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  WARN_BEFORE_MS,
  clearIdleWarning,
  idleWarnBusy,
  idleWarnError,
  idleWarnOpen,
  noteAlive,
  resetIdleWarningForTests,
  rescheduleIdleWarning,
  staySignedIn,
} from './idleWarning'
import { authClient } from './api'

vi.mock('./api', () => ({
  authClient: { getSession: vi.fn(), touch: vi.fn() },
}))
vi.mock('./sessionChannel', () => ({ postSessionMessage: vi.fn() }))

afterEach(() => {
  vi.useRealTimers()
  resetIdleWarningForTests()
  vi.restoreAllMocks()
})

function isoFromNow(ms: number): string {
  return new Date(Date.now() + ms).toISOString()
}

describe('idleWarning', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-21T12:00:00Z'))
  })

  it('opens two minutes before idle expiry', () => {
    rescheduleIdleWarning(isoFromNow(10 * 60 * 1000))
    expect(idleWarnOpen.value).toBe(false)
    vi.advanceTimersByTime(8 * 60 * 1000)
    expect(idleWarnOpen.value).toBe(true)
  })

  it('opens at once when expiry is already near and ignores bad input', () => {
    rescheduleIdleWarning(isoFromNow(30 * 1000))
    expect(idleWarnOpen.value).toBe(true)
    clearIdleWarning()
    rescheduleIdleWarning('')
    rescheduleIdleWarning('not-a-date')
    vi.advanceTimersByTime(WARN_BEFORE_MS * 2)
    expect(idleWarnOpen.value).toBe(false)
  })

  it('rescheduling replaces the timer and standing down closes', async () => {
    vi.mocked(authClient.getSession).mockResolvedValue({ session: { authenticated: true, idleExpiresAt: isoFromNow(60 * 60 * 1000) } } as never)
    rescheduleIdleWarning(isoFromNow(10 * 60 * 1000))
    rescheduleIdleWarning(isoFromNow(60 * 60 * 1000))
    vi.advanceTimersByTime(10 * 60 * 1000)
    expect(idleWarnOpen.value).toBe(false)
    clearIdleWarning()
    rescheduleIdleWarning(isoFromNow(60 * 1000))
    expect(idleWarnOpen.value).toBe(true)
    noteAlive()
    expect(idleWarnOpen.value).toBe(false)
    await vi.advanceTimersByTimeAsync(0)
    expect(authClient.getSession).toHaveBeenCalledTimes(1)
  })

  it('staying touches once and reschedules from the fresh expiry', async () => {
    vi.mocked(authClient.touch).mockResolvedValue({ session: { idleExpiresAt: isoFromNow(30 * 60 * 1000) } } as never)
    rescheduleIdleWarning(isoFromNow(60 * 1000))
    expect(idleWarnOpen.value).toBe(true)
    await staySignedIn()
    expect(authClient.touch).toHaveBeenCalledTimes(1)
    expect(authClient.getSession).not.toHaveBeenCalled()
    expect(idleWarnOpen.value).toBe(false)
    vi.advanceTimersByTime(20 * 60 * 1000)
    expect(idleWarnOpen.value).toBe(false)
    vi.advanceTimersByTime(8 * 60 * 1000 + 1000)
    expect(idleWarnOpen.value).toBe(true)
  })

  it('keeps the dialog open with an inline error when staying fails', async () => {
    vi.mocked(authClient.touch).mockRejectedValue(new Error('down'))
    rescheduleIdleWarning(isoFromNow(60 * 1000))
    await staySignedIn()
    expect(idleWarnOpen.value).toBe(true)
    expect(idleWarnBusy.value).toBe(false)
    expect(idleWarnError.value).toContain('Could not reach')
  })

  it('standing down re-reads the truth and rearms from the new expiry', async () => {
    vi.mocked(authClient.getSession).mockResolvedValue({ session: { authenticated: true, idleExpiresAt: isoFromNow(30 * 60 * 1000) } } as never)
    rescheduleIdleWarning(isoFromNow(60 * 1000))
    expect(idleWarnOpen.value).toBe(true)
    noteAlive()
    await vi.advanceTimersByTimeAsync(0)
    expect(authClient.getSession).toHaveBeenCalledTimes(1)
    // The new expiry pushed the warning out: nothing at the old mark.
    vi.advanceTimersByTime(10 * 60 * 1000)
    expect(idleWarnOpen.value).toBe(false)
  })

  it('reopens when the truth is still inside two minutes, throttled', async () => {
    vi.mocked(authClient.getSession).mockResolvedValue({ session: { authenticated: true, idleExpiresAt: isoFromNow(60 * 1000) } } as never)
    rescheduleIdleWarning(isoFromNow(60 * 1000))
    noteAlive()
    await vi.advanceTimersByTimeAsync(0)
    expect(authClient.getSession).toHaveBeenCalledTimes(1)
    expect(idleWarnOpen.value).toBe(true)
    // Mouse-only activity inside the throttle dismisses without another call.
    noteAlive()
    expect(idleWarnOpen.value).toBe(false)
    await Promise.resolve()
    expect(authClient.getSession).toHaveBeenCalledTimes(1)
  })

  it('stays stood down when the recheck finds a dead session', async () => {
    vi.mocked(authClient.getSession).mockRejectedValue(new Error('idle'))
    rescheduleIdleWarning(isoFromNow(60 * 1000))
    noteAlive()
    await vi.advanceTimersByTimeAsync(0)
    expect(authClient.getSession).toHaveBeenCalledTimes(1)
    expect(idleWarnOpen.value).toBe(false)
  })
})
