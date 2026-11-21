import { afterEach, describe, expect, it, vi } from 'vitest'
import { eventMatches, parseShortcut, registerShortcut, shortcutId, shortcutOwners, unregisterShortcut } from './shortcuts'

describe('shortcut parsing', () => {
  it.each([
    ['cmd+k', { key: 'k', meta: true, ctrl: false, alt: false, shift: false }],
    ['⌘+K', { key: 'k', meta: true, ctrl: false, alt: false, shift: false }],
    ['⌘K', { key: 'k', meta: true, ctrl: false, alt: false, shift: false }],
    ['⌃⇧P', { key: 'p', meta: false, ctrl: true, alt: false, shift: true }],
    ['ctrl+shift+p', { key: 'p', meta: false, ctrl: true, alt: false, shift: true }],
    ['alt+Enter', { key: 'enter', meta: false, ctrl: false, alt: true, shift: false }],
    ['escape', { key: 'escape', meta: false, ctrl: false, alt: false, shift: false }],
    ['space', { key: 'space', meta: false, ctrl: false, alt: false, shift: false }],
  ])('parses %s', (combo, expected) => {
    expect(parseShortcut(combo)).toEqual(expected)
  })

  it.each([[''], ['cmd'], ['a+b'], ['cmd+k+l']])('rejects %s', (combo) => {
    expect(parseShortcut(combo)).toBeNull()
  })

  it('canonicalizes modifier order', () => {
    expect(shortcutId(parseShortcut('shift+cmd+k')!)).toBe(shortcutId(parseShortcut('cmd+shift+k')!))
  })

  it('matches events exactly', () => {
    const parsed = parseShortcut('cmd+k')!
    expect(eventMatches({ key: 'k', metaKey: true, ctrlKey: false, altKey: false, shiftKey: false }, parsed)).toBe(true)
    expect(eventMatches({ key: 'k', metaKey: false, ctrlKey: true, altKey: false, shiftKey: false }, parsed)).toBe(false)
    expect(eventMatches({ key: 'K', metaKey: true, ctrlKey: false, altKey: false, shiftKey: false }, parsed)).toBe(true)
  })
})

describe('shortcut registry', () => {
  const handlers: (() => void)[] = []
  afterEach(() => {
    for (const handler of handlers.splice(0)) unregisterShortcut(handler)
    vi.restoreAllMocks()
  })

  function handler() {
    const fn = vi.fn()
    handlers.push(fn)
    return fn
  }

  it('registers and unregisters', () => {
    const fn = handler()
    expect(registerShortcut('cmd+1', 'First', fn)).toBe(true)
    expect(shortcutOwners()).toEqual([{ combo: 'meta+1', owner: 'First' }])
    unregisterShortcut(fn)
    expect(shortcutOwners()).toEqual([])
  })

  it('rejects conflicts and keeps the first owner', () => {
    const warned: string[] = []
    vi.spyOn(console, 'warn').mockImplementation((message: string) => warned.push(message))
    const first = handler()
    const second = handler()
    expect(registerShortcut('cmd+2', 'First', first)).toBe(true)
    expect(registerShortcut('cmd+2', 'Second', second)).toBe(false)
    expect(warned.join('\n')).toMatch(/conflicts with First/)
    expect(shortcutOwners()).toEqual([{ combo: 'meta+2', owner: 'First' }])
    unregisterShortcut(first)
    expect(registerShortcut('cmd+2', 'Second', second)).toBe(true)
  })

  it('reserves bare Escape for dismissal', () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(registerShortcut('escape', 'Never', handler())).toBe(false)
    expect(shortcutOwners()).toEqual([])
  })
})
