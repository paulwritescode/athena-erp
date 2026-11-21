// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { onSessionMessage, postSessionMessage, type SessionChannelMessage } from './sessionChannel'

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
})

describe('sessionChannel', () => {
  it('delivers posts to listeners on BroadcastChannel', async () => {
    const seen: SessionChannelMessage[] = []
    const stop = onSessionMessage((message) => { seen.push(message) })
    postSessionMessage('signed-out')
    postSessionMessage('activity')
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(seen.map((m) => m.kind)).toEqual(['signed-out', 'activity'])
    expect(typeof seen[0].at).toBe('number')
    stop()
  })

  it('ignores corrupt payloads', async () => {
    const seen: SessionChannelMessage[] = []
    const stop = onSessionMessage((message) => { seen.push(message) })
    new BroadcastChannel('athena-session').postMessage({ kind: 'bogus' })
    new BroadcastChannel('athena-session').postMessage('signed-out')
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(seen).toEqual([])
    stop()
  })

  it('falls back to the storage event without BroadcastChannel', async () => {
    vi.stubGlobal('BroadcastChannel', undefined)
    const seen: SessionChannelMessage[] = []
    const stop = onSessionMessage((message) => { seen.push(message) })
    postSessionMessage('permissions-changed')
    expect(JSON.parse(localStorage.getItem('athena-session-message') ?? '{}').kind).toBe('permissions-changed')
    window.dispatchEvent(new StorageEvent('storage', { key: 'athena-session-message', newValue: localStorage.getItem('athena-session-message') }))
    expect(seen.map((m) => m.kind)).toEqual(['permissions-changed'])
    window.dispatchEvent(new StorageEvent('storage', { key: 'other', newValue: '{}' }))
    expect(seen).toHaveLength(1)
    stop()
  })

  it('unsubscribing stops delivery', async () => {
    const seen: SessionChannelMessage[] = []
    const stop = onSessionMessage((message) => { seen.push(message) })
    stop()
    postSessionMessage('signed-out')
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(seen).toEqual([])
  })
})
