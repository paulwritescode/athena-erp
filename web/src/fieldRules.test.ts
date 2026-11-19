import { describe, expect, it } from 'vitest'
import cases from '../../internal/server/testdata/field_rules.json'
import { validateField } from './fieldRules'

// The server tests read the same table (internal/server/fieldrules_test.go),
// so a rule change that is made on only one side fails a test.
function expand(value: string): string {
  const match = /^\{repeat:(.*):(\d+)\}$/.exec(value)
  return match ? (match[1] ?? '').repeat(Number(match[2])) : value
}

describe('validateField parity with the server', () => {
  for (const c of cases) {
    it(c.name, () => {
      const field = { name: 'f', label: 'Field', type: c.type, required: c.required, target_record: c.target_record }
      expect(validateField(field, expand(c.value)) === '').toBe(c.valid)
    })
  }

  it('accepts any checkbox state', () => {
    expect(validateField({ name: 'f', label: 'Field', type: 'Checkbox', required: true }, '')).toBe('')
  })
})
