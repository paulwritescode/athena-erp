// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import ActionButton from './components/ActionButton.vue'
import type { ActionKind } from './components/actions'

type ButtonProps = InstanceType<typeof ActionButton>['$props']
type Mounted = { el: HTMLElement; pressed: number; unmount: () => void }

const mounted: Mounted[] = []

afterEach(() => {
  for (const m of mounted.splice(0)) m.unmount()
  document.body.innerHTML = ''
})

function mountButton(props: Record<string, unknown>): Mounted {
  const state = { pressed: 0 }
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ render: () => h(ActionButton, { ...props, onPress: () => { state.pressed += 1 } } as ButtonProps) })
  app.mount(el)
  const tracked: Mounted = {
    el,
    get pressed() {
      return state.pressed
    },
    unmount: () => {
      app.unmount()
      el.remove()
    },
  }
  mounted.push(tracked)
  return tracked
}

function button(el: HTMLElement): HTMLButtonElement {
  const found = el.querySelector('button')
  if (!found) throw new Error('no button rendered')
  return found as HTMLButtonElement
}

describe('ActionButton', () => {
  it('emits press on click', async () => {
    const m = mountButton({ label: 'Save' })
    await nextTick()
    button(m.el).click()
    expect(m.pressed).toBe(1)
    expect(button(m.el).textContent).toContain('Save')
  })

  it('shows the idle, busy and disabled states', async () => {
    const idle = mountButton({ label: 'Save' })
    await nextTick()
    expect(button(idle.el).disabled).toBe(false)
    expect(idle.el.querySelector('.animate-spin')).toBeNull()

    const busy = mountButton({ label: 'Save', busy: true })
    await nextTick()
    expect(button(busy.el).disabled).toBe(true)
    expect(busy.el.querySelector('.animate-spin')).not.toBeNull()
    expect(button(busy.el).textContent).toContain('Save')
    button(busy.el).click()
    expect(busy.pressed).toBe(0)

    const off = mountButton({ label: 'Save', disabledReason: 'Close its jobs first' })
    await nextTick()
    expect(button(off.el).disabled).toBe(true)
    expect(off.el.querySelector('span')?.getAttribute('title')).toBe('Close its jobs first')
    expect(button(off.el).getAttribute('aria-label')).toBe('Save: Close its jobs first')
    button(off.el).click()
    expect(off.pressed).toBe(0)
  })

  it('renders the six kinds with their looks', async () => {
    const kinds = {
      primary: ['bg-primary'],
      secondary: ['border-border', 'bg-background'],
      quiet: ['text-muted-foreground'],
      caution: ['text-warning', 'border-warning/60'],
      destructive: ['text-destructive', 'border-destructive/60', 'hover:bg-destructive', 'hover:text-destructive-foreground'],
      restore: [],
    } as const
    for (const [kind, classes] of Object.entries(kinds)) {
      const m = mountButton({ label: 'Do', kind: kind as ActionKind, icon: 'check' })
      await nextTick()
      const cls = button(m.el).getAttribute('class') ?? ''
      for (const want of classes) expect(cls, `${kind} wants ${want}`).toContain(want)
    }
    const restore = mountButton({ label: 'Do', kind: 'restore', icon: 'play' })
    await nextTick()
    expect(restore.el.querySelector('svg')?.getAttribute('class')).toContain('text-success')
  })

  it('forwards extra classes to the button, not the wrapper', async () => {
    const m = mountButton({ label: 'Do', class: 'max-sm:w-full' })
    await nextTick()
    expect(button(m.el).getAttribute('class')).toContain('max-sm:w-full')
    expect(m.el.querySelector('span')?.getAttribute('class')).not.toContain('max-sm:w-full')
  })

  it('renders counter, shortcut and helper', async () => {
    const m = mountButton({ label: 'Inbox', counter: 12, shortcut: '⌘1', helper: 'New' })
    await nextTick()
    expect(button(m.el).textContent).toContain('12')
    expect(button(m.el).querySelector('kbd')?.textContent).toBe('⌘1')
    expect(button(m.el).textContent).toContain('New')
  })

  it('renders icon-only with an accessible name', async () => {
    const m = mountButton({ iconOnly: true, label: 'More actions', icon: 'ellipsis' })
    await nextTick()
    const btn = button(m.el)
    expect(btn.getAttribute('aria-label')).toBe('More actions')
    expect(m.el.querySelector('span')?.getAttribute('title')).toBe('More actions')
    expect(btn.textContent?.trim()).toBe('')
  })

  it('fires on its shortcut and releases it on unmount', async () => {
    const m = mountButton({ label: 'Jump', shortcut: 'cmd+9' })
    await nextTick()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: '9', metaKey: true, bubbles: true }))
    expect(m.pressed).toBe(1)
    m.unmount()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: '9', metaKey: true, bubbles: true }))
    expect(m.pressed).toBe(1)
  })

  it('ignores shortcuts while typing', async () => {
    const m = mountButton({ label: 'Jump', shortcut: 'cmd+8' })
    await nextTick()
    const input = document.createElement('input')
    document.body.appendChild(input)
    input.dispatchEvent(new KeyboardEvent('keydown', { key: '8', metaKey: true, bubbles: true }))
    expect(m.pressed).toBe(0)
    input.remove()
  })
})
