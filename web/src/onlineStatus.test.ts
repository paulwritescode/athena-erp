// The offline page follows the browser's own online state both ways.
// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { useOnlineStatus } from './composables/useOnlineStatus'

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, 'onLine', { value, configurable: true })
}

describe('useOnlineStatus', () => {
  it('starts from navigator.onLine', () => {
    setOnline(false)
    // Note: the composable runs without a component instance, so Vue logs
    // its expected onUnmounted warning; the listeners still attach.
    const { online } = useOnlineStatus()
    expect(online.value).toBe(false)
  })

  it('drops and returns with the browser', () => {
    setOnline(true)
    const { online } = useOnlineStatus()
    expect(online.value).toBe(true)
    setOnline(false)
    window.dispatchEvent(new Event('offline'))
    expect(online.value).toBe(false)
    setOnline(true)
    window.dispatchEvent(new Event('online'))
    expect(online.value).toBe(true)
  })
})
