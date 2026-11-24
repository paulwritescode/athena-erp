// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, nextTick, type Component } from 'vue'
import DataTable from './DataTable.vue'

Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: () => ({
    matches: false,
    media: '',
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
    onchange: null,
  }),
})

const mounted: { unmount: () => void; el: HTMLElement }[] = []

afterEach(() => {
  for (const m of mounted.splice(0)) {
    m.unmount()
    m.el.remove()
  }
  document.body.innerHTML = ''
})

async function mountTable(props: Record<string, unknown>): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp(DataTable as Component, {
    columns: [{ accessorKey: 'name', header: 'Name' }],
    data: [],
    ...props,
  })
  app.mount(el)
  mounted.push({ unmount: () => app.unmount(), el })
  await nextTick()
  return el
}

describe('table search wiring', () => {
  it('derives the placeholder from the scope', async () => {
    const el = await mountTable({ searchScope: 'Diet concern' })
    expect(el.querySelector('input')?.getAttribute('placeholder')).toBe('Find diet concerns…')
  })

  it('prefers an explicit placeholder over the scope', async () => {
    const el = await mountTable({ searchScope: 'Diet concern', searchPlaceholder: 'Find things…' })
    expect(el.querySelector('input')?.getAttribute('placeholder')).toBe('Find things…')
  })

  it('shows no search box without a placeholder or scope', async () => {
    const el = await mountTable({})
    expect(el.querySelector('input')).toBeNull()
  })
})
