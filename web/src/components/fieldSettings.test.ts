import { describe, expect, it } from 'vitest'
import type { Field, RecordType } from '../types'
import {
  changeType,
  childTypesOf,
  composeTotal,
  fieldNameFromLabel,
  forgetSavedFields,
  isNewField,
  isTotalTarget,
  normalizeNewFieldName,
  totalSourceFields,
  trackNewField,
  updateFieldLabel,
  updateFieldName,
} from './fieldSettings'

function field(overrides: Partial<Field> = {}): Field {
  return { name: 'amount', label: 'Amount', type: 'Number', required: false, ...overrides }
}

function recordType(overrides: Partial<RecordType> = {}): RecordType {
  return { name: 'invoice', label: 'Invoice', description: '', kind: 'master', fields: [], revision: 1, record_count: 0, ...overrides }
}

describe('changeType', () => {
  it('drops what the new type cannot keep and takes its defaults', () => {
    const f = field({ type: 'Text', default: 'x', min: '1', formula: 'doc.a', target_record_type: 'customer', unique: true })
    f.type = 'Money'
    changeType(f)
    expect(f.currency).toBe('USD')
    expect(f.scale).toBe(2)
    expect(f.default).toBeUndefined()
    expect(f.formula).toBeUndefined()
    expect(f.target_record_type).toBeUndefined()
    expect(f.unique).toBe(true)
  })

  it('seeds one choice for Enum and plain text for the rest', () => {
    const f = field({ type: 'Text' })
    f.type = 'Enum'
    changeType(f)
    expect(f.values).toEqual([{ value: 'option_1', label: 'Option 1' }])
    f.type = 'Quantity'
    changeType(f)
    expect(f.uom).toBe('unit')
    expect(f.values).toBeUndefined()
  })

  it('drops unique and indexed where they cannot apply', () => {
    const f = field({ type: 'Text', unique: true, index: true })
    f.type = 'Long text'
    changeType(f)
    expect(f.unique).toBeUndefined()
    expect(f.index).toBeUndefined()
  })
})

describe('label to key derivation', () => {
  it('derives the key from the label while the field is new', () => {
    const f = field({ name: 'field_1', label: 'New field' })
    trackNewField(f)
    expect(isNewField(f)).toBe(true)
    updateFieldLabel(f, 'Customer name', [f])
    expect(f.label).toBe('Customer name')
    expect(f.name).toBe('customer_name')
  })

  it('leaves a hand-typed key alone', () => {
    const f = field({ name: 'field_1', label: 'New field' })
    trackNewField(f)
    updateFieldName(f, 'buyer')
    updateFieldLabel(f, 'Customer name', [f])
    expect(f.name).toBe('buyer')
  })

  it('normalizes a pending key and stops deriving after save', () => {
    const f = field({ name: 'field_1', label: 'New field' })
    trackNewField(f)
    updateFieldName(f, 'Customer Name!')
    normalizeNewFieldName(f, [f])
    expect(f.name).toBe('customer_name')
    forgetSavedFields([f])
    expect(isNewField(f)).toBe(false)
    updateFieldLabel(f, 'Buyer', [f])
    expect(f.name).toBe('customer_name')
  })

  it('avoids taken and reserved keys', () => {
    const other = field({ name: 'amount', label: 'Amount' })
    const f = field({ name: 'field_2', label: 'New field' })
    expect(fieldNameFromLabel('Amount', f, [other, f])).toBe('amount_2')
    expect(fieldNameFromLabel('Revision', f, [f])).toBe('field_revision')
  })
})

describe('totals composer', () => {
  const line = recordType({ name: 'line', label: 'Line', kind: 'child', parent_record_type: 'invoice', fields: [
    field({ name: 'qty', label: 'Qty', type: 'Number' }),
    field({ name: 'rate', label: 'Rate', type: 'Currency' }),
    field({ name: 'note', label: 'Note', type: 'Text' }),
  ] })
  const types = [recordType(), line, recordType({ name: 'customer', label: 'Customer' })]

  it('finds the child types of the edited type', () => {
    expect(childTypesOf(types, 'invoice').map((item) => item.name)).toEqual(['line'])
    expect(childTypesOf(types, 'customer')).toEqual([])
  })

  it('offers decimals to decimal fields and money to money fields', () => {
    expect(totalSourceFields(line.fields, 'Number').map((item) => item.name)).toEqual(['qty', 'rate'])
    expect(totalSourceFields(line.fields, 'Money')).toEqual([])
    const priced = recordType({ fields: [field({ name: 'price', label: 'Price', type: 'Money' })] })
    expect(totalSourceFields(priced.fields, 'Money').map((item) => item.name)).toEqual(['price'])
  })

  it('composes the total expression from picks', () => {
    expect(composeTotal('sum', 'line', 'qty')).toBe('sum(doc.line.qty)')
  })

  it('targets numeric and money fields only', () => {
    for (const type of ['Number', 'Currency', 'Money', 'Quantity']) expect(isTotalTarget(type)).toBe(true)
    for (const type of ['Text', 'Date', 'Checkbox', 'Link', 'Enum']) expect(isTotalTarget(type)).toBe(false)
  })
})
