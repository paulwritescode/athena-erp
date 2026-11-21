// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import PullToRefresh from './components/PullToRefresh.vue'

const cleanups: Array<() => void> = []

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

function touch(type: string, clientY: number): Event {
  const event = new Event(type, { bubbles: true })
  ;(event as unknown as { touches: Array<{ clientY: number }> }).touches = [{ clientY }]
  return event
}

function mountList() {
  const refreshing = ref(false)
  let refreshes = 0
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({
    render: () =>
      h(
        PullToRefresh,
        {
          refreshing: refreshing.value,
          'onUpdate:refreshing': (value: boolean) => {
            refreshing.value = value
          },
          onRefresh: () => {
            refreshes += 1
          },
        },
        { default: () => h('p', 'row') },
      ),
  })
  app.mount(el)
  cleanups.push(() => {
    app.unmount()
    el.remove()
  })
  return { el, count: () => refreshes }
}

describe('PullToRefresh', () => {
  it('refreshes on a downward pull from the top', async () => {
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 })
    const list = mountList()
    await nextTick()
    const root = list.el.firstElementChild as HTMLElement
    root.dispatchEvent(touch('touchstart', 100))
    root.dispatchEvent(touch('touchmove', 140))
    root.dispatchEvent(touch('touchmove', 180))
    await nextTick()
    expect(list.el.querySelector('[role="status"]')).not.toBeNull()
    root.dispatchEvent(touch('touchend', 180))
    expect(list.count()).toBe(1)
  })

  it('ignores short pulls, upward drags and mid-page pulls', async () => {
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 })
    const list = mountList()
    await nextTick()
    const root = list.el.firstElementChild as HTMLElement
    root.dispatchEvent(touch('touchstart', 100))
    root.dispatchEvent(touch('touchmove', 130))
    root.dispatchEvent(touch('touchend', 130))
    root.dispatchEvent(touch('touchstart', 200))
    root.dispatchEvent(touch('touchmove', 150))
    root.dispatchEvent(touch('touchend', 150))
    expect(list.count()).toBe(0)

    Object.defineProperty(window, 'scrollY', { configurable: true, value: 300 })
    root.dispatchEvent(touch('touchstart', 100))
    root.dispatchEvent(touch('touchmove', 300))
    root.dispatchEvent(touch('touchend', 300))
    expect(list.count()).toBe(0)
  })
})
