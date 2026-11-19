import { describe, expect, it } from 'vitest'
import cases from '../../internal/server/testdata/field_rules.json'
import { compareDecimal, docstates, fieldEditable, isCalculated, maxExpressionLength, validateExpressionText, validateField, validateManualName, validateNamingSeries } from './fieldRules'
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

// The lifecycle rule has no shared table: the server tests in
// internal/server/lifecycle_test.go cover the same three states.
describe('fieldEditable', () => {
  const plain: Field = { name: 'a', label: 'A', type: 'Text', required: false }
  const late: Field = { ...plain, allow_on_submit: true }
  it('lets a draft or a non-submittable Record Type change every field', () => {
    expect(fieldEditable(plain, true, docstates.draft)).toBe(true)
    expect(fieldEditable(plain, false, docstates.cancelled)).toBe(true)
  })
  it('lets a submitted document change only allow_on_submit fields', () => {
    expect(fieldEditable(plain, true, docstates.submitted)).toBe(false)
    expect(fieldEditable(late, true, docstates.submitted)).toBe(true)
  })
  it('keeps a cancelled document fixed', () => {
    expect(fieldEditable(late, true, docstates.cancelled)).toBe(false)
  })
})

// These lists match internal/numbering/numbering_test.go and internal/server/naming.go.
describe('naming rules', () => {
  it('accepts series patterns with one counter', () => {
    for (const pattern of ['SINV-{yyyy}-{mm}-{#####}', 'INV{yy}/{###}', '{#}', '']) expect(validateNamingSeries(pattern)).toBe('')
  })
  it('rejects series patterns the server refuses', () => {
    for (const pattern of ['NOCOUNTER', 'A-{#}-{##}', 'A-{foo}-{#}', 'A-{#', 'A}-{#}', 'A-{##########}', 'A-{}', 'A'.repeat(101) + '{#}']) expect(validateNamingSeries(pattern)).not.toBe('')
  })
  it('checks manual names', () => {
    expect(validateManualName('LEGACY-0001')).toBe('')
    expect(validateManualName('')).toBe('')
    expect(validateManualName('0190a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b')).not.toBe('')
    expect(validateManualName('x'.repeat(141))).not.toBe('')
  })
})

// These match internal/server/expressions.go: a formula field is stored by the
// kernel, so it is never edited or required of a person, and an expression is
// at most 1000 characters.
describe('calculated fields', () => {
  const total: Field = { name: 'total', label: 'Total', type: 'Number', required: true, formula: 'doc.qty * doc.rate' }
  it('is not editable in any state and never asks for a value', () => {
    expect(isCalculated(total)).toBe(true)
    for (const state of [docstates.draft, docstates.submitted, docstates.cancelled]) expect(fieldEditable({ ...total, allow_on_submit: true }, true, state)).toBe(false)
    expect(validateField(total, '')).toBe('')
  })
  it('limits expression length', () => {
    expect(validateExpressionText('doc.a > 1')).toBe('')
    expect(validateExpressionText('x'.repeat(maxExpressionLength))).toBe('')
    expect(validateExpressionText('x'.repeat(maxExpressionLength + 1))).not.toBe('')
  })
})
