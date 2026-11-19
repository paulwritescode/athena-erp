import { describe, expect, it } from 'vitest'
import cases from '../../internal/server/testdata/field_rules.json'
import { compareDecimal, validateField } from './fieldRules'
import type { Field } from './types'

// The server tests read the same table (internal/server/fieldrules_test.go),
// so a rule change that is made on only one side fails a test.
function expand(value: string): string {
  const match = /^\{repeat:(.*):(\d+)\}$/.exec(value)
  return match ? (match[1] ?? '').repeat(Number(match[2])) : value
}

type Case = (typeof cases)[number] & { max_length?: number; min?: string; max?: string; pattern?: string; values?: string[]; currency?: string; scale?: number; uom?: string }

function fieldOf(c: Case): Field {
  return {
    name: 'f',
    label: 'Field',
    type: c.type,
    required: c.required,
    target_record_type: c.target_record_type,
    ...(c.max_length ? { max_length: c.max_length } : {}),
    ...(c.min ? { min: c.min } : {}),
    ...(c.max ? { max: c.max } : {}),
    ...(c.pattern ? { pattern: c.pattern } : {}),
    ...(c.values ? { values: c.values.map((value) => ({ value, label: value })) } : {}),
    ...(c.currency ? { currency: c.currency } : {}),
    ...(c.uom ? { uom: c.uom } : {}),
    ...(c.scale !== undefined ? { scale: c.scale } : {}),
  }
}

describe('validateField parity with the server', () => {
  for (const c of cases as Case[]) {
    it(c.name, () => {
      expect(validateField(fieldOf(c), expand(c.value)) === '').toBe(c.valid)
    })
  }

  it('accepts any checkbox state', () => {
    expect(validateField({ name: 'f', label: 'Field', type: 'Checkbox', required: true }, '')).toBe('')
  })
})

describe('compareDecimal', () => {
  it('orders exactly, with no binary rounding', () => {
    expect(compareDecimal('0.1', '0.10')).toBe(0)
    expect(compareDecimal('0.30000000000000004', '0.3')).toBe(1)
    expect(compareDecimal('-1', '-0.999999')).toBe(-1)
    expect(compareDecimal('-0.5', '0.5')).toBe(-1)
  })
})
