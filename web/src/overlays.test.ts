// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import DetailSheet from './components/DetailSheet.vue'

const cleanups: (() => void)[] = []

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

function studioMatchMedia(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: (query: string) => ({
      matches,
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
      onchange: null,
    }),
  })
}

function mountSheet(initial = true) {
  studioMatchMedia(false)
  const open = ref(initial)
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({
    render: () =>
      h(
        DetailSheet,
        {
          open: open.value,
          'onUpdate:open': (next: boolean) => {
            open.value = next
          },
          title: 'Record',
          description: 'Details',
        },
        { default: () => 'body', footer: () => 'foot' },
      ),
  })
  app.mount(el)
  cleanups.push(() => {
    app.unmount()
    el.remove()
  })
  return { el, open }
}

describe('DetailSheet', () => {
  it('opens and closes through the model', async () => {
    const { open } = mountSheet(true)
    await nextTick()
    expect(document.body.textContent).toContain('Record')
    open.value = false
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(document.body.textContent).not.toContain('Record')
  })

  it('dismisses with Escape', async () => {
    const { open } = mountSheet(true)
    await nextTick()
    expect(open.value).toBe(true)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(open.value).toBe(false)
  })

  it('moves focus inside on open', async () => {
    mountSheet(true)
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 50))
    const sheet = document.querySelector('[data-slot="sheet-content"]')
    expect(sheet?.contains(document.activeElement)).toBe(true)
  })

  it('spans the full height with the spec widths and a status slot', async () => {
    studioMatchMedia(false)
    for (const wide of [false, true]) {
      const open = ref(true)
      const el = document.createElement('div')
      document.body.appendChild(el)
      const app = createApp({
        render: () =>
          h(
            DetailSheet,
            {
              open: open.value,
              'onUpdate:open': (next: boolean) => {
                open.value = next
              },
              title: 'Ada',
              description: 'User',
              wide,
            },
            { default: () => 'body', status: () => (wide ? 'WIDE-ACTIVE' : 'ACTIVE') },
          ),
      })
      app.mount(el)
      cleanups.push(() => {
        app.unmount()
        el.remove()
      })
    }
    await nextTick()
    const sheets = [...document.querySelectorAll('[data-slot="sheet-content"]')]
    expect(sheets).toHaveLength(2)
    for (const sheet of sheets) {
      expect(sheet.getAttribute('data-side')).toBe('right')
      expect(sheet.getAttribute('class')).toContain('data-[side=right]:h-full')
      expect(sheet.getAttribute('class')).toContain('data-[side=right]:inset-y-0')
    }
    const classes = sheets.map((sheet) => sheet.getAttribute('class') ?? '')
    expect(classes.some((cls) => cls.includes('data-[side=right]:w-[480px]'))).toBe(true)
    expect(classes.some((cls) => cls.includes('data-[side=right]:w-[640px]'))).toBe(true)
    for (const cls of classes) {
      expect(cls).not.toContain('data-[side=right]:w-3/4')
      expect(cls).not.toContain('data-[side=right]:sm:max-w-sm')
    }
    expect(document.body.textContent).toContain('ACTIVE')
    expect(document.body.textContent).toContain('WIDE-ACTIVE')
  })
})

describe('ConfirmDialog dismiss-while-busy', () => {
  function mountConfirm(props: Record<string, unknown>) {
    const open = ref(true)
    const confirmed: unknown[][] = []
    const el = document.createElement('div')
    document.body.appendChild(el)
    const app = createApp({
      render: () =>
        h(ConfirmDialog, {
          ...props,
          open: open.value,
          'onUpdate:open': (next: boolean) => {
            open.value = next
          },
          title: 'Delete?',
          body: 'Gone.',
          onConfirm: (...args: unknown[]) => confirmed.push(args),
        }),
    })
    app.mount(el)
    cleanups.push(() => {
      app.unmount()
      el.remove()
    })
    return { el, open, confirmed }
  }

  it('stays open on Escape and outside clicks while busy', async () => {
    const { open } = mountConfirm({ busy: true })
    await nextTick()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await nextTick()
    expect(open.value).toBe(true)
    document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    await nextTick()
    expect(open.value).toBe(true)
  })

  it('dismisses with Escape when idle', async () => {
    const { open } = mountConfirm({})
    await nextTick()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(open.value).toBe(false)
  })
})
