import { describe, expect, it } from 'vitest'
import { buildRoutePath, parseRoutePath } from './routePaths'
import type { Page } from './types'

describe('route paths', () => {
  it.each([
    ['/build/ledger', { kind: 'build', page: 'ledger' }],
    ['/build/gallery', { kind: 'build', page: 'gallery' }],
    ['/view/task', { kind: 'records', recordType: 'task', record: '' }],
    ['/view/task/new', { kind: 'records', recordType: 'task', record: 'new' }],
    ['/view/task/01a1fbb8-8153', { kind: 'records', recordType: 'task', record: '01a1fbb8-8153' }],
    ['/view/-/queue', { kind: 'stub', page: 'queue' }],
    ['/view/-/count', { kind: 'stub', page: 'count' }],
    ['/view', { kind: 'view' }],
    ['/view/', { kind: 'view' }],
    ['/view/-', { kind: 'view' }],
    ['/view/-/nope', { kind: 'view' }],
  ])('parses %s', (pathname, expected) => {
    expect(parseRoutePath(pathname)).toEqual(expected)
  })

  it.each([['/'], [''], ['/build/nope'], ['/other/deep']])('leaves %s to the query fallback', (pathname) => {
    expect(parseRoutePath(pathname)).toBeNull()
  })

  it('routes a Record Type keyed like a stub to its own records', () => {
    expect(parseRoutePath('/view/queue')).toEqual({ kind: 'records', recordType: 'queue', record: '' })
    expect(parseRoutePath('/view/count')).toEqual({ kind: 'records', recordType: 'count', record: '' })
  })

  it.each([
    ['build', 'ledger', '', '', '/build/ledger'],
    ['build', 'gallery', '', '', '/build/gallery'],
    ['view', 'overview', 'task', '', '/view/task'],
    ['view', 'overview', 'task', 'new', '/view/task/new'],
    ['view', 'overview', 'task', '01a1fbb8-8153', '/view/task/01a1fbb8-8153'],
    ['view', 'queue', '', '', '/view/-/queue'],
    ['view', 'count', '', '', '/view/-/count'],
    ['view', 'overview', '', '', '/view'],
  ])('builds %s', (mode, page, selected, record, expected) => {
    expect(buildRoutePath(mode as 'build' | 'view', page as Page, selected as string, record as string)).toBe(expected)
  })
})
