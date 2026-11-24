// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick, reactive, ref, type Component } from 'vue'
import FieldSettingsDialog from './FieldSettingsDialog.vue'
import { trackNewField } from './fieldSettings'
import type { Field, RecordType } from '../types'

// jsdom lacks the pointer-capture and scrolling APIs Reka drives; the real
// browser path is covered by the browser pass.
if (!Element.prototype.hasPointerCapture) Element.prototype.hasPointerCapture = () => false
if (!Element.prototype.releasePointerCapture) Element.prototype.releasePointerCapture = () => {}
if (!Element.prototype.setPointerCapture) Element.prototype.setPointerCapture = () => {}
if (!Element.prototype.scrollIntoView) Element.prototype.scrollIntoView = () => {}

function stubMatchMedia(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: () => ({
      matches,
      media: '',
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
      onchange: null,
    }),
  })
}

const mounted: { unmount: () => void; el: HTMLElement }[] = []

afterEach(() => {
  for (const m of mounted.splice(0)) {
    m.unmount()
    m.el.remove()
  }
  document.body.innerHTML = ''
})

function testField(overrides: Partial<Field> = {}): Field {
  return { name: 'amount', label: 'Amount', type: 'Number', required: false, ...overrides }
}

function testTypes(): RecordType[] {
  return [
    { name: 'invoice', label: 'Invoice', description: '', kind: 'master', fields: [], revision: 1, record_count: 0 },
    { name: 'line', label: 'Line', description: '', kind: 'child', parent_record_type: 'invoice', fields: [
      testField({ name: 'qty', label: 'Qty', type: 'Number' }),
      testField({ name: 'rate', label: 'Rate', type: 'Currency' }),
      testField({ name: 'note', label: 'Note', type: 'Text' }),
    ], revision: 1, record_count: 0 },
    { name: 'customer', label: 'Customer', description: '', kind: 'master', fields: [], revision: 1, record_count: 0 },
  ]
}

// The sheet portals to the body, so every query below reads the document.
async function mountSheet(field: Field, overrides: { lifecycle?: boolean; recordTypes?: RecordType[]; currentTypeName?: string } = {}) {
  const closed: string[] = []
  // Fields nest in reactive editor state in production; the proxy keeps the
  // in-place edits observable here too.
  const state = reactive(field)
  const current = ref(state)
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({
    setup: () => () => h(FieldSettingsDialog as Component, {
      field: current.value,
      lifecycle: overrides.lifecycle ?? false,
      recordTypes: overrides.recordTypes ?? testTypes(),
      currentTypeName: overrides.currentTypeName ?? 'invoice',
      onClose: () => closed.push('close'),
    }),
  })
  app.mount(el)
  const entry = { unmount: () => app.unmount(), el }
  mounted.push(entry)
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 60))
  await nextTick()
  return {
    closed,
    state,
    setField: (next: Field) => {
      current.value = reactive(next)
      return current.value
    },
    unmount: () => {
      entry.unmount()
      el.remove()
      mounted.splice(mounted.indexOf(entry), 1)
    },
  }
}

async function flush() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

function sectionTrigger(title: string): HTMLElement {
  const found = [...document.querySelectorAll('button')].find((button) => button.textContent === title)
  if (!found) throw new Error(`no section ${title}`)
  return found as HTMLElement
}

function click(target: Element | null) {
  target?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
}

async function choose(id: string, optionText: string) {
  // Reka opens on pointerdown and selects on pointerup; clicks alone do neither.
  document.getElementById(id)?.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 50))
  const option = [...document.querySelectorAll('[role="option"]')].find((item) => item.textContent === optionText)
  if (!option) throw new Error(`no option ${optionText}`)
  option.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, button: 0 }))
  await flush()
}

async function setInput(input: HTMLInputElement, value: string) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await flush()
}

