// Mirror pins for web/src/conditions.ts: byte-identical output with Go's
// internal/expr CompileTree for representative trees (the Go suite pins
// every operator; these guard the mirror against drift).
import { describe, expect, it } from 'vitest'
import { builderFieldsFor, compileTree, isEmptyTree, OPERATORS_FOR_TYPE, parseConditionTree, type ConditionOp, type TreeCondition, type TreeField } from './conditions'

const fields: Record<string, TreeField> = {
  'doc.name': { kind: 'string' },
  'doc.qty': { kind: 'decimal' },
  'doc.total': { kind: 'money', currency: 'USD', scale: 2 },
  'doc.due': { kind: 'date' },
  'doc.paid': { kind: 'bool' },
  'doc.docstate': { kind: 'int' },
}

function cond(field: string, op: ConditionOp, values?: string[]): TreeCondition {
  return values === undefined ? { field, op } : { field, op, values }
}

describe('compileTree', () => {
  it.each([
    ['is', cond('doc.name', 'is', ['Acme']), `doc.name == 'Acme'`],
    ['escapes', cond('doc.name', 'is', [`o'clock\\side`]), `doc.name == 'o\\'clock\\\\side'`],
    ['contains', cond('doc.name', 'contains', ['ac.me']), 'doc.name.matches(\'(?s).*ac\\\\.me.*\')'],
    ['not contains', cond('doc.name', 'not_contains', ['x']), `!(doc.name.matches('(?s).*x.*'))`],
    ['starts with', cond('doc.name', 'starts_with', ['Ac']), `doc.name.startsWith('Ac')`],
    ['empty', cond('doc.name', 'is_empty'), `doc.name == null`],
    ['any of', cond('doc.name', 'is_any_of', ['a', 'b']), `(doc.name == 'a' || doc.name == 'b')`],
    ['none of', cond('doc.name', 'is_none_of', ['a', 'b']), `(doc.name != 'a' && doc.name != 'b')`],
    ['decimal', cond('doc.qty', 'more_than', ['1.50']), `doc.qty > dec('1.50')`],
    ['money', cond('doc.total', 'at_most', ['10.50']), `doc.total <= money(dec('10.50'), 'USD', 2)`],
    ['date', cond('doc.due', 'before', ['2026-12-01']), `doc.due < date('2026-12-01')`],
    ['last n days', cond('doc.due', 'in_the_last_n_days', ['30']), `(doc.due >= add_days(today(), -30) && doc.due <= today())`],
    ['next n days', cond('doc.due', 'in_the_next_n_days', ['7']), `(doc.due >= today() && doc.due <= add_days(today(), 7))`],
    ['checked', cond('doc.paid', 'is_checked'), `doc.paid == true`],
    ['docstate', cond('doc.docstate', 'is', ['1']), `doc.docstate == 1`],
  ] as Array<[string, TreeCondition, string]>)('%s', (_name, c, want) => {
    expect(compileTree({ root: { all: true, conditions: [c] } }, fields)).toBe(want)
  })

  it('nests groups', () => {
    expect(compileTree({
      root: {
        all: true,
        conditions: [{ field: 'doc.paid', op: 'is_checked' }],
        groups: [{ all: false, conditions: [
          { field: 'doc.name', op: 'is', values: ['a'] },
          { field: 'doc.qty', op: 'more_than', values: ['2'] },
        ] }],
      },
    }, fields)).toBe(`(doc.paid == true && (doc.name == 'a' || doc.qty > dec('2')))`)
  })

  it.each([
    ['unknown field', cond('doc.nope', 'is', ['a']), 'unknown field'],
    ['op on kind', cond('doc.qty', 'contains', ['a']), 'cannot be used'],
    ['bad decimal', cond('doc.qty', 'more_than', ['abc']), 'not a number'],
    ['bad date', cond('doc.due', 'before', ['yesterday']), 'not a YYYY-MM-DD date'],
    ['bad calendar date', cond('doc.due', 'before', ['2026-02-30']), 'not a YYYY-MM-DD date'],
    ['bad days', cond('doc.due', 'in_the_last_n_days', ['-1']), 'not a day count'],
    ['money fit', cond('doc.total', 'is', ['1.234']), 'does not fit USD'],
  ] as Array<[string, TreeCondition, string]>)('refuses %s', (_name, c, want) => {
    expect(() => compileTree({ root: { all: true, conditions: [c] } }, fields)).toThrowError(want)
  })

  it('refuses an empty root and nested singles', () => {
    expect(() => compileTree({ root: { all: true } }, fields)).toThrowError('at least one condition')
    expect(() => compileTree({ root: { all: true, conditions: [{ field: 'doc.name', op: 'is', values: ['a'] }, { field: 'doc.name', op: 'is_not', values: ['b'] }], groups: [{ all: true, conditions: [{ field: 'doc.name', op: 'is', values: ['c'] }] }] } }, fields)).toThrowError('at least two')
  })
})

