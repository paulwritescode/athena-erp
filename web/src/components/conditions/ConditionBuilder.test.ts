// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import type { BuilderField, ConditionTree } from '@/conditions'
import ConditionBuilder from './ConditionBuilder.vue'

const cleanups: Array<() => void> = []

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
})

const fields: BuilderField[] = [
  { name: 'doc.name', label: 'Name', type: 'Text', kind: 'string' },
  { name: 'doc.qty', label: 'Qty', type: 'Number', kind: 'decimal' },
]

function mountBuilder(initial: ConditionTree | null) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  let latest: ConditionTree | null = initial
  const app = createApp({
    render: () => h(ConditionBuilder as never, {
      modelValue: latest,
      fields,
      'onUpdate:modelValue': (t: ConditionTree) => { latest = t },
    }),
  })
  app.mount(host)
  cleanups.push(() => app.unmount())
  return { host, latest: () => latest as ConditionTree | null }
}

function buttonByText(host: HTMLElement, text: string): HTMLButtonElement {
  const found = [...host.querySelectorAll('button')].find((b) => (b.textContent ?? '').includes(text))
  if (!found) throw new Error(`button ${text} missing: ${host.innerHTML.slice(0, 300)}`)
  return found as HTMLButtonElement
}

function setSelect(select: HTMLSelectElement, value: string) {
  select.value = value
  select.dispatchEvent(new Event('change', { bubbles: true }))
}

function setInput(input: HTMLInputElement, value: string) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('ConditionBuilder', () => {
  it('shows the empty state without an error', async () => {
    const { host } = mountBuilder(null)
    await nextTick()
    expect(host.textContent).toContain('No conditions yet')
    expect(host.querySelector('[data-testid="builder-error"]')).toBeNull()
  })

  it('adds a condition and compiles it live', async () => {
    const { host, latest } = mountBuilder(null)
    await nextTick()
    buttonByText(host, 'Add condition').click()
    await nextTick()
    const row = host.querySelector('[data-testid="condition-row"]')
    expect(row).not.toBeNull()
    const selects = [...(row as HTMLElement).querySelectorAll('select')]
    setSelect(selects[0] as HTMLSelectElement, 'doc.name')
    await nextTick()
    setInput(row?.querySelector('input') as HTMLInputElement, 'Acme')
    await nextTick()
    expect(latest()?.root.conditions?.[0]).toMatchObject({ field: 'doc.name', op: 'is', values: ['Acme'] })
    buttonByText(host, 'Show as expression').click()
    await nextTick()
    expect(host.querySelector('[data-testid="builder-expression"]')?.textContent).toBe(`doc.name == 'Acme'`)
  })

  it('narrows operators by field type', async () => {
    const { host } = mountBuilder({ root: { all: true, conditions: [{ field: 'doc.qty', op: 'more_than', values: ['2'] }] } })
    await nextTick()
    const selects = [...host.querySelectorAll('[data-testid="condition-row"] select')]
    const options = [...(selects[1] as HTMLSelectElement).querySelectorAll('option')].map((o) => o.textContent)
    expect(options).toContain('more than')
    expect(options).not.toContain('contains')
  })

  it('nests groups', async () => {
    const { host, latest } = mountBuilder({ root: { all: true, conditions: [{ field: 'doc.name', op: 'is', values: ['a'] }] } })
    await nextTick()
    buttonByText(host, 'Add group').click()
    await nextTick()
    expect(host.querySelectorAll('[data-testid="condition-group"]')).toHaveLength(2)
    expect(latest()?.root.groups).toHaveLength(1)
  })

  it('shows compile problems inline', async () => {
    const { host } = mountBuilder({ root: { all: true, conditions: [{ field: 'doc.nope', op: 'is', values: ['a'] }] } })
    await nextTick()
    expect(host.querySelector('[data-testid="builder-error"]')?.textContent).toContain('unknown field')
  })

  it('compact mode hides nested groups', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const app = createApp({ render: () => h(ConditionBuilder as never, { modelValue: null, fields, compact: true }) })
    app.mount(host)
    cleanups.push(() => app.unmount())
    await nextTick()
    expect(host.textContent).toContain('Add condition')
    expect(host.textContent).not.toContain('Add group')
  })
})
