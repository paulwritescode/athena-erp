// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import DataTableSearch from './DataTableSearch.vue'
import { SEARCH_DEBOUNCE_MS, pluralScope, searchPlaceholderFor } from './dataTableSearch'

describe('search scope plural', () => {
  it('lowercases and pluralizes a type label', () => {
    expect(pluralScope('Diet concern')).toBe('diet concerns')
    expect(pluralScope('Note')).toBe('notes')
    expect(pluralScope('  Customer ')).toBe('customers')
  })

  it('pluralizes -y, sibilants and -ch endings', () => {
    expect(pluralScope('Company')).toBe('companies')
    expect(pluralScope('Branch')).toBe('branches')
    expect(pluralScope('Box')).toBe('boxes')
    expect(pluralScope('Batch')).toBe('batches')
    expect(pluralScope('Day')).toBe('days')
  })

  it('leaves an empty scope empty', () => {
    expect(pluralScope('')).toBe('')
    expect(pluralScope('   ')).toBe('')
  })
})

describe('search placeholder', () => {
  it('derives Find words from the type label', () => {
    expect(searchPlaceholderFor('Diet concern')).toBe('Find diet concerns…')
    expect(searchPlaceholderFor('Note')).toBe('Find notes…')
  })
})

const mounted: { unmount: () => void; el: HTMLElement }[] = []

afterEach(() => {
  for (const m of mounted.splice(0)) {
    m.unmount()
    m.el.remove()
  }
  document.body.innerHTML = ''
  vi.useRealTimers()
})

function mountSearch(initial = '') {
  vi.useFakeTimers()
  const emitted: string[] = []
  const value = ref(initial)
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({
    setup: () => () => h(DataTableSearch, {
      modelValue: value.value,
      placeholder: 'Find notes…',
      'onUpdate:modelValue': (next: string) => { emitted.push(next); value.value = next },
    }),
  })
  app.mount(el)
  const entry = { unmount: () => app.unmount(), el }
  mounted.push(entry)
  return {
    emitted,
    value,
    input: () => el.querySelector('input') as HTMLInputElement,
    clearButton: () => el.querySelector('button[aria-label="Clear search"]') as HTMLButtonElement | null,
    unmount: () => {
      entry.unmount()
      el.remove()
      mounted.splice(mounted.indexOf(entry), 1)
    },
  }
}

async function type(input: HTMLInputElement, text: string) {
  input.value = text
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

describe('table search box', () => {
  it('shows the placeholder on the input', () => {
    const search = mountSearch()
    expect(search.input().getAttribute('placeholder')).toBe('Find notes…')
    expect(search.input().getAttribute('aria-label')).toBe('Find notes…')
  })

  it('emits the first character after the pause', async () => {
    const search = mountSearch()
    await type(search.input(), 'z')
    // The box shows the keystroke at once; the table hears it after the pause.
    expect(search.input().value).toBe('z')
    expect(search.emitted).toEqual([])
    vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS - 1)
    expect(search.emitted).toEqual([])
    vi.advanceTimersByTime(1)
    expect(search.emitted).toEqual(['z'])
  })

  it('emits one burst of typing once', async () => {
    const search = mountSearch()
    await type(search.input(), 'a')
    vi.advanceTimersByTime(100)
    await type(search.input(), 'ab')
    vi.advanceTimersByTime(100)
    expect(search.emitted).toEqual([])
    vi.advanceTimersByTime(50)
    expect(search.emitted).toEqual(['ab'])
  })

  it('clears at once', async () => {
    const search = mountSearch()
    await type(search.input(), 'a')
    vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS)
    expect(search.emitted).toEqual(['a'])
    search.clearButton()?.click()
    await nextTick()
    expect(search.emitted).toEqual(['a', ''])
    expect(search.input().value).toBe('')
  })

  it('cancels a pending burst when cleared', async () => {
    const search = mountSearch()
    await type(search.input(), 'a')
    search.clearButton()?.click()
    await nextTick()
    vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS * 2)
    expect(search.emitted).toEqual([''])
  })

  it('lets the caller value win over a pending burst', async () => {
    const search = mountSearch()
    await type(search.input(), 'a')
    search.value.value = 'x'
    await nextTick()
    expect(search.input().value).toBe('x')
    vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS * 2)
    expect(search.emitted).toEqual([])
  })

  it('cancels a pending burst on unmount', async () => {
    const search = mountSearch()
    await type(search.input(), 'a')
    search.unmount()
    vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS * 2)
    expect(search.emitted).toEqual([])
  })
})
