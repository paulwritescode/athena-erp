import { describe, expect, it } from 'vitest'
import { actionLetter, describeScope, diffAssignments, formatScope, maxScopeValueBytes, parseScope, previewRoleGrants } from './assignmentPreview'

describe('parseScope', () => {
  it('reads dimensions and values', () => {
    const { scope, problems } = parseScope('company=acme,globex;branch=north')
    expect(problems).toEqual([])
    expect(scope).toEqual({ company: ['acme', 'globex'], branch: ['north'] })
  })
  it('treats empty text as the empty Scope', () => {
    expect(parseScope('  ')).toEqual({ scope: {}, problems: [] })
  })
  it('rejects dimensions that are not field keys', () => {
    const { problems } = parseScope('Company=acme;co-mpany=x;novalue')
    expect(problems).toHaveLength(3)
  })
  it('rejects empty and oversized values', () => {
    expect(parseScope('company=').problems).toHaveLength(1)
    expect(parseScope(`company=${'x'.repeat(maxScopeValueBytes + 1)}`).problems).toHaveLength(1)
  })
  it('rejects too many dimensions', () => {
    const text = Array.from({ length: 33 }, (_, i) => `d${i}=x`).join(';')
    expect(parseScope(text).problems.length).toBeGreaterThan(0)
  })
})

describe('formatScope and describeScope', () => {
  it('round-trips through parse', () => {
    const text = 'branch=north;company=acme,globex'
    expect(formatScope(parseScope(text).scope)).toBe(text)
    expect(formatScope({})).toBe('')
  })
  it('reads aloud', () => {
    expect(describeScope({})).toBe('Everywhere')
    expect(describeScope({ company: ['acme', 'globex'], branch: ['north'] })).toBe('Branch north · Company acme, globex')
  })
})

describe('diffAssignments', () => {
  const base = { role: 'Clerk', scope: { company: ['acme'] }, activeFrom: '', expiresAt: '' }
  it('lists every difference', () => {
    expect(
      diffAssignments(base, { role: 'Manager', scope: { company: ['acme', 'globex'], branch: ['north'] }, activeFrom: '2026-01-01', expiresAt: '' }),
    ).toEqual([
      'Role: Clerk → Manager.',
      'Active from: unbounded → 2026-01-01.',
      'Scope gains branch=north.',
      'Scope company gains globex.',
    ])
  })
  it('lists removals', () => {
    expect(diffAssignments(base, { role: 'Clerk', scope: {}, activeFrom: '', expiresAt: '2026-12-31' })).toEqual([
      'Expires: unbounded → 2026-12-31.',
      'Scope loses company.',
    ])
  })
  it('is empty without changes', () => {
    expect(diffAssignments(base, { ...base, scope: { company: ['acme'] } })).toEqual([])
  })
})

describe('actionLetter', () => {
  it('maps every action to one letter', () => {
    expect([
      ['read', 'R'],
      ['create', 'C'],
      ['write', 'W'],
      ['delete', 'D'],
      ['submit', 'S'],
      ['cancel', 'X'],
      ['amend', 'A'],
      ['audit', 'V'],
    ].map(([action, letter]) => actionLetter(action) === letter).every(Boolean)).toBe(true)
    expect(actionLetter('fly')).toBe('?')
  })
})

describe('previewRoleGrants', () => {
  it('summarizes Grants with Scope', () => {
    expect(
      previewRoleGrants(
        [
          { recordTypeKey: 'invoice', actions: ['read', 'write'], permissionLevel: 0, condition: '' },
          { recordTypeKey: '*', actions: ['read'], permissionLevel: 2, condition: 'doc.total > 1' },
        ],
        { invoice: 'Invoices' },
        { company: ['acme'] },
      ),
    ).toEqual([
      'Read, Write Invoices in Company acme.',
      'Read everything in Company acme (field level 2) where doc.total > 1.',
    ])
  })
  it('says everywhere for an empty Scope', () => {
    expect(previewRoleGrants([{ recordTypeKey: 'invoice', actions: ['read'], permissionLevel: 0, condition: '' }], {}, {})).toEqual([
      'Read invoice.',
    ])
  })
})
