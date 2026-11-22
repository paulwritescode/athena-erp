// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick, type Component } from 'vue'
import CheckboxField from './components/fields/CheckboxField.vue'
import DateField from './components/fields/DateField.vue'
import LinkField from './components/fields/LinkField.vue'
import LongTextField from './components/fields/LongTextField.vue'
import MultiSelectField from './components/fields/MultiSelectField.vue'
import NumberField from './components/fields/NumberField.vue'
import SelectField from './components/fields/SelectField.vue'
import TextField from './components/fields/TextField.vue'

// jsdom has no scrolling: the combobox scrolls its highlighted option into
// view, so stub it to keep the suite's exit code clean.
Object.defineProperty(Element.prototype, 'scrollIntoView', { configurable: true, value: () => {} })

const cleanups: Array<() => void> = []

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
})

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

type Mounted = { el: HTMLElement; emitted: unknown[][] }

function mountField(component: Component, props: Record<string, unknown>): Mounted {
  const emitted: unknown[][] = []
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({
    render: () =>
      h(component, {
        ...props,
        'onUpdate:modelValue': (...args: unknown[]) => emitted.push(args),
      }),
  })
  app.mount(el)
  cleanups.push(() => {
    app.unmount()
    el.remove()
  })
  return { el, emitted }
}

describe('field chrome', () => {
  it('labels, marks optional, and swaps help for error', async () => {
    const plain = mountField(TextField, { id: 'f-name', label: 'Name', help: 'Shown everywhere.' })
    await nextTick()
    expect(plain.el.querySelector('label')?.textContent).toBe('Name')
    expect(plain.el.querySelector('#f-name-help')?.textContent).toBe('Shown everywhere.')
    expect(plain.el.querySelector('input')?.getAttribute('aria-describedby')).toBe('f-name-help')

    const optional = mountField(TextField, { id: 'f-nick', label: 'Nickname', optional: true })
    await nextTick()
    expect(optional.el.querySelector('label')?.textContent).toBe('Nickname (optional)')

    const failed = mountField(TextField, { id: 'f-mail', label: 'Email', help: 'Shown everywhere.', error: 'Enter an email address.' })
    await nextTick()
    expect(failed.el.querySelector('#f-mail-help')).toBeNull()
    expect(failed.el.querySelector('#f-mail-error')?.textContent).toContain('Enter an email address.')
    expect(failed.el.querySelector('input')?.getAttribute('aria-describedby')).toBe('f-mail-error')
    expect(failed.el.querySelector('input')?.getAttribute('aria-invalid')).toBe('true')
  })

  it('emits input and honors disabled', async () => {
    const field = mountField(TextField, { id: 'f-qty', label: 'Quantity' })
    await nextTick()
    const input = field.el.querySelector('input')
    input!.value = '12'
    input!.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(field.emitted).toEqual([['12']])

    const off = mountField(TextField, { id: 'f-lock', label: 'Locked', disabled: true })
    await nextTick()
    expect(off.el.querySelector('input')?.disabled).toBe(true)
  })

  it('emits enter on Enter', async () => {
    let entered = 0
    const el = document.createElement('div')
    document.body.appendChild(el)
    const app = createApp({ render: () => h(TextField, { id: 'f-go', label: 'Go', onEnter: () => { entered += 1 } }) })
    app.mount(el)
    cleanups.push(() => {
      app.unmount()
      el.remove()
    })
    await nextTick()
    el.querySelector('input')!.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true }))
    await nextTick()
    expect(entered).toBe(1)
  })

  it('renders long text and numbers', async () => {
    const long = mountField(LongTextField, { id: 'f-body', label: 'Body', modelValue: 'hi' })
    await nextTick()
    expect(long.el.querySelector('textarea')?.getAttribute('rows')).toBe('4')

    const num = mountField(NumberField, { id: 'f-total', label: 'Total' })
    await nextTick()
    expect(num.el.querySelector('input')?.getAttribute('inputmode')).toBe('decimal')
  })
})

describe('DateField', () => {
  it('opens a calendar popover on desktop', async () => {
    stubMatchMedia(true)
    const field = mountField(DateField, { id: 'f-when', label: 'When', modelValue: '2026-11-21' })
    await nextTick()
    expect(field.el.querySelector('button')?.textContent).toContain('21')
  })

  it('falls back to a native date input below 1024 px', async () => {
    stubMatchMedia(false)
    const field = mountField(DateField, { id: 'f-when', label: 'When' })
    await nextTick()
    expect(field.el.querySelector('input')?.getAttribute('type')).toBe('date')
  })
})

