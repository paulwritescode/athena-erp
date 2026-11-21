// Mode flips push exactly one history entry carrying the final address,
// including the resolved default Record Type.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useDeskRoute } from './composables/useDeskRoute'

// Note: each case logs Vue's expected onUnmounted warning (the composable
// runs without a component instance); the popstate listener is stubbed.
function setupRoute() {
  return useDeskRoute()
}

type FakeWindow = {
  location: { pathname: string; search: string }
  pushes: string[]
  replaces: string[]
}

function fakeWindow(pathname = '/', search = ''): FakeWindow {
  const state: FakeWindow = { location: { pathname, search }, pushes: [], replaces: [] }
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
    addEventListener: vi.fn(),
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

describe('mode flip history', () => {
  it('loads without pushing', async () => {
    const win = fakeWindow('/build/gallery', '')
    useDeskRoute()
    await settled()
    expect(win.pushes).toEqual([])
  })

  it('flipping to View pushes once with the resolved list', async () => {
    fakeWindow('/build/gallery', '')
    const route = setupRoute()
    route.setDefaultTypeResolver(() => 'task')
    await settled()
    route.setMode('view')
    await settled()
    expect(route.mode.value).toBe('view')
    const win = (window as unknown as { location: { pathname: string } }).location
    expect(win.pathname).toBe('/view/task')
  })

  it('counts exactly one push per flip', async () => {
    const win = fakeWindow('/build/gallery', '')
    const route = setupRoute()
    route.setDefaultTypeResolver(() => 'task')
    await settled()
    const before = win.pushes.length
    route.setMode('view')
    await settled()
    expect(win.pushes.length - before).toBe(1)
    expect(win.pushes[win.pushes.length - 1]).toBe('/view/task')
    route.setMode('build')
    await settled()
    expect(win.pushes.length - before).toBe(2)
    expect(win.pushes[win.pushes.length - 1]).toBe('/build/gallery?record_type=task')
  })

  it('resolution sync replaces instead of pushing', async () => {
    const win = fakeWindow('/view', '')
    const route = setupRoute()
    await settled()
    route.resolveSelected('task')
    await settled()
    expect(win.pushes).toEqual([])
    expect(win.replaces).toEqual(['/view/task'])
  })
})
