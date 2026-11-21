import { describe, expect, it } from 'vitest'
import PrimaryCell from '@/components/PrimaryCell.vue'
import { combineRecordFilter, recordFieldHeader, recordFieldText, buildRecordColumns } from './recordColumns'
import type { Record as RecordMessage } from '@/gen/athena/v1/records_pb'
import type { Field } from '@/types'

function field(partial: Partial<Field> & { name: string }): Field {
  return { label: partial.name, type: 'Text', required: false, ...partial }
}

function record(values: Record<string, string | boolean>): RecordMessage {
  return {
    values: Object.fromEntries(Object.entries(values).map(([key, value]) => [
      key,
      typeof value === 'boolean'
        ? { kind: { case: 'boolValue', value } }
        : { kind: { case: 'stringValue', value } },
    ])),
  } as unknown as RecordMessage
}

describe('recordFieldText', () => {
  it('reads text and yes/no', () => {
    expect(recordFieldText(field({ name: 'title' }), record({ title: 'Linen' }), {})).toBe('Linen')
    expect(recordFieldText(field({ name: 'done', type: 'Checkbox' }), record({ done: true }), {})).toBe('Yes')
    expect(recordFieldText(field({ name: 'done', type: 'Checkbox' }), record({ done: false }), {})).toBe('No')
  })

  it('labels enum values and hides unknown links', () => {
    const color = field({ name: 'color', type: 'Enum', values: [{ value: 'r', label: 'Red' }] })
    expect(recordFieldText(color, record({ color: 'r' }), {})).toBe('Red')
    expect(recordFieldText(color, record({ color: 'zz' }), {})).toBe('zz')
    const owner = field({ name: 'owner', type: 'Link', target_record_type: 'customer' })
    expect(recordFieldText(owner, record({ owner: 'c1' }), { c1: 'Acme' })).toBe('Acme')
    expect(recordFieldText(owner, record({ owner: 'c9' }), {})).toBe('')
  })

  it('stays empty without a value', () => {
    expect(recordFieldText(field({ name: 'title' }), record({}), {})).toBe('')
  })
})

describe('recordFieldHeader', () => {
  it('carries the currency and unit', () => {
    expect(recordFieldHeader(field({ name: 'price', label: 'Price', type: 'Money', currency: 'USD' }))).toBe('Price (USD)')
    expect(recordFieldHeader(field({ name: 'qty', label: 'Qty', type: 'Quantity', uom: 'kg' }))).toBe('Qty (kg)')
    expect(recordFieldHeader(field({ name: 'title', label: 'Title' }))).toBe('Title')
  })
})

describe('combineRecordFilter', () => {
  it('passes each side through alone', () => {
    expect(combineRecordFilter('', [])).toBe('')
    expect(combineRecordFilter('doc.total > 1', [])).toBe('(doc.total > 1)')
    expect(combineRecordFilter('', [1])).toBe('(doc.docstate == 1)')
  })

  it('ands the search with the states', () => {
    expect(combineRecordFilter('doc.total > 1', [1])).toBe('(doc.total > 1) && (doc.docstate == 1)')
    expect(combineRecordFilter('', [0, 2])).toBe('(doc.docstate == 0 || doc.docstate == 2)')
    expect(combineRecordFilter('  ', [1, 1])).toBe('(doc.docstate == 1)')
  })
})

describe('buildRecordColumns', () => {
  const fields = [field({ name: 'title' }), field({ name: 'amount', type: 'Number' }), field({ name: 'note' }), field({ name: 'extra' })]
  const base = { fields, submittable: false, linkLabels: {}, updatedLine: () => 'Today' }

  it('leads with status for submittable types', () => {
    const ids = buildRecordColumns({ ...base, submittable: true }).map((column) => column.id)
    expect(ids).toEqual(['docstate', 'title', 'amount', 'note', 'extra'])
  })

  it('stacks the status and first two fields on phones', () => {
    const metas = buildRecordColumns({ ...base, submittable: true }).map((column) => (column.meta as { hideOnMobile?: boolean } | undefined)?.hideOnMobile ?? false)
    expect(metas).toEqual([false, false, false, true, true])
  })

  it('right-aligns numbers and centers checkboxes', () => {
    const columns = buildRecordColumns({ ...base, fields: [field({ name: 'amount', type: 'Number' }), field({ name: 'done', type: 'Checkbox' })] })
    const metas = columns.map((column) => column.meta as { numeric?: boolean; center?: boolean })
    expect(metas[0]?.numeric).toBe(true)
    expect(metas[1]?.center).toBe(true)
  })

  it('picks the label field as primary, else the first field', () => {
    const render = (columns: ReturnType<typeof buildRecordColumns>, id: string) => {
      const column = columns.find((item) => item.id === id)
      const cell = column?.cell as unknown as (ctx: { row: { original: RecordMessage } }) => { type: unknown }
      return cell({ row: { original: record({ title: 'T', note: 'N' }) } }).type
    }
    const primary = buildRecordColumns({ ...base, primaryField: 'note' })
    expect(primary.map((column) => column.id)).toEqual(['title', 'amount', 'note', 'extra'])
    expect(render(primary, 'note')).toBe(PrimaryCell)
    expect(render(primary, 'title')).toBe('span')
    const missing = buildRecordColumns({ ...base, primaryField: 'gone' })
    expect(render(missing, 'title')).toBe(PrimaryCell)
  })

  it('never sorts: server pages arrive in API order', () => {
    for (const column of buildRecordColumns({ ...base, submittable: true })) {
      expect(column.enableSorting).toBe(false)
    }
  })
})
