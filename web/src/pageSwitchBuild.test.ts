// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import PageSwitch from './components/PageSwitch.vue'
import { useDeskRoute } from './composables/useDeskRoute'
import { usePreview } from './composables/usePreview'

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: () => ({
    matches: true,
    media: '',
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
    onchange: null,
  }),
})

// The hairline figures observe intersection; jsdom has no
// IntersectionObserver, so the figure load throws and latches the page
// switch error state. A no-op observer keeps the art mount quiet.
class NoopIntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
Object.defineProperty(window, 'IntersectionObserver', { configurable: true, value: NoopIntersectionObserver })
Object.defineProperty(globalThis, 'IntersectionObserver', { configurable: true, value: NoopIntersectionObserver })

async function flush() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

function fakeWindow(pathname: string) {
  const location = { pathname, search: '' }
  const apply = (url: string) => {
    const parsed = new URL(url, 'http://desk.test')
    location.pathname = parsed.pathname
    location.search = parsed.search
  }
  vi.stubGlobal('window', {
    location,
    history: {
      pushState: (_d: unknown, _t: string, url: string) => apply(url),
      replaceState: (_d: unknown, _t: string, url: string) => apply(url),
    },
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    matchMedia: window.matchMedia,
  })
}

// The Build caption and the Role preview panel (B8): Build always shows
// the quiet "Building" caption; an active preview replaces the page until
// the builder exits it.
describe('PageSwitch build chrome', () => {
  async function mountSwitch() {
    fakeWindow('/build/packs')
    const route = useDeskRoute()
    const session = {
      canBuild: ref(true),
      user: ref({ username: 'amy', displayName: 'Amy', owner: false, systemPermissions: ['build', 'access:roles'] }),
      databaseState: ref('Connected'),
      apiState: ref('connected'),
      canWriteRules: ref(false),
      canWriteFiles: ref(false),
      canWriteLedger: ref(false),
    }
    const library = {
      unknownSelectedType: ref(false),
      loaded: ref(true),
      loadFailed: ref(false),
      viewRecordTypes: ref([{ name: 'invoice', label: 'Invoice', fields: [{ name: 'title', permission_level: 0 }] }]),
      viewRecordType: ref(null),
      recordTypes: ref([]),
      currentRecordType: ref({}),
      pendingSave: ref(null),
    }
    const preview = usePreview()
    const el = document.createElement('div')
    document.body.appendChild(el)
    const app = createApp({ setup: () => () => h(PageSwitch, { route, session: session as never, library: library as never, preview }) })
    app.mount(el)
    cleanups.push(() => app.unmount())
    // Pages load asynchronously; wait for the Packs title to land.
    for (let i = 0; i < 50 && !document.querySelector('[data-page-main] h1'); i++) {
      await flush()
    }
    return { route, preview }
  }

  it('shows the Building caption above the page in Build', async () => {
    await mountSwitch()
    const main = document.querySelector('[data-page-main]')
    expect(main?.textContent ?? '').toContain('Building')
    expect(main?.querySelector('h1')?.textContent ?? '').toContain('Packs')
  })

  it('replaces the page with the Role preview until exit', async () => {
    const { preview } = await mountSwitch()
    preview.roles.value = [{ name: 'Clerk', grants: [{ recordTypeKey: '*', actions: ['read'], permissionLevel: 9 }] }]
    preview.enter('Clerk')
    await flush()
    let main = document.querySelector('[data-page-main]')
    expect(main?.textContent ?? '').toContain('View as Clerk')
    expect(main?.textContent ?? '').toContain('Invoice')
    expect(main?.querySelector('h1')?.textContent ?? '').not.toContain('Packs')
    preview.exit()
    await flush()
    main = document.querySelector('[data-page-main]')
    expect(main?.querySelector('h1')?.textContent ?? '').toContain('Packs')
  })

  it('hides the caption in View', async () => {
    const { route } = await mountSwitch()
    route.setMode('view')
    await flush()
    expect(document.querySelector('[data-page-main]')?.textContent ?? '').not.toContain('Building')
  })
})