describe('OPERATORS_FOR_TYPE', () => {
  it('narrows by field type', () => {
    expect(OPERATORS_FOR_TYPE['Enum']).toContain('is_any_of')
    expect(OPERATORS_FOR_TYPE['Enum']).not.toContain('contains')
    expect(OPERATORS_FOR_TYPE['Link']).toEqual(['is', 'is_not', 'is_empty', 'is_not_empty'])
    expect(OPERATORS_FOR_TYPE['Checkbox']).toEqual(['is_checked', 'is_not_checked'])
    expect(OPERATORS_FOR_TYPE['Date']).toContain('in_the_last_n_days')
  })
})

// The builder field map both authoring hosts share (Rules form today, the
// lists editor next): doc. names, kind mapping, and Enum choices.
describe('builderFieldsFor', () => {
  it('maps names, kinds and money scale', () => {
    const out = builderFieldsFor([
      { name: 'total', label: 'Total', type: 'Money', currency: 'KES', scale: 2 },
      { name: 'paid', label: 'Paid', type: 'Checkbox' },
      { name: 'city', label: 'City', type: 'ShortText' },
    ])
    expect(out[0]).toMatchObject({ name: 'doc.total', label: 'Total', type: 'Money', kind: 'money', currency: 'KES', scale: 2 })
    expect(out[1]).toMatchObject({ name: 'doc.paid', kind: 'bool' })
    expect(out[2]).toMatchObject({ name: 'doc.city', kind: 'string' })
  })

  it('carries Enum choices through', () => {
    const out = builderFieldsFor([
      { name: 'status', label: 'Status', type: 'Enum', values: [{ value: 'open' }, { value: 'shut' }] },
    ])
    expect(out[0].choices).toEqual(['open', 'shut'])
  })
})

// The list chips parse stored trees and treat an empty one as clearing.
describe('parseConditionTree', () => {
  it('reads a stored tree', () => {
    const tree = parseConditionTree('{"root":{"all":true,"conditions":[{"field":"doc.name","op":"is","values":["x"]}]}}')
    expect(tree?.root.conditions).toHaveLength(1)
  })

  it('returns null for corrupt text and wrong shapes', () => {
    expect(parseConditionTree('nope')).toBeNull()
    expect(parseConditionTree('{"root":null}')).toBeNull()
    expect(parseConditionTree('[]')).toBeNull()
  })
})

describe('isEmptyTree', () => {
  it('treats null and bare groups as empty', () => {
    expect(isEmptyTree(null)).toBe(true)
    expect(isEmptyTree({ root: { all: true, conditions: [] } })).toBe(true)
    expect(isEmptyTree({ root: { all: true, groups: [{ all: false }] } })).toBe(true)
  })

  it('spots a condition at any depth', () => {
    expect(isEmptyTree({ root: { all: true, conditions: [{ field: 'doc.a', op: 'is' }] } })).toBe(false)
    expect(isEmptyTree({ root: { all: true, groups: [{ all: true, conditions: [{ field: 'doc.a', op: 'is' }] }] } })).toBe(false)
  })
})
