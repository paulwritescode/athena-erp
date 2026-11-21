import { describe, expect, it } from 'vitest'
import {
  DEFAULT_PAGE_SIZE,
  loadHiddenColumns,
  loadPageSize,
  saveHiddenColumns,
  savePageSize,
  tableColumnsKey,
  tablePageSizeKey,
} from './tablePrefs'

function memory(): Storage {
  const map = new Map<string, string>()
  return {
    getItem: (key: string) => (map.has(key) ? map.get(key)! : null),
    setItem: (key: string, value: string) => { map.set(key, value) },
    removeItem: (key: string) => { map.delete(key) },
    clear: () => map.clear(),
    key: (index: number) => [...map.keys()][index] ?? null,
    get length() { return map.size },
  }
}

describe('table preference keys', () => {
  it('namespaces columns and page size by user and table', () => {
    expect(tableColumnsKey('records:customer', 'ada')).toBe('athena-table-columns:ada:records:customer')
    expect(tablePageSizeKey('records:customer', 'ada')).toBe('athena-table-page-size:ada:records:customer')
  })

  it('falls back to a shared namespace without a username', () => {
    expect(tableColumnsKey('records:customer', '')).toBe('athena-table-columns:shared:records:customer')
    expect(tablePageSizeKey('users', '')).toBe('athena-table-page-size:shared:users')
  })
})

describe('hidden columns', () => {
  it('round-trips the hidden ids', () => {
    const storage = memory()
    saveHiddenColumns('users', 'ada', new Set(['email', 'roles']), storage)
    expect(loadHiddenColumns('users', 'ada', storage)).toEqual(new Set(['email', 'roles']))
  })

  it('keeps users apart', () => {
    const storage = memory()
    saveHiddenColumns('users', 'ada', ['email'], storage)
    expect(loadHiddenColumns('users', 'grace', storage)).toEqual(new Set())
  })

  it('clears the key when nothing is hidden', () => {
    const storage = memory()
    saveHiddenColumns('users', 'ada', ['email'], storage)
    saveHiddenColumns('users', 'ada', [], storage)
    expect(storage.getItem(tableColumnsKey('users', 'ada'))).toBeNull()
    expect(loadHiddenColumns('users', 'ada', storage)).toEqual(new Set())
  })

  it('ignores corrupt values', () => {
    const storage = memory()
    storage.setItem(tableColumnsKey('users', 'ada'), '{nope')
    expect(loadHiddenColumns('users', 'ada', storage)).toEqual(new Set())
    storage.setItem(tableColumnsKey('users', 'ada'), JSON.stringify({ email: true }))
    expect(loadHiddenColumns('users', 'ada', storage)).toEqual(new Set())
  })
})

describe('page size', () => {
  it('round-trips a valid size', () => {
    const storage = memory()
    savePageSize('users', 'ada', 50, storage)
    expect(loadPageSize('users', 'ada', storage)).toBe(50)
  })

  it('falls back for missing and invalid sizes', () => {
    const storage = memory()
    expect(loadPageSize('users', 'ada', storage)).toBe(DEFAULT_PAGE_SIZE)
    storage.setItem(tablePageSizeKey('users', 'ada'), '7')
    expect(loadPageSize('users', 'ada', storage)).toBe(DEFAULT_PAGE_SIZE)
    storage.setItem(tablePageSizeKey('users', 'ada'), 'nope')
    expect(loadPageSize('users', 'ada', storage)).toBe(DEFAULT_PAGE_SIZE)
  })
})
