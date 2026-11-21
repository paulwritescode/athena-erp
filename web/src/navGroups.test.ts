import { describe, expect, it } from 'vitest'
import {
  loadCollapsedGroups,
  navGroupsKey,
  recordTypeGroups,
  saveCollapsedGroups,
  visibleBuildGroups,
} from './pages/registry'

function fakeStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial))
  return {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => {
      data.set(key, value)
    },
    removeItem: (key: string) => {
      data.delete(key)
    },
    data,
  }
}

describe('visibleBuildGroups', () => {
  it('orders §15 groups with Overview first and unheaded', () => {
    const groups = visibleBuildGroups(true, ['rules:read', 'files:read', 'access:users', 'ledger:read'])
    expect(groups.map((group) => group.label)).toEqual([null, 'Build', 'Operate', 'Administer'])
    expect(groups[0]?.entries.map((entry) => entry.key)).toEqual(['overview'])
    expect(groups[1]?.entries.map((entry) => entry.key)).toEqual(['record-types', 'packs', 'rules', 'files'])
    expect(groups[2]?.entries.map((entry) => entry.key)).toEqual(['ledger', 'reports', 'jobs'])
    expect(groups[3]?.entries.map((entry) => entry.key)).toEqual(['access', 'settings', 'setup'])
  })

  it('hides groups left empty by gating', () => {
    const groups = visibleBuildGroups(false, ['rules:read'])
    expect(groups.map((group) => group.label)).toEqual(['Build', 'Administer'])
    expect(groups[0]?.entries.map((entry) => entry.key)).toEqual(['rules'])
    expect(groups[1]?.entries.map((entry) => entry.key)).toEqual(['settings'])
  })

  it('hides everything signed out', () => {
    expect(visibleBuildGroups(true, [], false)).toEqual([])
  })
})

describe('recordTypeGroups', () => {
  it('puts ungrouped types under Records', () => {
    const groups = recordTypeGroups([
      { name: 'task', label: 'Task' },
      { name: 'note', label: 'Note' },
    ])
    expect(groups).toHaveLength(1)
    expect(groups[0]?.key).toBe('records')
    expect(groups[0]?.types.map((type) => type.name)).toEqual(['task', 'note'])
  })

  it('groups Pack and Module types under their owner', () => {
    const groups = recordTypeGroups([
      { name: 'task', label: 'Task' },
      { name: 'invoice', label: 'Invoice', pack: 'Ledger' },
      { name: 'payment', label: 'Payment', pack: 'Ledger' },
      { name: 'shift', label: 'Shift', module: 'Rota' },
    ])
    expect(groups.map((group) => group.label)).toEqual(['Records', 'Ledger', 'Rota'])
    expect(groups[1]?.types.map((type) => type.name)).toEqual(['invoice', 'payment'])
  })
})

describe('collapsed group storage', () => {
  it('keys memory per user', () => {
    expect(navGroupsKey('ada')).toBe('athena-nav-groups:ada')
  })

  it('round-trips a set and clears on empty', () => {
    const storage = fakeStorage()
    saveCollapsedGroups('ada', new Set(['data', 'records']), storage)
    expect(loadCollapsedGroups('ada', storage)).toEqual(new Set(['data', 'records']))
    expect(loadCollapsedGroups('grace', storage)).toEqual(new Set())
    saveCollapsedGroups('ada', new Set(), storage)
    expect(storage.data.has('athena-nav-groups:ada')).toBe(false)
  })

  it('survives corrupt JSON and missing users', () => {
    const storage = fakeStorage({ 'athena-nav-groups:ada': 'not json{', 'athena-nav-groups:bad': '"nope"' })
    expect(loadCollapsedGroups('ada', storage)).toEqual(new Set())
    expect(loadCollapsedGroups('bad', storage)).toEqual(new Set())
    expect(loadCollapsedGroups('', storage)).toEqual(new Set())
  })
})
