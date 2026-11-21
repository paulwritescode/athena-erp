import { describe, expect, it } from 'vitest'
import { deriveRecordTypeKey, snakeKey, validateNewRecordType } from './recordTypeKeys'

describe('record type keys', () => {
  it('derives snake keys from labels', () => {
    expect(deriveRecordTypeKey('Customer', [])).toBe('customer')
    expect(deriveRecordTypeKey('Sales Order!', [])).toBe('sales_order')
    expect(deriveRecordTypeKey('Café Münchën', [])).toBe('cafe_munchen')
    expect(deriveRecordTypeKey('  ', [])).toBe('record_type')
    expect(deriveRecordTypeKey('12 Months', [])).toBe('record_type_12_months')
  })

  it('makes derived keys unique', () => {
    expect(deriveRecordTypeKey('Customer', ['customer'])).toBe('customer_2')
    expect(deriveRecordTypeKey('Customer', ['customer', 'customer_2'])).toBe('customer_3')
  })

  it('keeps field keys out of reserved and taken names', () => {
    expect(snakeKey('Name', 'field', new Set(['name']), ['id'])).toBe('name_2')
    expect(snakeKey('Id', 'field', new Set(), ['id'])).toBe('field_id')
  })

  it('validates the create dialog entry', () => {
    expect(validateNewRecordType('', 'x', [], [])).toEqual({ name: 'Enter a name.' })
    expect(validateNewRecordType('  ', 'x', [], [])).toEqual({ name: 'Enter a name.' })
    expect(validateNewRecordType('Customer', 'customer', [], ['customer'])).toEqual({ name: 'That name is taken. Try another.' })
    expect(validateNewRecordType('Customer', 'Customer', [], [])).toEqual({ key: 'Use lowercase letters, numbers and underscores, starting with a letter.' })
    expect(validateNewRecordType('Customer', 'customer', ['customer'], [])).toEqual({ key: 'That key is taken. Try another.' })
    expect(validateNewRecordType('Customer', 'customer', [], [])).toEqual({})
    expect(validateNewRecordType('x'.repeat(256), 'x', [], [])).toEqual({ name: 'Keep the name under 255 characters.' })
  })
})