describe('SelectField', () => {
  const options = [
    { value: 'draft', label: 'Draft' },
    { value: 'sent', label: 'Sent' },
  ]

  it('opens a combobox on desktop', async () => {
    stubMatchMedia(true)
    const field = mountField(SelectField, { id: 'f-state', label: 'State', options, modelValue: 'sent' })
    await nextTick()
    expect(field.el.querySelector('button')?.getAttribute('role')).toBe('combobox')
    expect(field.el.querySelector('button')?.textContent).toContain('Sent')
  })

  it('opens the option list without crashing', async () => {
    stubMatchMedia(true)
    const field = mountField(SelectField, { id: 'f-state', label: 'State', options })
    await nextTick()
    field.el.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(document.querySelectorAll('[role="option"]')).toHaveLength(2)
  })

  it('falls back to a native select below 1024 px', async () => {
    stubMatchMedia(false)
    const field = mountField(SelectField, { id: 'f-state', label: 'State', options })
    await nextTick()
    expect(field.el.querySelector('select')).not.toBeNull()
  })

  it('offers None on an optional desktop combobox and clears through it', async () => {
    stubMatchMedia(true)
    const field = mountField(SelectField, { id: 'f-state', label: 'State', optional: true, options, modelValue: 'sent' })
    await nextTick()
    field.el.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 50))
    const items = [...document.querySelectorAll('[role="option"]')]
    expect(items).toHaveLength(3)
    items[0].dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(field.emitted).toContainEqual([''])
  })

  it('offers no None on a required desktop combobox', async () => {
    stubMatchMedia(true)
    const field = mountField(SelectField, { id: 'f-state', label: 'State', options, modelValue: 'sent' })
    await nextTick()
    field.el.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 50))
    const labels = [...document.querySelectorAll('[role="option"]')].map((item) => item.textContent)
    expect(labels).not.toContain('None')
  })

  it('keeps the empty placeholder option on a native optional select', async () => {
    stubMatchMedia(false)
    const field = mountField(SelectField, { id: 'f-state', label: 'State', optional: true, options, modelValue: 'sent' })
    await nextTick()
    const select = field.el.querySelector('select')
    expect(select?.querySelector('option')?.getAttribute('value')).toBe('')
  })
})

describe('MultiSelectField', () => {
  const options = [
    { value: 'clerk', label: 'Clerk' },
    { value: 'manager', label: 'Manager' },
  ]

  it('chips the selection and removes on x', async () => {
    stubMatchMedia(true)
    const field = mountField(MultiSelectField, { id: 'f-roles', label: 'Roles', optional: true, options, modelValue: ['clerk'] })
    await nextTick()
    expect(field.el.textContent).toContain('Clerk')
    expect(field.el.querySelector('button')?.getAttribute('aria-label')).toBe('Remove Clerk')
    field.el.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(field.emitted).toEqual([[[]]])
  })

  it('adds one through the native select below 1024 px', async () => {
    stubMatchMedia(false)
    const field = mountField(MultiSelectField, { id: 'f-roles', label: 'Roles', optional: true, options, modelValue: ['clerk'] })
    await nextTick()
    const select = field.el.querySelector('select')
    expect(select).not.toBeNull()
    expect(select?.querySelectorAll('option')).toHaveLength(2)
    select!.value = 'manager'
    select!.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()
    expect(field.emitted).toEqual([[['clerk', 'manager']]])
  })
})

describe('LinkField', () => {
  it('wraps the record picker with the shared chrome', async () => {
    const field = mountField(LinkField, { id: 'f-owner', label: 'Owner', recordTypeKey: 'task', targetLabel: 'Task' })
    await nextTick()
    expect(field.el.querySelector('label')?.textContent).toBe('Owner')
    expect(field.el.querySelector('[role="combobox"]')).not.toBeNull()
  })
})

describe('CheckboxField', () => {
  it('toggles and names the answer', async () => {
    const field = mountField(CheckboxField, { id: 'f-done', label: 'Done', modelValue: false })
    await nextTick()
    expect(field.el.textContent).toContain('No')
    field.el.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(field.emitted).toEqual([[true]])
  })
})
