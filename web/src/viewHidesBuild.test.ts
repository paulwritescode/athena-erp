// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import CommandPalette from './components/shell/CommandPalette.vue'
import RecordsPage from './pages/RecordsPage.vue'
import { pageEntry } from './pages/registry'

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
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

async function flush() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

// View hides all Build navigation (B8): the palette drops its Build group
// in View, and the empty Records page offers no way back to builders-only
// Build for users without the build permission.
describe('View hides Build navigation', () => {
  async function mountPalette(mode: 'build' | 'view') {
    const open = ref(true)
    const el = document.createElement('div')
    document.body.appendChild(el)
    const app = createApp({
      setup: () => () => h(CommandPalette, {
        buildEntries: [pageEntry('access')!],
        recordTypes: [],
        isDark: false,
        mode,
        open: open.value,
        'onUpdate:open': (value: boolean) => { open.value = value },
      }),
    })
    app.mount(el)
    cleanups.push(() => app.unmount())
    await flush()
  }

  it('shows the Build group in Build mode only', async () => {
    await mountPalette('build')
    expect(document.body.textContent ?? '').toContain('Access')
    for (const cleanup of cleanups.splice(0)) cleanup()
    document.body.innerHTML = ''
    await mountPalette('view')
    const text = document.body.textContent ?? ''
    expect(text).not.toContain('Access')
    expect(text).toContain('Work queue')
  })

  it('gates the empty Records page on the build permission', async () => {
    async function mountRecords(canBuild: boolean) {
      const el = document.createElement('div')
      document.body.appendChild(el)
      const app = createApp({
        setup: () => () => h(RecordsPage, { recordType: null, recordTypeKey: '', recordId: '', canBuild }),
      })
      app.mount(el)
      cleanups.push(() => app.unmount())
      await flush()
    }
    await mountRecords(true)
    expect(document.body.textContent ?? '').toContain('Go to Build')
    for (const cleanup of cleanups.splice(0)) cleanup()
    document.body.innerHTML = ''
    await mountRecords(false)
    const text = document.body.textContent ?? ''
    expect(text).toContain('Nothing to view yet')
    expect(text).not.toContain('Go to Build')
    expect(text).toContain('Lists appear here when someone shares them with you.')
  })
})
