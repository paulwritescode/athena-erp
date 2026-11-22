import { describe, expect, it } from 'vitest'
import { filterRecordTypes, kindLabel, listSortValues, packLabel } from './recordTypeList'
import type { RecordType } from '@/types'

function type(overrides: Partial<RecordType> & { name: string }): RecordType {
  return { label: overrides.name, description: '', kind: 'master', fields: [], revision: 1, ...overrides }
}

const rows = [
  type({ name: 'customer', label: 'Customer', description: 'People who buy', record_count: 12 }),
  type({ name: 'invoice', label: 'Invoice', kind: 'document', description: 'Bills sent', record_count: 3 }),
  type({ name: 'secret', label: 'Secret', description: 'Restricted', record_count: 0, count_hidden: true }),
]

describe('record type list', () => {
  it('searches label, key and description', () => {
    expect(filterRecordTypes(rows, 'bill', [], []).map((row) => row.name)).toEqual(['invoice'])
    expect(filterRecordTypes(rows, 'CUSTOMER', [], []).map((row) => row.name)).toEqual(['customer'])
    expect(filterRecordTypes(rows, 'restricted', [], []).map((row) => row.name)).toEqual(['secret'])
    expect(filterRecordTypes(rows, 'nope', [], [])).toEqual([])
  })

  it('filters by kind and pack', () => {
    expect(filterRecordTypes(rows, '', ['document'], []).map((row) => row.name)).toEqual(['invoice'])
    expect(filterRecordTypes(rows, '', [], ['none']).map((row) => row.name)).toEqual(['customer', 'invoice', 'secret'])
    expect(filterRecordTypes(rows, '', ['master'], ['none']).map((row) => row.name)).toEqual(['customer', 'secret'])
    expect(filterRecordTypes(rows, '', ['master', 'document'], []).map((row) => row.name)).toEqual(['customer', 'invoice', 'secret'])
  })

  it('labels kinds and packs', () => {
    expect(kindLabel('document')).toBe('Document')
    expect(kindLabel('master')).toBe('Master')
    expect(packLabel(undefined)).toBe('None')
    expect(packLabel('')).toBe('None')
    expect(packLabel('shop')).toBe('shop')
  })

  it('sorts hidden counts below zero and stamps chronologically', () => {
    const values = (key: string) => rows.map((row) => listSortValues[key]!(row))
    expect(values('records')).toEqual([12, 3, -1])
    expect(values('fields')).toEqual([0, 0, 0])
    const timed = [
      type({ name: 'a', updated_at: '2026-11-21T10:00:00Z' }),
      type({ name: 'b', updated_at: '2026-11-20T10:00:00Z' }),
      type({ name: 'c' }),
    ]
    expect(timed.map((row) => listSortValues.updated!(row))).toEqual(['2026-11-21T10:00:00Z', '2026-11-20T10:00:00Z', ''])
    expect(values('name')).toEqual(['customer', 'invoice', 'secret'])
  })
})
