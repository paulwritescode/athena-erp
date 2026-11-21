import { describe, expect, it } from 'vitest'
import { actionKind, chunkByKind, type ActionItem, type ActionKind } from './components/actions'

function action(id: string, kind?: ActionKind): ActionItem {
  return { id, label: id, kind }
}

describe('actionKind', () => {
  it('defaults to secondary', () => {
    expect(actionKind(action('a'))).toBe('secondary')
    expect(actionKind(action('a', 'destructive'))).toBe('destructive')
  })
})

describe('chunkByKind', () => {
  it('groups consecutive actions of one kind', () => {
    const chunks = chunkByKind([action('a', 'primary'), action('b', 'secondary'), action('c', 'secondary')])
    expect(chunks).toHaveLength(2)
    expect(chunks[0]?.actions.map((item) => item.id)).toEqual(['a'])
    expect(chunks[1]?.actions.map((item) => item.id)).toEqual(['b', 'c'])
  })

  it('splits when a kind returns later and never reorders', () => {
    const chunks = chunkByKind([action('a', 'secondary'), action('b', 'caution'), action('c', 'secondary')])
    expect(chunks.map((chunk) => chunk.actions.map((item) => item.id))).toEqual([['a'], ['b'], ['c']])
  })

  it('marks a divider before a trailing destructive chunk only', () => {
    const trailing = chunkByKind([action('a', 'secondary'), action('b', 'destructive')])
    expect(trailing.map((chunk) => chunk.dividerBefore)).toEqual([false, true])
    const lone = chunkByKind([action('b', 'destructive')])
    expect(lone.map((chunk) => chunk.dividerBefore)).toEqual([false])
    const safe = chunkByKind([action('a', 'primary'), action('b', 'caution')])
    expect(safe.map((chunk) => chunk.dividerBefore)).toEqual([false, false])
  })

  it('handles empty input', () => {
    expect(chunkByKind([])).toEqual([])
  })
})
