// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import UserMenu from './components/shell/UserMenu.vue'
import { usePreview } from './composables/usePreview'

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
}

async function openCaption(user: { username: string; displayName: string; owner: boolean }) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({
    setup: () => () => h(UserMenu, {
      user: { ...user, systemPermissions: [] },
      isDark: false,
      themeMode: 'system',
      mode: 'build',
      canBuildDesk: true,
      preview: usePreview(),
      previewAllowed: false,
      previewDeniedReason: '',
    }),
  })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await flush()
  const trigger = document.querySelector(`button[aria-label^="Signed in as"]`) as HTMLElement
  trigger.click()
  await flush()
  return document.body.textContent ?? ''
}

describe('UserMenu caption', () => {
  it('calls the root account Root administrator', async () => {
    const text = await openCaption({ username: 'administrator', displayName: 'Administrator', owner: true })
    expect(text).toContain('Root administrator')
    expect(text).not.toContain('Instance owner')
  })

  it('keeps the owner and username captions for everyone else', async () => {
    const ownerText = await openCaption({ username: 'amy', displayName: 'Amy', owner: true })
    expect(ownerText).toContain('Instance owner')
    for (const cleanup of cleanups.splice(0)) cleanup()
    document.body.innerHTML = ''
    const plainText = await openCaption({ username: 'bob', displayName: '', owner: false })
    expect(plainText).toContain('bob')
    expect(plainText).not.toContain('Root administrator')
  })
})
