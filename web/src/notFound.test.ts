// The not-found address: nothing parses it, the bar keeps it, and any
// navigation leaves it. Bare addresses still read the legacy query form.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useDeskRoute } from './composables/useDeskRoute'

// Note: each case logs Vue's expected onUnmounted warning (the composable
// runs without a component instance); the popstate listener is captured
// from the stub so cases can drive it.
type FakeWindow = {
  location: { pathname: string; search: string }
  pushes: string[]
  replaces: string[]
  listeners: Record<string, () => void>
}

function fakeWindow(pathname = '/', search = ''): FakeWindow {
  const state: FakeWindow = { location: { pathname, search }, pushes: [], replaces: [], listeners: {} }
  const apply = (url: string) => {
    const parsed = new URL(url, 'http://desk.test')
    state.location.pathname = parsed.pathname
    state.location.search = parsed.search
  }
  vi.stubGlobal('window', {
    location: state.location,
    history: {
      pushState: (_data: unknown, _title: string, url: string) => {
        state.pushes.push(url)
        apply(url)
      },
      replaceState: (_data: unknown, _title: string, url: string) => {
        state.replaces.push(url)
        apply(url)
      },
    },
    addEventListener: vi.fn((name: string, handler: () => void) => {
      state.listeners[name] = handler
    }),
    removeEventListener: vi.fn(),
  })
  return state
}

afterEach(() => {
  vi.unstubAllGlobals()
})

async function settled(times = 3) {
  for (let i = 0; i < times; i += 1) {
    await nextTick()
  }
}

describe('not-found address', () => {
  it.each([['/no-such-page'], ['/build/no-such-page'], ['/frobnicate/deep/link']])('marks %s as not found', async (pathname) => {
    fakeWindow(pathname, '')
    const route = useDeskRoute()
    await settled()
    expect(route.notFound.value).toBe(true)
  })

  it('keeps the failed address in the bar', async () => {
    const win = fakeWindow('/no-such-page', '')
    useDeskRoute()
    await settled()
    expect(win.location.pathname).toBe('/no-such-page')
    expect(win.pushes).toEqual([])
    expect(win.replaces).toEqual([])
  })

  it('leaves bare addresses alone', async () => {
    for (const pathname of ['/', '/build', '/build/']) {
      vi.unstubAllGlobals()
      fakeWindow(pathname, '')
      const route = useDeskRoute()
      await settled()
      expect(route.notFound.value, pathname).toBe(false)
      expect(route.page.value, pathname).toBe('overview')
    }
  })

  it('clears on any navigation', async () => {
    fakeWindow('/no-such-page', '')
    const route = useDeskRoute()
    await settled()
    route.openBuildPage('overview')
    expect(route.notFound.value).toBe(false)
  })

  it('answers popstate both ways', async () => {
    const win = fakeWindow('/build/overview', '')
    const route = useDeskRoute()
    await settled()
    expect(route.notFound.value).toBe(false)
    win.location.pathname = '/no-such-page'
    win.listeners['popstate']()
    expect(route.notFound.value).toBe(true)
    win.location.pathname = '/build/overview'
    win.listeners['popstate']()
    expect(route.notFound.value).toBe(false)
  })
})
