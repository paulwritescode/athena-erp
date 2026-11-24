import { describe, expect, it } from 'vitest'
import { ruleEvents, ruleProblem, ruleTypes, type RuleAuthoring } from './ruleDefinitions'

const valid = { key: 'qty_positive', label: 'Quantity above zero', ruleType: 'validation', recordTypeKey: 'invoice', event: 'validate', priority: 10, source: 'def validate(doc, ctx):\n    pass\n' }

describe('ruleProblem', () => {
  it('accepts a valid definition', () => expect(ruleProblem(valid)).toBeNull())
  it('mirrors the key pattern', () => {
    for (const key of ['', 'Upper', '1abc', 'a-b', 'a'.repeat(64)]) expect(ruleProblem({ ...valid, key })).not.toBeNull()
    expect(ruleProblem({ ...valid, key: 'a'.repeat(63) })).toBeNull()
  })
  it('limits the event to the type', () => {
    expect(ruleProblem({ ...valid, ruleType: 'derivation', event: 'before_submit' })).not.toBeNull()
    expect(ruleProblem({ ...valid, ruleType: 'guard', event: 'on_trash' })).toBeNull()
    expect(ruleProblem({ ...valid, ruleType: 'transform', recordTypeKey: '', event: '' })).toBeNull()
    expect(ruleProblem({ ...valid, ruleType: 'transform', recordTypeKey: '', event: 'validate' })).not.toBeNull()
  })
  it('needs a Record Type unless it is a transform, and a priority in range', () => {
    expect(ruleProblem({ ...valid, recordTypeKey: '' })).not.toBeNull()
    expect(ruleProblem({ ...valid, priority: 1001 })).not.toBeNull()
    expect(ruleProblem({ ...valid, priority: -1 })).not.toBeNull()
  })
  it('lists every type with its events', () => {
    for (const type of ruleTypes) expect(Array.isArray(ruleEvents[type])).toBe(true)
  })
  it('accepts a valid builder validation without source', () => {
    const tree = { root: { all: true, conditions: [{ field: 'doc.qty', op: 'more_than' as const, values: ['0'] }] } }
    const fields = { 'doc.qty': { kind: 'decimal' as const } }
    expect(ruleProblem({ ...valid, source: '' }, { mode: 'builder', tree, message: 'Qty must beat zero.', sets: [], fields })).toBeNull()
  })
  it('checks builder trees, messages and sets', () => {
    const authoring: RuleAuthoring = {
      mode: 'builder',
      tree: { root: { all: true, conditions: [{ field: 'doc.qty', op: 'more_than', values: ['0'] }] } },
      message: 'Fix it.',
      sets: [],
      fields: { 'doc.qty': { kind: 'decimal' } },
    }
    expect(ruleProblem({ ...valid, source: '' }, { ...authoring, tree: null })).not.toBeNull()
    expect(ruleProblem({ ...valid, source: '' }, { ...authoring, message: '' })).not.toBeNull()
    expect(ruleProblem({ ...valid, ruleType: 'derivation', event: 'validate', source: '' }, authoring)).not.toBeNull()
    expect(ruleProblem({ ...valid, ruleType: 'derivation', event: 'validate', source: '' }, { ...authoring, sets: [{ field: 'doc.qty', valueExpr: 'dec("1")' }] })).toBeNull()
    expect(ruleProblem({ ...valid, source: '' }, { ...authoring, tree: { root: { all: true, conditions: [{ field: 'doc.nope', op: 'is', values: ['a'] }] } } })).not.toBeNull()
  })
})