function conditionBlock(name: string): HTMLElement {
  const found = document.querySelector(`[data-testid="condition-${name}"]`)
  if (!found) throw new Error(`no condition block ${name}`)
  return found as HTMLElement
}

async function switchEditor(name: string, editor: 'Builder' | 'Written') {
  const group = conditionBlock(name).querySelector('[role="group"]')
  const button = [...(group?.querySelectorAll('button') ?? [])].find((item) => item.textContent === editor)
  if (!button) throw new Error(`no ${editor} toggle for ${name}`)
  click(button)
  await flush()
}

function setSelect(select: HTMLSelectElement, value: string) {
  select.value = value
  select.dispatchEvent(new Event('change', { bubbles: true }))
}

describe('field settings sheet', () => {
  it('opens Basics with Rules and Advanced collapsed', async () => {
    stubMatchMedia(false)
    await mountSheet(testField())
    expect(document.querySelector('#fs-label')).not.toBeNull()
    expect(document.querySelector('#fs-formula')).toBeNull()
    expect(document.querySelector('#fs-min')).toBeNull()
    expect(sectionTrigger('Basics').getAttribute('aria-expanded')).toBe('true')
    expect(sectionTrigger('Rules for this field').getAttribute('aria-expanded')).toBe('false')
    click(sectionTrigger('Rules for this field'))
    await flush()
    expect(document.querySelector('#fs-formula')).not.toBeNull()
  })

  it('edits the label and derives the key of a new field', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField({ name: 'field_1', label: 'New field' }))
    trackNewField(state)
    await setInput(document.querySelector('#fs-label') as HTMLInputElement, 'Customer name')
    expect(state.label).toBe('Customer name')
    expect(state.name).toBe('customer_name')
  })

  it('changes the type through the shared rules', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField({ default: 'x' }))
    await choose('fs-type', 'Money')
    expect(state.type).toBe('Money')
    expect(state.currency).toBe('USD')
    expect(state.default).toBeUndefined()
  })

  it('toggles required', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField())
    const label = [...document.querySelectorAll('.settings-flags label')].find((item) => item.textContent?.startsWith('Required')) as HTMLElement
    label.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flush()
    expect(state.required).toBe(true)
  })

  it('picks the link target from the other types', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField({ type: 'Link' }))
    await choose('fs-target', 'Customer')
    expect(state.target_record_type).toBe('customer')
  })

  it('picks the permission level', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField())
    click(sectionTrigger('Advanced'))
    await flush()
    await choose('fs-level', 'Level 3')
    expect(state.permission_level).toBe(3)
    await choose('fs-level', 'Level 0')
    expect(state.permission_level).toBeUndefined()
  })

  it('composes a lines total into the formula', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField())
    click(sectionTrigger('Rules for this field'))
    await flush()
    expect(document.body.textContent).toContain('Totals from lines')
    expect(document.body.textContent).toContain('sum(doc.line.qty)')
    const use = [...document.querySelectorAll('button')].find((button) => (button.textContent ?? '').trim() === 'Use this total')
    if (!use) throw new Error('no Use this total button')
    click(use)
    await flush()
    expect(state.formula).toBe('sum(doc.line.qty)')
    expect((document.querySelector('#fs-formula') as HTMLInputElement).value).toBe('sum(doc.line.qty)')
  })

  it('hides totals without child lines', async () => {
    stubMatchMedia(false)
    const field = testField()
    await mountSheet(field, { recordTypes: [testTypes()[0]!], currentTypeName: 'invoice' })
    click(sectionTrigger('Rules for this field'))
    await flush()
    expect(document.body.textContent).not.toContain('Totals from lines')
  })

  it('flags an overlong expression inline', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField())
    click(sectionTrigger('Rules for this field'))
    await flush()
    await switchEditor('validate', 'Written')
    await setInput(document.querySelector('#fs-validate') as HTMLInputElement, `doc.ok && '${'x'.repeat(1000)}'`)
    expect(state.validate).toContain('doc.ok')
    expect(document.querySelector('.field-error')?.textContent).toContain('at most 1000 characters')
  })

  it('builds a condition into the stored expression', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField({ name: 'qty', label: 'Qty', type: 'Number' }), { currentTypeName: 'line' })
    click(sectionTrigger('Rules for this field'))
    await flush()
    const block = conditionBlock('required_if')
    expect(block.querySelector('[data-testid="condition-builder"]')).not.toBeNull()
    expect(block.querySelector('#fs-required-if')).toBeNull()
    const add = [...block.querySelectorAll('button')].find((button) => (button.textContent ?? '').includes('Add condition'))
    if (!add) throw new Error('no Add condition button')
    click(add)
    await flush()
    // Unfinished (no value yet): nothing stored.
    expect(state.required_if).toBeUndefined()
    const row = block.querySelector('[data-testid="condition-row"]') as HTMLElement
    const selects = [...row.querySelectorAll('select')]
    setSelect(selects[1] as HTMLSelectElement, 'more_than')
    await flush()
    await setInput(row.querySelector('input') as HTMLInputElement, '2')
    expect(state.required_if).toBe(`doc.qty > dec('2')`)
    const show = [...block.querySelectorAll('button')].find((button) => (button.textContent ?? '').includes('Show as expression'))
    if (!show) throw new Error('no Show as expression button')
    click(show)
    await flush()
    expect(block.querySelector('[data-testid="builder-expression"]')?.textContent).toBe(`doc.qty > dec('2')`)
  })

  it('round-trips between the builder and the written input', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField({ name: 'qty', label: 'Qty', type: 'Number' }), { currentTypeName: 'line' })
    click(sectionTrigger('Rules for this field'))
    await flush()
    const block = conditionBlock('required_if')
    const add = [...block.querySelectorAll('button')].find((button) => (button.textContent ?? '').includes('Add condition'))
    if (!add) throw new Error('no Add condition button')
    click(add)
    await flush()
    const row = block.querySelector('[data-testid="condition-row"]') as HTMLElement
    await setInput(row.querySelector('input') as HTMLInputElement, '2')
    expect(state.required_if).toBe(`doc.qty == dec('2')`)
    // Builder to written: the input shows the built expression.
    await switchEditor('required_if', 'Written')
    expect((document.querySelector('#fs-required-if') as HTMLInputElement).value).toBe(`doc.qty == dec('2')`)
    expect(state.required_if).toBe(`doc.qty == dec('2')`)
    // Written text survives a builder peek untouched — the peek itself
    // must not delete the stored expression.
    await setInput(document.querySelector('#fs-required-if') as HTMLInputElement, 'doc.paid')
    expect(state.required_if).toBe('doc.paid')
    await switchEditor('required_if', 'Builder')
    expect(state.required_if).toBe('doc.paid')
    expect(conditionBlock('required_if').textContent).toContain('A written version is stored')
    await switchEditor('required_if', 'Written')
    expect((document.querySelector('#fs-required-if') as HTMLInputElement).value).toBe('doc.paid')
    expect(state.required_if).toBe('doc.paid')
  })

  it('keeps the stored expression when the sheet closes mid-peek', async () => {
    stubMatchMedia(false)
    const { state, closed } = await mountSheet(testField({ name: 'qty', label: 'Qty', type: 'Number', required_if: 'doc.paid' }), { currentTypeName: 'line' })
    click(sectionTrigger('Rules for this field'))
    await flush()
    await switchEditor('required_if', 'Builder')
    expect(state.required_if).toBe('doc.paid')
    const done = [...document.querySelectorAll('button')].find((button) => button.textContent === 'Done') as HTMLElement
    click(done)
    await flush()
    expect(closed).toEqual(['close'])
    expect(state.required_if).toBe('doc.paid')
  })

  it('opens stored expressions as written and clears through the builder', async () => {
    stubMatchMedia(false)
    const { state } = await mountSheet(testField({ name: 'qty', label: 'Qty', type: 'Number', required_if: 'doc.paid' }), { currentTypeName: 'line' })
    click(sectionTrigger('Rules for this field'))
    await flush()
    expect((document.querySelector('#fs-required-if') as HTMLInputElement).value).toBe('doc.paid')
    expect(conditionBlock('required_if').querySelector('[data-testid="condition-builder"]')).toBeNull()
    expect(conditionBlock('validate').querySelector('[data-testid="condition-builder"]')).not.toBeNull()
    await switchEditor('required_if', 'Builder')
    await flush()
    expect(conditionBlock('required_if').querySelector('[data-testid="condition-builder"]')).not.toBeNull()
    expect(state.required_if).toBe('doc.paid')
    await switchEditor('required_if', 'Written')
    expect(state.required_if).toBe('doc.paid')
    // Build one condition, then remove it: the attribute clears.
    await switchEditor('required_if', 'Builder')
    const block = conditionBlock('required_if')
    const add = [...block.querySelectorAll('button')].find((button) => (button.textContent ?? '').includes('Add condition'))
    if (!add) throw new Error('no Add condition button')
    click(add)
    await flush()
    const row = block.querySelector('[data-testid="condition-row"]') as HTMLElement
    await setInput(row.querySelector('input') as HTMLInputElement, '2')
    expect(state.required_if).toBe(`doc.qty == dec('2')`)
    const remove = [...row.querySelectorAll('button')].find((button) => button.getAttribute('aria-label') === 'Remove condition')
    if (!remove) throw new Error('no Remove condition button')
    click(remove)
    await flush()
    expect(state.required_if).toBeUndefined()
  })

  it('keeps labels and help free of expression syntax', async () => {
    stubMatchMedia(false)
    const field = testField()
    await mountSheet(field)
    for (const title of ['Rules for this field', 'Advanced']) {
      click(sectionTrigger(title))
      await flush()
    }
    const words = [...document.querySelectorAll('label, .settings-note:not(.font-mono), h2')].map((node) => node.textContent ?? '')
    expect(words.length).toBeGreaterThan(10)
    for (const text of words) expect(text).not.toContain('doc.')
  })

  it('renders a right sheet on desktop and a drawer on phones', async () => {
    stubMatchMedia(false)
    const desktop = await mountSheet(testField())
    const sheet = document.querySelector('[data-slot="sheet-content"]')
    expect(sheet).not.toBeNull()
    expect(sheet?.className).toContain('sm:max-w-[480px]')
    const body = document.querySelector('.overlay-stack')
    expect(body?.className).toContain('overflow-y-auto')
    desktop.unmount()
    document.body.innerHTML = ''

    stubMatchMedia(true)
    await mountSheet(testField())
    expect(document.querySelector('[data-slot="sheet-content"]')).toBeNull()
    expect(document.querySelector('[data-vaul-drawer]')).not.toBeNull()
    expect(document.querySelector('#fs-label')).not.toBeNull()
  })

  it('resets collapsed sections for the next field', async () => {
    stubMatchMedia(false)
    const { setField } = await mountSheet(testField())
    click(sectionTrigger('Rules for this field'))
    await flush()
    expect(document.querySelector('#fs-formula')).not.toBeNull()
    setField(testField({ name: 'rate', label: 'Rate', type: 'Currency' }))
    await flush()
    expect(document.querySelector('#fs-label')).not.toBeNull()
    expect(document.querySelector('#fs-formula')).toBeNull()
    expect(sectionTrigger('Rules for this field').getAttribute('aria-expanded')).toBe('false')
  })

  it('closes through Done', async () => {
    stubMatchMedia(false)
    const { closed } = await mountSheet(testField())
    const done = [...document.querySelectorAll('button')].find((button) => button.textContent === 'Done') as HTMLElement
    click(done)
    await flush()
    expect(closed).toEqual(['close'])
  })
})
